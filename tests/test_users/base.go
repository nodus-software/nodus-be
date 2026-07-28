package test_users

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
	"nodus-health/internal/users"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/security"
	"nodus-health/pkg/utility"
)

type discardAudit struct{}

func (discardAudit) Record(context.Context, audit.Entry) error { return nil }

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
	service := users.NewService(repo, discardAudit{}, logger.NewLogger(), users.Config{
		AccessReviewCycle: 90 * 24 * time.Hour,
	})
	authorizer := &stubAuthorizer{permissions: map[string][]string{}}
	handler := users.NewHandler(service, authorizer, testJWTSecret, logger.NewLogger())
	r := chi.NewRouter()
	handler.RegisterRoutes(r)
	return &Env{Router: r, Repo: repo, authorizer: authorizer}
}

// NewActor registers a caller (with the given permissions and superuser
// status) with the stub authorizer and returns a bearer token for them. It
// does not create a row in Repo — use CreateUser for that.
func (e *Env) NewActor(t *testing.T, superuser bool, permissions ...string) (userID, accessToken string) {
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

// CreateUser adds a user row directly to the repo, bypassing the invite
// flow (owned by a different domain).
func (e *Env) CreateUser(t *testing.T, status users.Status) string {
	t.Helper()
	id, err := utility.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	e.Repo.users[id] = users.User{
		ID: id, FullName: "Test User", Username: "user-" + id, Email: id + "@example.com",
		Status: status, CreatedAt: time.Now(),
	}
	return id
}

func (e *Env) CreateRole(id, name string, isSuperuser, requiresProviderIdentifier bool) {
	e.Repo.roles[id] = users.Role{ID: id, Name: name, IsSuperuserRole: isSuperuser, RequiresProviderIdentifier: requiresProviderIdentifier}
}

func (e *Env) LockUser(userID string) {
	e.Repo.Lock()
	defer e.Repo.Unlock()
	u := e.Repo.users[userID]
	until := time.Now().Add(time.Hour)
	u.LockedUntil = &until
	e.Repo.users[userID] = u
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
	users      map[string]users.User
	userRoles  map[string][]string // userID -> role IDs
	roles      map[string]users.Role
	superusers map[string]bool
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		users: map[string]users.User{}, userRoles: map[string][]string{},
		roles: map[string]users.Role{}, superusers: map[string]bool{},
	}
}

func (r *memoryRepo) hydrate(u users.User) users.User {
	roleIDs := r.userRoles[u.ID]
	names := make([]string, 0, len(roleIDs))
	for _, rid := range roleIDs {
		if role, ok := r.roles[rid]; ok {
			names = append(names, role.Name)
		}
	}
	u.RoleNames = names
	if u.Permissions == nil {
		u.Permissions = []string{}
	}
	return u
}

func (r *memoryRepo) ListUsers(_ context.Context, filter users.ListUsersFilter) ([]users.User, error) {
	var out []users.User
	for _, u := range r.users {
		if filter.Status != nil && string(u.Status) != *filter.Status {
			continue
		}
		out = append(out, r.hydrate(u))
	}
	return out, nil
}

func (r *memoryRepo) GetUserByID(_ context.Context, id string) (*users.User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, users.ErrUserNotFound
	}
	v := r.hydrate(u)
	return &v, nil
}

func (r *memoryRepo) ReplaceUserRoles(_ context.Context, userID string, roleIDs []string) error {
	r.userRoles[userID] = roleIDs
	return nil
}

func (r *memoryRepo) UpdateUserStatus(_ context.Context, userID, status string) error {
	u, ok := r.users[userID]
	if !ok {
		return users.ErrUserNotFound
	}
	u.Status = users.Status(status)
	r.users[userID] = u
	return nil
}

func (r *memoryRepo) SetProviderIdentifier(_ context.Context, userID, identifier string) error {
	u, ok := r.users[userID]
	if !ok {
		return users.ErrUserNotFound
	}
	u.ProviderIdentifier = &identifier
	r.users[userID] = u
	return nil
}

func (r *memoryRepo) RecordAccessReview(_ context.Context, userID string, reviewedAt, nextDue time.Time) error {
	u, ok := r.users[userID]
	if !ok {
		return users.ErrUserNotFound
	}
	u.LastAccessReviewAt = &reviewedAt
	u.NextAccessReviewDue = &nextDue
	r.users[userID] = u
	return nil
}

func (r *memoryRepo) UnlockUser(_ context.Context, userID string) error {
	u, ok := r.users[userID]
	if !ok {
		return users.ErrUserNotFound
	}
	u.LockedUntil = nil
	r.users[userID] = u
	return nil
}

func (r *memoryRepo) GetRolesByIDs(_ context.Context, ids []string) ([]users.Role, error) {
	seen := map[string]bool{}
	var out []users.Role
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if role, ok := r.roles[id]; ok {
			out = append(out, role)
		}
	}
	return out, nil
}

func (r *memoryRepo) HasSuperuserRole(_ context.Context, userID string) (bool, error) {
	return r.superusers[userID], nil
}

func (r *memoryRepo) CountOtherActiveSuperusers(_ context.Context, userID string) (int64, error) {
	var count int64
	for id, isSuperuser := range r.superusers {
		if id != userID && isSuperuser && r.users[id].Status == users.StatusActive {
			count++
		}
	}
	return count, nil
}

func (r *memoryRepo) LockUserLifecycle(context.Context) error { return nil }

func (r *memoryRepo) DeactivateUser(_ context.Context, userID string, at time.Time) error {
	u, ok := r.users[userID]
	if !ok {
		return users.ErrUserNotFound
	}
	u.Status = users.StatusDeactivated
	u.DeactivatedAt = &at
	r.users[userID] = u
	return nil
}

func (r *memoryRepo) RevokeSessionsByUser(context.Context, string) error             { return nil }
func (r *memoryRepo) RevokeRefreshTokensByUser(context.Context, string) error        { return nil }
func (r *memoryRepo) ConsumeLoginChallengesByUser(context.Context, string) error     { return nil }
func (r *memoryRepo) ConsumePasswordResetTokensByUser(context.Context, string) error { return nil }
func (r *memoryRepo) ConsumeEnrollmentTokensByUser(context.Context, string) error    { return nil }

func (r *memoryRepo) WithinTx(_ context.Context, fn func(users.Repository) error) error {
	return fn(r)
}
