package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nodus-health/internal/tenant"
	"nodus-health/pkg/logger"
)

type recordingTenantResolver struct {
	called bool
}

func (r *recordingTenantResolver) ResolveTenant(context.Context, string) (tenant.Identity, error) {
	r.called = true
	return tenant.Identity{}, nil
}

func TestHealth(t *testing.T) {
	resolver := &recordingTenantResolver{}
	srv := New(Config{
		RequestTimeout: time.Second,
		TenantResolver: resolver,
	}, logger.NewLogger())

	recorder := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if resolver.called {
		t.Fatal("health endpoint must bypass tenant resolution")
	}

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Success || body.Data.Status != "ok" {
		t.Fatalf("body = %+v, want successful health response", body)
	}
}
