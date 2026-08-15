package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool opens and validates a pgx connection pool against dsn.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse db config: %w", err)
	}

	// Do not retain server-side prepared plans across online schema migrations.
	// PostgreSQL cannot reuse a prepared result descriptor after a migration
	// replaces a returned enum/domain type (SQLSTATE 0A000). Exec mode keeps the
	// normal binary-safe pgx encoding while allowing every execution to observe
	// the current schema without requiring an application restart.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create db pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return pool, nil
}

// ValidateTenantRuntimeRole prevents deployments from silently disabling every
// tenant RLS policy by connecting the API as a PostgreSQL superuser or a role
// with BYPASSRLS. Explicit tenant predicates remain required in repositories;
// this is the independent database-level backstop.
func ValidateTenantRuntimeRole(ctx context.Context, pool *pgxpool.Pool) error {
	var role string
	var superuser, bypassRLS bool
	if err := pool.QueryRow(ctx, `SELECT rolname, rolsuper, rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&role, &superuser, &bypassRLS); err != nil {
		return fmt.Errorf("inspect database runtime role: %w", err)
	}
	if superuser || bypassRLS {
		return fmt.Errorf("database runtime role %q must be NOSUPERUSER and NOBYPASSRLS", role)
	}
	return nil
}
