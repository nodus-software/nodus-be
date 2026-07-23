package test_audit

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
	"nodus-health/pkg/logger"
	"nodus-health/pkg/security"
	"nodus-health/pkg/utility"
)

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
	Service    *audit.Service
	authorizer *stubAuthorizer
}

func Setup(t *testing.T) *Env {
	t.Helper()
	repo := newMemoryRepo()
	service := audit.NewService(repo)
	authorizer := &stubAuthorizer{permissions: map[string][]string{}}
	handler := audit.NewHandler(service, authorizer, testJWTSecret, logger.NewLogger())
	r := chi.NewRouter()
	handler.RegisterRoutes(r)
	return &Env{Router: r, Service: service, authorizer: authorizer}
}

func (e *Env) NewActor(t *testing.T, permissions ...string) (userID, accessToken string) {
	t.Helper()
	id, err := utility.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	e.authorizer.grant(id, permissions)
	token, _, err := security.IssueAccessToken(testJWTSecret, time.Hour, id, "session")
	if err != nil {
		t.Fatal(err)
	}
	return id, token
}

func (e *Env) JSON(t *testing.T, method, path, accessToken string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(""))
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
	entries []audit.Entry
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{}
}

func (r *memoryRepo) Insert(_ context.Context, entry audit.Entry) error {
	r.Lock()
	defer r.Unlock()
	r.entries = append(r.entries, entry)
	return nil
}

func (r *memoryRepo) List(_ context.Context, filter audit.Filter, limit int) ([]audit.Entry, error) {
	r.Lock()
	defer r.Unlock()
	var out []audit.Entry
	for i := len(r.entries) - 1; i >= 0; i-- {
		e := r.entries[i]
		if filter.UserID != nil && (e.UserID == nil || *e.UserID != *filter.UserID) {
			continue
		}
		if filter.Action != nil && e.Action != *filter.Action {
			continue
		}
		if filter.From != nil && e.Timestamp.Before(*filter.From) {
			continue
		}
		if filter.To != nil && e.Timestamp.After(*filter.To) {
			continue
		}
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
