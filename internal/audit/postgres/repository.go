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

func (r *Repository) List(ctx context.Context, filter audit.Filter, limit int) ([]audit.Entry, error) {
	var from, to pgtype.Timestamptz
	if filter.From != nil {
		from = pgtype.Timestamptz{Time: *filter.From, Valid: true}
	}
	if filter.To != nil {
		to = pgtype.Timestamptz{Time: *filter.To, Valid: true}
	}

	rows, err := r.queries.ListAuditLogs(ctx, sqlcgen.ListAuditLogsParams{
		Limit: int32(limit), UserID: filter.UserID, Action: filter.Action, FromTs: from, ToTs: to,
	})
	if err != nil {
		return nil, err
	}

	out := make([]audit.Entry, 0, len(rows))
	for _, row := range rows {
		var metadata map[string]any
		if len(row.Metadata) > 0 {
			if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
				return nil, err
			}
		}
		out = append(out, audit.Entry{
			ID: row.ID, Timestamp: row.Timestamp.Time, UserID: row.UserID, Action: row.Action,
			TargetResource: row.TargetResource, IPAddress: row.IpAddress,
			Result: audit.Result(row.Result), Metadata: metadata,
		})
	}
	return out, nil
}
