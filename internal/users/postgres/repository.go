package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nodus-health/internal/platform/db"
	"nodus-health/internal/users"
	"nodus-health/internal/users/postgres/sqlcgen"
)

type Repository struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{queries: sqlcgen.New(pool), pool: pool}
}

func (r *Repository) q(ctx context.Context) *sqlcgen.Queries {
	if executor, ok := db.Executor(ctx); ok {
		return sqlcgen.New(executor)
	}
	return r.queries
}

// WithinTx runs fn against a Repository bound to a single transaction.
func (r *Repository) WithinTx(ctx context.Context, fn func(users.Repository) error) error {
	if executor, ok := db.Executor(ctx); ok {
		return fn(&Repository{queries: sqlcgen.New(executor), pool: r.pool})
	}
	return db.RunInTx(ctx, r.pool, func(tx pgx.Tx) *Repository {
		return &Repository{queries: sqlcgen.New(tx), pool: r.pool}
	}, func(txRepo *Repository) error {
		return fn(txRepo)
	})
}
