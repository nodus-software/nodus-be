package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"nodus-health/internal/auth"
	"nodus-health/internal/auth/postgres/sqlcgen"
)

func userFromRow(u sqlcgen.User) *auth.User {
	return &auth.User{
		ID: u.ID, FullName: u.FullName, Username: u.Username, Email: u.Email,
		PasswordHash: u.PasswordHash, ProviderIdentifier: u.ProviderIdentifier,
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
	u, err := r.queries.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrUserNotFound
		}
		return nil, err
	}
	return userFromRow(u), nil
}

func (r *Repository) GetUserByID(ctx context.Context, id string) (*auth.User, error) {
	u, err := r.queries.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrUserNotFound
		}
		return nil, err
	}
	return userFromRow(u), nil
}

func (r *Repository) IncrementFailedLoginAttempts(ctx context.Context, userID string) (int, error) {
	n, err := r.queries.IncrementFailedLoginAttempts(ctx, userID)
	return int(n), err
}

func (r *Repository) LockUser(ctx context.Context, userID string, until time.Time) error {
	return r.queries.LockUser(ctx, sqlcgen.LockUserParams{ID: userID, LockedUntil: toTimestamptz(until)})
}

func (r *Repository) ResetFailedLoginAttempts(ctx context.Context, userID string) error {
	return r.queries.ResetFailedLoginAttempts(ctx, userID)
}

func (r *Repository) CreateLoginChallenge(ctx context.Context, challenge auth.LoginChallenge) error {
	return r.queries.CreateLoginChallenge(ctx, sqlcgen.CreateLoginChallengeParams{
		ID: challenge.ID, UserID: challenge.UserID,
		ChallengeTokenHash: challenge.ChallengeTokenHash,
		ExpiresAt:          toTimestamptz(challenge.ExpiresAt),
	})
}

func (r *Repository) GetLoginChallengeByHash(ctx context.Context, tokenHash string) (*auth.LoginChallenge, error) {
	c, err := r.queries.GetLoginChallengeByHash(ctx, tokenHash)
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
	return r.queries.ConsumeLoginChallenge(ctx, id)
}
