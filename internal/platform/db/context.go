package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX matches the common interface emitted by sqlc without importing any
// domain package (which would introduce an import cycle).
type DBTX interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}

type contextKey struct{}

func WithExecutor(ctx context.Context, executor DBTX) context.Context {
	return context.WithValue(ctx, contextKey{}, executor)
}

func Executor(ctx context.Context) (DBTX, bool) {
	executor, ok := ctx.Value(contextKey{}).(DBTX)
	return executor, ok
}
