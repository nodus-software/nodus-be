package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nodus-health/internal/auth"
	"nodus-health/internal/platform/db"
	"nodus-health/internal/tenant"
)

func TestObserveAuthenticationFailureAtomicPolicy(t *testing.T) {
	dsn := os.Getenv("TEST_DB_URL")
	if dsn == "" {
		t.Skip("TEST_DB_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var userID, tenantID string
	if err := tx.QueryRow(ctx, `SELECT id,tenant_id FROM users LIMIT 1`).Scan(&userID, &tenantID); err != nil {
		if err == pgx.ErrNoRows {
			t.Skip("test database has no user fixture")
		}
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id',$1,true)", tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM authentication_failure_states WHERE user_id=$1`, userID); err != nil {
		t.Fatal(err)
	}
	repo := New(pool)
	repo.executor = tx
	tenantCtx := tenant.WithContext(db.WithExecutor(ctx, tx), tenant.Identity{ID: tenantID})
	policy := auth.FailurePolicy{ObservationWindow: 15 * time.Minute, CycleWindow: 24 * time.Hour, LockThreshold: 10, InitialLock: 15 * time.Minute, MaximumLock: time.Hour}
	var state *auth.AuthenticationFailureState
	for i := 1; i <= 10; i++ {
		state, err = repo.ObserveAuthenticationFailure(tenantCtx, userID, auth.AuthenticationMechanismPassword, policy)
		if err != nil {
			t.Fatalf("failure %d: %v", i, err)
		}
		if state.FailureCount != i {
			t.Fatalf("failure count=%d, want %d", state.FailureCount, i)
		}
	}
	if state.LockedUntil == nil || state.LockCycleCount != 1 {
		t.Fatalf("expected first adaptive restriction, got %+v", state)
	}
	mfa, err := repo.ObserveAuthenticationFailure(tenantCtx, userID, auth.AuthenticationMechanismMFA, policy)
	if err != nil || mfa.FailureCount != 1 {
		t.Fatalf("independent MFA state: %+v err=%v", mfa, err)
	}
}
