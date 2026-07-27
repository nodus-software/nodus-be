package test_invitation

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
	"nodus-health/internal/auth"
	"nodus-health/internal/invitation"
	"nodus-health/internal/tenant"
	"nodus-health/pkg/logger"
	"nodus-health/pkg/security"
	"nodus-health/pkg/utility"
)

type discardAudit struct{}

func (discardAudit) Record(context.Context, audit.Entry) error { return nil }

type sentMail struct{ To, Subject, Body string }
type memoryMailer struct {
	mu   sync.Mutex
	Sent []sentMail
}

func (m *memoryMailer) Send(_ context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Sent = append(m.Sent, sentMail{to, subject, body})
	return nil
}

func (m *memoryMailer) LastToken() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Sent) == 0 {
		return ""
	}
	const marker = "?token="
	parts := strings.SplitN(m.Sent[len(m.Sent)-1].Body, marker, 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

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
const testTenantID = "00000000-0000-0000-0000-000000000099"

type Env struct {
	Router     http.Handler
	Repo       *memoryRepo
	Mailer     *memoryMailer
	authorizer *stubAuthorizer
}

func Setup(t *testing.T) *Env {
	t.Helper()
	repo := newMemoryRepo()
	mailer := &memoryMailer{}
	service := invitation.NewService(repo, discardAudit{}, mailer, logger.NewLogger(), invitation.Config{
		BaseURL: "https://app.test", InviteTokenTTL: 24 * time.Hour, EnrollmentTokenTTL: 30 * time.Minute,
		BcryptCost: 4, OrganizationName: "Nodus Test",
		PasswordPolicy: auth.PasswordPolicy{MinLength: 12, RequireUppercase: true, RequireNumber: true, RequireSymbol: true},
	})
	authorizer := &stubAuthorizer{permissions: map[string][]string{}}
	handler := invitation.NewHandler(service, authorizer, testJWTSecret, logger.NewLogger())
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := tenant.WithContext(req.Context(), tenant.Identity{ID: testTenantID, Slug: "nodus-test"})
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	handler.RegisterRoutes(r)
	return &Env{Router: r, Repo: repo, Mailer: mailer, authorizer: authorizer}
}

func (e *Env) NewActor(t *testing.T, permissions ...string) (userID, accessToken string) {
	t.Helper()
	id, err := utility.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	e.authorizer.grant(id, permissions)
	token, _, err := security.IssueAccessToken(testJWTSecret, time.Hour, id, "session", testTenantID)
	if err != nil {
		t.Fatal(err)
	}
	return id, token
}

func (e *Env) CreateRole(id, name string, requiresProviderIdentifier bool) {
	e.Repo.roles[id] = invitation.Role{ID: id, Name: name, RequiresProviderIdentifier: requiresProviderIdentifier}
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
	users       map[string]invitation.PendingUser // by ID
	usersByMail map[string]string                 // email -> userID
	userRoles   map[string][]string               // userID -> role IDs
	roles       map[string]invitation.Role
	invitations map[string]invitation.Invitation // by ID
	byTokenHash map[string]string                // tokenHash -> invitation ID
	enrollments []invitation.EnrollmentToken
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		users: map[string]invitation.PendingUser{}, usersByMail: map[string]string{},
		userRoles: map[string][]string{}, roles: map[string]invitation.Role{},
		invitations: map[string]invitation.Invitation{}, byTokenHash: map[string]string{},
	}
}

func (r *memoryRepo) GetRolesByIDs(_ context.Context, ids []string) ([]invitation.Role, error) {
	seen := map[string]bool{}
	var out []invitation.Role
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

func (r *memoryRepo) GetUserByEmail(_ context.Context, _, email string) (*invitation.PendingUser, error) {
	id, ok := r.usersByMail[email]
	if !ok {
		return nil, invitation.ErrUserNotFound
	}
	u := r.users[id]
	return &u, nil
}

func (r *memoryRepo) GetUserByID(_ context.Context, id string) (*invitation.PendingUser, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, invitation.ErrUserNotFound
	}
	return &u, nil
}

func (r *memoryRepo) CreateInvitedUser(_ context.Context, params invitation.CreateInvitedUserParams) error {
	r.users[params.ID] = invitation.PendingUser{
		ID: params.ID, FullName: params.FullName, Email: params.Email, Status: invitation.UserStatusInvited,
	}
	r.usersByMail[params.Email] = params.ID
	return nil
}

func (r *memoryRepo) AssignUserRole(_ context.Context, userID, roleID string) error {
	r.userRoles[userID] = append(r.userRoles[userID], roleID)
	return nil
}

func (r *memoryRepo) GetUserRoleNames(_ context.Context, userID string) ([]string, error) {
	var names []string
	for _, rid := range r.userRoles[userID] {
		if role, ok := r.roles[rid]; ok {
			names = append(names, role.Name)
		}
	}
	return names, nil
}

func (r *memoryRepo) CreateInvitation(_ context.Context, inv invitation.Invitation) error {
	inv.CreatedAt = time.Now()
	r.invitations[inv.ID] = inv
	r.byTokenHash[inv.TokenHash] = inv.ID
	return nil
}

func (r *memoryRepo) GetInvitationByTokenHash(_ context.Context, tokenHash string) (*invitation.Invitation, error) {
	id, ok := r.byTokenHash[tokenHash]
	if !ok {
		return nil, invitation.ErrTokenInvalid
	}
	inv := r.invitations[id]
	return &inv, nil
}

func (r *memoryRepo) GetLatestInvitationByUserID(_ context.Context, userID string) (*invitation.Invitation, error) {
	var latest *invitation.Invitation
	for _, inv := range r.invitations {
		if inv.UserID != userID {
			continue
		}
		v := inv
		if latest == nil || v.CreatedAt.After(latest.CreatedAt) {
			latest = &v
		}
	}
	if latest == nil {
		return nil, invitation.ErrTokenInvalid
	}
	return latest, nil
}

func (r *memoryRepo) ConsumeInvitation(_ context.Context, id string) error {
	inv, ok := r.invitations[id]
	if !ok {
		return invitation.ErrTokenInvalid
	}
	now := time.Now()
	inv.UsedAt = &now
	r.invitations[id] = inv
	return nil
}

func (r *memoryRepo) ActivateUserWithPassword(_ context.Context, userID, _ string) error {
	u, ok := r.users[userID]
	if !ok {
		return invitation.ErrUserNotFound
	}
	u.Status = invitation.UserStatusActive
	r.users[userID] = u
	return nil
}

func (r *memoryRepo) CreateEnrollmentToken(_ context.Context, token invitation.EnrollmentToken) error {
	r.enrollments = append(r.enrollments, token)
	return nil
}

func (r *memoryRepo) WithinTx(_ context.Context, fn func(invitation.Repository) error) error {
	return fn(r)
}
