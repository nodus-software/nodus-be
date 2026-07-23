package test_audit

import (
	"context"
	"net/http"
	"testing"

	"nodus-health/internal/audit"
)

func TestListAuditLogs_GoldenPath(t *testing.T) {
	env := Setup(t)
	ctx := context.Background()
	userID := "user-1"
	if err := env.Service.Record(ctx, audit.Entry{UserID: &userID, Action: "login_success", Result: audit.ResultSuccess}); err != nil {
		t.Fatal(err)
	}
	if err := env.Service.Record(ctx, audit.Entry{UserID: &userID, Action: "login_failed", Result: audit.ResultFailure}); err != nil {
		t.Fatal(err)
	}

	_, actorToken := env.NewActor(t, "audit:read")
	rec := env.JSON(t, http.MethodGet, "/audit-logs", actorToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var entries []audit.AuditLogEntryResponse
	Decode(t, rec, &entries)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestListAuditLogs_FiltersByAction(t *testing.T) {
	env := Setup(t)
	ctx := context.Background()
	userID := "user-1"
	env.Service.Record(ctx, audit.Entry{UserID: &userID, Action: "login_success", Result: audit.ResultSuccess})
	env.Service.Record(ctx, audit.Entry{UserID: &userID, Action: "login_failed", Result: audit.ResultFailure})

	_, actorToken := env.NewActor(t, "audit:read")
	rec := env.JSON(t, http.MethodGet, "/audit-logs?action=login_failed", actorToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var entries []audit.AuditLogEntryResponse
	Decode(t, rec, &entries)
	if len(entries) != 1 || entries[0].Action != "login_failed" {
		t.Fatalf("expected one login_failed entry, got %+v", entries)
	}
}

func TestListAuditLogs_FiltersByUserID(t *testing.T) {
	env := Setup(t)
	ctx := context.Background()
	userA, userB := "user-a", "user-b"
	env.Service.Record(ctx, audit.Entry{UserID: &userA, Action: "login_success", Result: audit.ResultSuccess})
	env.Service.Record(ctx, audit.Entry{UserID: &userB, Action: "login_success", Result: audit.ResultSuccess})

	_, actorToken := env.NewActor(t, "audit:read")
	rec := env.JSON(t, http.MethodGet, "/audit-logs?user_id="+userA, actorToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var entries []audit.AuditLogEntryResponse
	Decode(t, rec, &entries)
	if len(entries) != 1 || entries[0].UserID == nil || *entries[0].UserID != userA {
		t.Fatalf("expected one entry for %s, got %+v", userA, entries)
	}
}

func TestListAuditLogs_RequiresPermission(t *testing.T) {
	env := Setup(t)
	_, actorToken := env.NewActor(t) // no permissions
	rec := env.JSON(t, http.MethodGet, "/audit-logs", actorToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without audit:read, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListAuditLogs_RequiresAuthentication(t *testing.T) {
	env := Setup(t)
	rec := env.JSON(t, http.MethodGet, "/audit-logs", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a token, got %d: %s", rec.Code, rec.Body.String())
	}
}
