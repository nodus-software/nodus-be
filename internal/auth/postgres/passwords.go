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
	return r.q(ctx).UpdatePasswordHash(ctx, sqlcgen.UpdatePasswordHashParams{ID: userID, PasswordHash: &hash})
}

func (r *Repository) CreatePasswordResetToken(ctx context.Context, token auth.PasswordResetToken) error {
	return r.q(ctx).CreatePasswordResetToken(ctx, sqlcgen.CreatePasswordResetTokenParams{
		ID: token.ID, UserID: token.UserID, TokenHash: token.TokenHash,
		ExpiresAt: toTimestamptz(token.ExpiresAt),
	})
}

func (r *Repository) GetPasswordResetTokenByHash(ctx context.Context, tokenHash string) (*auth.PasswordResetToken, error) {
	t, err := r.q(ctx).GetPasswordResetTokenByHash(ctx, tokenHash)
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
	return r.q(ctx).ConsumePasswordResetToken(ctx, id)
}

func (r *Repository) InvalidateOtherPasswordResetTokens(ctx context.Context, userID, exceptID string) error {
	return r.q(ctx).InvalidateOtherPasswordResetTokens(ctx, sqlcgen.InvalidateOtherPasswordResetTokensParams{
		UserID: userID, ID: exceptID,
	})
}

func (r *Repository) RecordPasswordResetAttempt(ctx context.Context, id, usernameAttempted, ip string) error {
	return r.q(ctx).RecordPasswordResetAttempt(ctx, sqlcgen.RecordPasswordResetAttemptParams{
		ID: id, UsernameAttempted: usernameAttempted, IpAddress: ip,
	})
}

func (r *Repository) CountPasswordResetAttemptsByUsername(ctx context.Context, username string, since time.Time) (int, error) {
	n, err := r.q(ctx).CountPasswordResetAttemptsByUsername(ctx, sqlcgen.CountPasswordResetAttemptsByUsernameParams{
		UsernameAttempted: username, CreatedAt: toTimestamptz(since),
	})
	return int(n), err
}

func (r *Repository) CountPasswordResetAttemptsByIP(ctx context.Context, ip string, since time.Time) (int, error) {
	n, err := r.q(ctx).CountPasswordResetAttemptsByIP(ctx, sqlcgen.CountPasswordResetAttemptsByIPParams{
		IpAddress: ip, CreatedAt: toTimestamptz(since),
	})
	return int(n), err
}

func (r *Repository) InvalidateRecoveryEmailTokens(ctx context.Context, userID string, intent auth.RecoveryIntent) error {
	return r.q(ctx).InvalidateRecoveryEmailTokens(ctx, sqlcgen.InvalidateRecoveryEmailTokensParams{UserID: userID, Intent: sqlcgen.RecoveryIntent(intent)})
}

func (r *Repository) CreateRecoveryEmailToken(ctx context.Context, token auth.RecoveryEmailToken, tokenHash string) error {
	return r.q(ctx).CreateRecoveryEmailToken(ctx, sqlcgen.CreateRecoveryEmailTokenParams{ID: token.ID, UserID: token.UserID, Intent: sqlcgen.RecoveryIntent(token.Intent), TokenHash: tokenHash, ExpiresAt: toTimestamptz(token.ExpiresAt)})
}

func (r *Repository) ConsumeRecoveryEmailToken(ctx context.Context, tokenHash string) (*auth.RecoveryEmailToken, error) {
	t, err := r.q(ctx).ConsumeRecoveryEmailToken(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrRecoveryTokenInvalid
		}
		return nil, err
	}
	return &auth.RecoveryEmailToken{ID: t.ID, UserID: t.UserID, Intent: auth.RecoveryIntent(t.Intent), ExpiresAt: fromTimestamptz(t.ExpiresAt)}, nil
}

func (r *Repository) CreateRecoverySession(ctx context.Context, session auth.RecoverySession, tokenHash string) error {
	return r.q(ctx).CreateRecoverySession(ctx, sqlcgen.CreateRecoverySessionParams{ID: session.ID, UserID: session.UserID, TokenHash: tokenHash, CanResetPassword: session.CanResetPassword, CanReplaceMfa: session.CanReplaceMFA, ExpiresAt: toTimestamptz(session.ExpiresAt)})
}

func recoverySessionFromRow(s sqlcgen.RecoverySession) *auth.RecoverySession {
	return &auth.RecoverySession{ID: s.ID, UserID: s.UserID, CanResetPassword: s.CanResetPassword, CanReplaceMFA: s.CanReplaceMfa, PasswordCompletedAt: fromNullTimestamptz(s.PasswordCompletedAt), MFACompletedAt: fromNullTimestamptz(s.MfaCompletedAt), ConsumedAt: fromNullTimestamptz(s.ConsumedAt), FailedAttempts: int(s.FailedAttempts), ExpiresAt: fromTimestamptz(s.ExpiresAt)}
}

func (r *Repository) GetRecoverySessionByHash(ctx context.Context, tokenHash string) (*auth.RecoverySession, error) {
	s, err := r.q(ctx).GetRecoverySessionByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrRecoveryTokenInvalid
		}
		return nil, err
	}
	return recoverySessionFromRow(s), nil
}
func (r *Repository) IncrementRecoverySessionFailure(ctx context.Context, id string) (int, error) {
	n, err := r.q(ctx).IncrementRecoverySessionFailure(ctx, id)
	return int(n), err
}
func (r *Repository) CompleteRecoveryPassword(ctx context.Context, id string) error {
	n, err := r.q(ctx).CompleteRecoveryPassword(ctx, id)
	if err == nil && n == 0 {
		return auth.ErrRecoveryTokenInvalid
	}
	return err
}
func (r *Repository) CompleteRecoveryMFA(ctx context.Context, id string) error {
	n, err := r.q(ctx).CompleteRecoveryMFA(ctx, id)
	if err == nil && n == 0 {
		return auth.ErrRecoveryTokenInvalid
	}
	return err
}
func (r *Repository) InvalidateRecoverySessionsByUser(ctx context.Context, userID, exceptID string) error {
	return r.q(ctx).InvalidateRecoverySessionsByUser(ctx, sqlcgen.InvalidateRecoverySessionsByUserParams{UserID: userID, ID: exceptID})
}
