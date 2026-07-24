package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"nodus-health/internal/auth"
	"nodus-health/internal/auth/postgres/sqlcgen"
)

func sessionFromRow(s sqlcgen.Session) *auth.Session {
	return &auth.Session{
		ID: s.ID, TenantID: s.TenantID, UserID: s.UserID, DeviceLabel: s.DeviceLabel, IPAddress: s.IpAddress,
		UserAgent: s.UserAgent, CreatedAt: fromTimestamptz(s.CreatedAt),
		LastActiveAt: fromTimestamptz(s.LastActiveAt), RevokedAt: fromNullTimestamptz(s.RevokedAt),
	}
}

func (r *Repository) CreateSession(ctx context.Context, session auth.Session) error {
	return r.q(ctx).CreateSession(ctx, sqlcgen.CreateSessionParams{
		ID: session.ID, UserID: session.UserID, DeviceLabel: session.DeviceLabel,
		IpAddress: session.IPAddress, UserAgent: session.UserAgent,
	})
}

func (r *Repository) GetSessionByID(ctx context.Context, id string) (*auth.Session, error) {
	s, err := r.q(ctx).GetSessionByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrSessionNotFound
		}
		return nil, err
	}
	return sessionFromRow(s), nil
}

func (r *Repository) ListActiveSessionsByUser(ctx context.Context, userID string) ([]auth.Session, error) {
	rows, err := r.q(ctx).ListActiveSessionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	sessions := make([]auth.Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, *sessionFromRow(row))
	}
	return sessions, nil
}

func (r *Repository) RevokeSession(ctx context.Context, id string) error {
	return r.q(ctx).RevokeSession(ctx, id)
}

func (r *Repository) RevokeSessionsByUser(ctx context.Context, userID string) error {
	return r.q(ctx).RevokeSessionsByUser(ctx, userID)
}

func (r *Repository) RevokeSessionsByUserExceptSession(ctx context.Context, userID, exceptSessionID string) error {
	return r.q(ctx).RevokeSessionsByUserExceptSession(ctx, sqlcgen.RevokeSessionsByUserExceptSessionParams{
		UserID: userID, ID: exceptSessionID,
	})
}

func (r *Repository) TouchSessionLastActive(ctx context.Context, id string) error {
	return r.q(ctx).TouchSessionLastActive(ctx, id)
}
