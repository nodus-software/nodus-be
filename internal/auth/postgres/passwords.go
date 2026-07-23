package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"nodus-health/internal/auth"
	"nodus-health/internal/auth/postgres/sqlcgen"
)

func (r *Repository) UpdatePasswordHash(ctx context.Context, userID, hash string) error {
	return r.queries.UpdatePasswordHash(ctx, sqlcgen.UpdatePasswordHashParams{ID: userID, PasswordHash: &hash})
}

func (r *Repository) CreatePasswordResetToken(ctx context.Context, token auth.PasswordResetToken) error {
	return r.queries.CreatePasswordResetToken(ctx, sqlcgen.CreatePasswordResetTokenParams{
		ID: token.ID, UserID: token.UserID, TokenHash: token.TokenHash,
		ExpiresAt: toTimestamptz(token.ExpiresAt),
	})
}

func (r *Repository) GetPasswordResetTokenByHash(ctx context.Context, tokenHash string) (*auth.PasswordResetToken, error) {
	t, err := r.queries.GetPasswordResetTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrResetTokenInvalid
		}
		return nil, err
	}
	return &auth.PasswordResetToken{
		ID: t.ID, UserID: t.UserID, TokenHash: t.TokenHash,
		ExpiresAt: fromTimestamptz(t.ExpiresAt), UsedAt: fromNullTimestamptz(t.UsedAt),
		CreatedAt: fromTimestamptz(t.CreatedAt),
	}, nil
}

func (r *Repository) ConsumePasswordResetToken(ctx context.Context, id string) error {
	return r.queries.ConsumePasswordResetToken(ctx, id)
}

func (r *Repository) InvalidateOtherPasswordResetTokens(ctx context.Context, userID, exceptID string) error {
	return r.queries.InvalidateOtherPasswordResetTokens(ctx, sqlcgen.InvalidateOtherPasswordResetTokensParams{
		UserID: userID, ID: exceptID,
	})
}

func (r *Repository) RecordPasswordResetAttempt(ctx context.Context, id, usernameAttempted, ip string) error {
	return r.queries.RecordPasswordResetAttempt(ctx, sqlcgen.RecordPasswordResetAttemptParams{
		ID: id, UsernameAttempted: usernameAttempted, IpAddress: ip,
	})
}

func (r *Repository) CountPasswordResetAttemptsByUsername(ctx context.Context, username string, since time.Time) (int, error) {
	n, err := r.queries.CountPasswordResetAttemptsByUsername(ctx, sqlcgen.CountPasswordResetAttemptsByUsernameParams{
		UsernameAttempted: username, CreatedAt: toTimestamptz(since),
	})
	return int(n), err
}

func (r *Repository) CountPasswordResetAttemptsByIP(ctx context.Context, ip string, since time.Time) (int, error) {
	n, err := r.queries.CountPasswordResetAttemptsByIP(ctx, sqlcgen.CountPasswordResetAttemptsByIPParams{
		IpAddress: ip, CreatedAt: toTimestamptz(since),
	})
	return int(n), err
}
