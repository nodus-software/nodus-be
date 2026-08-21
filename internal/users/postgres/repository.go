package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nodus-health/internal/email"
	"nodus-health/internal/platform/db"
	"nodus-health/internal/users"
	"nodus-health/internal/users/postgres/sqlcgen"
)

type Repository struct {
	queries  *sqlcgen.Queries
	pool     *pgxpool.Pool
	executor sqlcgen.DBTX
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{queries: sqlcgen.New(pool), pool: pool, executor: pool}
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
		return fn(&Repository{queries: sqlcgen.New(executor), pool: r.pool, executor: executor})
	}
	return db.RunInTx(ctx, r.pool, func(tx pgx.Tx) *Repository {
		return &Repository{queries: sqlcgen.New(tx), pool: r.pool, executor: tx}
	}, func(txRepo *Repository) error {
		return fn(txRepo)
	})
}

func (r *Repository) QueueEmail(ctx context.Context, message email.Message) error {
	executor := r.executor
	if tx, ok := db.Executor(ctx); ok {
		executor = tx
	}
	return email.Enqueue(ctx, executor, message)
}
