package test_roles

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"nodus-health/internal/audit"
	"nodus-health/internal/roles"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/security"
	"nodus-health/pkg/utility"
)

type discardAudit struct{}

func (discardAudit) Record(context.Context, audit.Entry) error { return nil }

// stubAuthorizer stands in for the Auth domain's real Authorize method:
// tests grant a user permissions directly rather than going through login.
type stubAuthorizer struct {
	mu          sync.Mutex
	permissions map[string][]string
}

func (a *stubAuthorizer) Authorize(_ context.Context, userID, _ string) ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.permissions[userID], nil
}

func (a *stubAuthorizer) grant(userID string, permissions []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissions[userID] = permissions
}

const testJWTSecret = "test-jwt-secret"

type Env struct {
	Router     http.Handler
	Repo       *memoryRepo
	authorizer *stubAuthorizer
}

func Setup(t *testing.T) *Env {
	t.Helper()
	repo := newMemoryRepo()
	service := roles.NewService(repo, discardAudit{}, logger.NewLogger())
	authorizer := &stubAuthorizer{permissions: map[string][]string{}}
	handler := roles.NewHandler(service, authorizer, testJWTSecret, logger.NewLogger())
	r := chi.NewRouter()
	handler.RegisterRoutes(r)
	return &Env{Router: r, Repo: repo, authorizer: authorizer}
}

// NewUser registers a user (with the given permissions and superuser
// status) with the stub authorizer and returns a bearer token for them.
func (e *Env) NewUser(t *testing.T, superuser bool, permissions ...string) (userID, accessToken string) {
	t.Helper()
	id, err := utility.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	e.authorizer.grant(id, permissions)
	e.Repo.superusers[id] = superuser
	token, _, err := security.IssueAccessToken(testJWTSecret, time.Hour, id, "session")
	if err != nil {
		t.Fatal(err)
	}
	return id, token
}

func (e *Env) SeedPermission(code string) {
	e.Repo.SeedPermission(code)
}

func (e *Env) JSON(t *testing.T, method, path, accessToken string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = strings.NewReader(string(data))
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	rec := httptest.NewRecorder()
	e.Router.ServeHTTP(rec, req)
	return rec
}

func Decode(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	data := envelope.Data
	if len(data) == 0 {
		data = rec.Body.Bytes()
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
}

// memoryRepo is a complete, deliberately small Repository implementation
// giving handler tests deterministic isolated state.
type memoryRepo struct {
	sync.Mutex
	roles       map[string]roles.Role
	rolePerms   map[string][]string
	permissions map[string]roles.Permission
	superusers  map[string]bool
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		roles: map[string]roles.Role{}, rolePerms: map[string][]string{},
		permissions: map[string]roles.Permission{}, superusers: map[string]bool{},
	}
}

func (r *memoryRepo) SeedPermission(code string) {
	r.permissions[code] = roles.Permission{ID: code, Code: code}
}

func (r *memoryRepo) ListRolesWithPermissions(_ context.Context) ([]roles.Role, error) {
	out := make([]roles.Role, 0, len(r.roles))
	for _, role := range r.roles {
		role.Permissions = r.rolePerms[role.ID]
		out = append(out, role)
	}
	return out, nil
}

func (r *memoryRepo) GetRoleByID(_ context.Context, id string) (*roles.Role, error) {
	role, ok := r.roles[id]
	if !ok {
		return nil, roles.ErrRoleNotFound
	}
	v := role
	v.Permissions = r.rolePerms[id]
	return &v, nil
}

func (r *memoryRepo) CreateRole(_ context.Context, role roles.Role) (*roles.Role, error) {
	for _, existing := range r.roles {
		if existing.Name == role.Name {
			return nil, roles.ErrRoleNameTaken
		}
	}
	r.roles[role.ID] = role
	v := role
	return &v, nil
}

func (r *memoryRepo) GetPermissionsByCodes(_ context.Context, codes []string) ([]roles.Permission, error) {
	seen := map[string]bool{}
	var out []roles.Permission
	for _, c := range codes {
		if seen[c] {
			continue
		}
		seen[c] = true
		if p, ok := r.permissions[c]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *memoryRepo) AddRolePermission(_ context.Context, roleID, permissionID string) error {
	r.rolePerms[roleID] = append(r.rolePerms[roleID], permissionID)
	return nil
}

func (r *memoryRepo) HasSuperuserRole(_ context.Context, userID string) (bool, error) {
	return r.superusers[userID], nil
}

func (r *memoryRepo) WithinTx(_ context.Context, fn func(roles.Repository) error) error {
	return fn(r)
}
