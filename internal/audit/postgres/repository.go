package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"nodus-health/internal/audit"
	"nodus-health/internal/audit/postgres/sqlcgen"
)

type Repository struct {
	queries *sqlcgen.Queries
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{queries: sqlcgen.New(pool)}
}

func (r *Repository) Insert(ctx context.Context, entry audit.Entry) error {
	metadata, err := json.Marshal(entry.Metadata)
	if err != nil {
		return err
	}

	return r.queries.InsertAuditLog(ctx, sqlcgen.InsertAuditLogParams{
		ID:             entry.ID,
		Timestamp:      pgtype.Timestamptz{Time: entry.Timestamp, Valid: true},
		UserID:         entry.UserID,
		Action:         entry.Action,
		TargetResource: entry.TargetResource,
		IpAddress:      entry.IPAddress,
		Result:         sqlcgen.AuditResult(entry.Result),
		Metadata:       metadata,
	})
}
