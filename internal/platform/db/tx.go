package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RunInTx begins a transaction on pool, constructs a domain-specific querier
// bound to that transaction via newQ, and runs fn with it — committing on
// nil and rolling back otherwise (including on panic). This lets a service
// compose several of a domain's own repository operations as a single
// atomic unit without any domain package needing to know about pgx.Tx
// itself.
func RunInTx[Q any](ctx context.Context, pool *pgxpool.Pool, newQ func(pgx.Tx) Q, fn func(Q) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(newQ(tx)); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("%w (rollback failed: %v)", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
