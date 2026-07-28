package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"nodus-health/internal/auth"
	"nodus-health/internal/auth/postgres/sqlcgen"
)

func (r *Repository) CreateRefreshToken(ctx context.Context, token auth.RefreshToken) error {
	return r.q(ctx).CreateRefreshToken(ctx, sqlcgen.CreateRefreshTokenParams{
		ID: token.ID, SessionID: token.SessionID, UserID: token.UserID,
		TokenHash: token.TokenHash, ExpiresAt: toTimestamptz(token.ExpiresAt),
	})
}

func (r *Repository) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*auth.RefreshToken, error) {
	t, err := r.q(ctx).GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrRefreshTokenInvalid
		}
		return nil, err
	}
	return &auth.RefreshToken{
		ID: t.ID, SessionID: t.SessionID, UserID: t.UserID, TokenHash: t.TokenHash,
		ExpiresAt: fromTimestamptz(t.ExpiresAt), RevokedAt: fromNullTimestamptz(t.RevokedAt),
		CreatedAt: fromTimestamptz(t.CreatedAt),
	}, nil
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, id string) error {
	rows, err := r.q(ctx).RevokeRefreshToken(ctx, id)
	if err != nil {
		return err
	}
	if rows != 1 {
		return auth.ErrRefreshTokenRevoked
	}
	return nil
}

func (r *Repository) RevokeRefreshTokensByUser(ctx context.Context, userID string) error {
	return r.q(ctx).RevokeRefreshTokensByUser(ctx, userID)
}

func (r *Repository) RevokeRefreshTokensByUserExceptSession(ctx context.Context, userID, exceptSessionID string) error {
	return r.q(ctx).RevokeRefreshTokensByUserExceptSession(ctx, sqlcgen.RevokeRefreshTokensByUserExceptSessionParams{
		UserID: userID, SessionID: exceptSessionID,
	})
}

func (r *Repository) RevokeRefreshTokensBySession(ctx context.Context, sessionID string) error {
	return r.q(ctx).RevokeRefreshTokensBySession(ctx, sessionID)
}
