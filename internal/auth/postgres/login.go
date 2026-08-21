package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"nodus-health/internal/auth"
	"nodus-health/internal/auth/postgres/sqlcgen"
	"nodus-health/internal/platform/db"
	"nodus-health/internal/tenant"
)

func userFromRow(u sqlcgen.User) *auth.User {
	var passwordHash string
	if u.PasswordHash != nil {
		passwordHash = *u.PasswordHash
	}
	return &auth.User{
		ID: u.ID, TenantID: u.TenantID, FullName: u.FullName, Username: u.Username, Email: u.Email,
		PasswordHash: passwordHash, ProviderIdentifier: u.ProviderIdentifier,
		Status:              auth.UserStatus(u.Status),
		FailedLoginAttempts: int(u.FailedLoginAttempts),
		LockedUntil:         fromNullTimestamptz(u.LockedUntil),
		PasswordChangedAt:   fromTimestamptz(u.PasswordChangedAt),
		LastAccessReviewAt:  fromNullTimestamptz(u.LastAccessReviewAt),
		NextAccessReviewDue: fromNullTimestamptz(u.NextAccessReviewDue),
		CreatedAt:           fromTimestamptz(u.CreatedAt),
		UpdatedAt:           fromTimestamptz(u.UpdatedAt),
	}
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*auth.User, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	u, err := r.q(ctx).GetUserByUsername(ctx, sqlcgen.GetUserByUsernameParams{
		TenantID: tenantID,
		Username: username,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrUserNotFound
		}
		return nil, err
	}
	return userFromRow(u), nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*auth.User, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	u, err := r.q(ctx).GetUserByEmail(ctx, sqlcgen.GetUserByEmailParams{
		TenantID: tenantID,
		Email:    email,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrUserNotFound
		}
		return nil, err
	}
	return userFromRow(u), nil
}

func (r *Repository) GetUserByID(ctx context.Context, id string) (*auth.User, error) {
	tenantID, err := tenant.ID(ctx)
	if err != nil {
		return nil, err
	}
	u, err := r.q(ctx).GetUserByID(ctx, sqlcgen.GetUserByIDParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrUserNotFound
		}
		return nil, err
	}
	return userFromRow(u), nil
}

func (r *Repository) IncrementFailedLoginAttempts(ctx context.Context, userID string) (int, error) {
	n, err := r.q(ctx).IncrementFailedLoginAttempts(ctx, userID)
	return int(n), err
}

func (r *Repository) LockUser(ctx context.Context, userID string, until time.Time) error {
	return r.q(ctx).LockUser(ctx, sqlcgen.LockUserParams{ID: userID, LockedUntil: toTimestamptz(until)})
}

func (r *Repository) ResetFailedLoginAttempts(ctx context.Context, userID string) error {
	return r.q(ctx).ResetFailedLoginAttempts(ctx, userID)
}

func (r *Repository) ObserveAuthenticationFailure(ctx context.Context, userID string, mechanism auth.AuthenticationMechanism, policy auth.FailurePolicy) (*auth.AuthenticationFailureState, error) {
	const query = `
INSERT INTO authentication_failure_states(user_id, mechanism, failure_count, window_started_at, last_failure_at)
VALUES ($1,$2,1,now(),now())
ON CONFLICT (tenant_id,user_id,mechanism) DO UPDATE SET
  failure_count=CASE WHEN authentication_failure_states.window_started_at <= now()-$3::interval THEN 1 ELSE authentication_failure_states.failure_count+1 END,
  window_started_at=CASE WHEN authentication_failure_states.window_started_at <= now()-$3::interval THEN now() ELSE authentication_failure_states.window_started_at END,
  last_failure_at=now(),
  next_attempt_at=CASE
    WHEN (CASE WHEN authentication_failure_states.window_started_at <= now()-$3::interval THEN 1 ELSE authentication_failure_states.failure_count+1 END) BETWEEN 6 AND 9
    THEN now()+make_interval(secs => LEAST(30,power(2,(CASE WHEN authentication_failure_states.window_started_at <= now()-$3::interval THEN 1 ELSE authentication_failure_states.failure_count+1 END)-5)::int))
    ELSE NULL END,
  locked_until=CASE
    WHEN (CASE WHEN authentication_failure_states.window_started_at <= now()-$3::interval THEN 1 ELSE authentication_failure_states.failure_count+1 END) = $5
    THEN now()+LEAST($7::interval, $6::interval * power(2,CASE WHEN authentication_failure_states.cycle_window_start IS NULL OR authentication_failure_states.cycle_window_start <= now()-$4::interval THEN 0 ELSE authentication_failure_states.lock_cycle_count END)::double precision)
    ELSE authentication_failure_states.locked_until END,
  lock_cycle_count=CASE
    WHEN (CASE WHEN authentication_failure_states.window_started_at <= now()-$3::interval THEN 1 ELSE authentication_failure_states.failure_count+1 END) = $5
    THEN (CASE WHEN authentication_failure_states.cycle_window_start IS NULL OR authentication_failure_states.cycle_window_start <= now()-$4::interval THEN 0 ELSE authentication_failure_states.lock_cycle_count END)+1
    WHEN authentication_failure_states.cycle_window_start <= now()-$4::interval THEN 0
    ELSE authentication_failure_states.lock_cycle_count END,
	  cycle_window_start=CASE
	    WHEN (CASE WHEN authentication_failure_states.window_started_at <= now()-$3::interval THEN 1 ELSE authentication_failure_states.failure_count+1 END) = $5
	      AND (authentication_failure_states.cycle_window_start IS NULL OR authentication_failure_states.cycle_window_start <= now()-$4::interval) THEN now()
	    ELSE authentication_failure_states.cycle_window_start END
RETURNING mechanism,failure_count,window_started_at,last_failure_at,next_attempt_at,locked_until,lock_cycle_count,cycle_window_start`
	window := policy.ObservationWindow.String()
	cycle := policy.CycleWindow.String()
	initial := policy.InitialLock.String()
	maximum := policy.MaximumLock.String()
	var state auth.AuthenticationFailureState
	executor := r.executor
	if tx, ok := db.Executor(ctx); ok {
		executor = tx
	}
	err := executor.QueryRow(ctx, query, userID, string(mechanism), window, cycle, policy.LockThreshold, initial, maximum).Scan(
		&state.Mechanism, &state.FailureCount, &state.WindowStartedAt, &state.LastFailureAt,
		&state.NextAttemptAt, &state.LockedUntil, &state.LockCycleCount, &state.CycleWindowStart,
	)
	return &state, err
}

func (r *Repository) GetAuthenticationFailure(ctx context.Context, userID string, mechanism auth.AuthenticationMechanism) (*auth.AuthenticationFailureState, error) {
	const query = `SELECT mechanism,failure_count,window_started_at,last_failure_at,next_attempt_at,locked_until,lock_cycle_count,cycle_window_start
FROM authentication_failure_states WHERE user_id=$1 AND mechanism=$2`
	executor := r.executor
	if tx, ok := db.Executor(ctx); ok {
		executor = tx
	}
	var state auth.AuthenticationFailureState
	err := executor.QueryRow(ctx, query, userID, string(mechanism)).Scan(
		&state.Mechanism, &state.FailureCount, &state.WindowStartedAt, &state.LastFailureAt,
		&state.NextAttemptAt, &state.LockedUntil, &state.LockCycleCount, &state.CycleWindowStart,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &state, err
}

func (r *Repository) ResetAuthenticationFailures(ctx context.Context, userID string) error {
	executor := r.executor
	if tx, ok := db.Executor(ctx); ok {
		executor = tx
	}
	_, err := executor.Exec(ctx, `DELETE FROM authentication_failure_states WHERE user_id=$1`, userID)
	return err
}

func (r *Repository) CreateLoginChallenge(ctx context.Context, challenge auth.LoginChallenge) error {
	return r.q(ctx).CreateLoginChallenge(ctx, sqlcgen.CreateLoginChallengeParams{
		ID: challenge.ID, UserID: challenge.UserID,
		ChallengeTokenHash: challenge.ChallengeTokenHash,
		ExpiresAt:          toTimestamptz(challenge.ExpiresAt),
	})
}

func (r *Repository) GetLoginChallengeByHash(ctx context.Context, tokenHash string) (*auth.LoginChallenge, error) {
	c, err := r.q(ctx).GetLoginChallengeByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrChallengeInvalid
		}
		return nil, err
	}
	return &auth.LoginChallenge{
		ID: c.ID, UserID: c.UserID, ChallengeTokenHash: c.ChallengeTokenHash,
		ExpiresAt: fromTimestamptz(c.ExpiresAt), ConsumedAt: fromNullTimestamptz(c.ConsumedAt),
		CreatedAt: fromTimestamptz(c.CreatedAt),
	}, nil
}

func (r *Repository) ConsumeLoginChallenge(ctx context.Context, id string) error {
	return r.q(ctx).ConsumeLoginChallenge(ctx, id)
}
