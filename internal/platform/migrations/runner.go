package migrations

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	pgxdriver "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"

	migrationfiles "nodus-health/migrations"
)

// Up applies every pending embedded migration. The pgx migration driver keeps
// the application and migration paths on the same PostgreSQL client stack.
func Up(dsn string) error {
	if dsn == "" {
		return errors.New("migration database URL is empty")
	}

	source, err := iofs.New(migrationfiles.Files, ".")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}

	database, err := sql.Open("pgx", dsn)
	if err != nil {
		_ = source.Close()
		return fmt.Errorf("open migration database: %w", err)
	}

	driver, err := pgxdriver.WithInstance(database, &pgxdriver.Config{})
	if err != nil {
		_ = source.Close()
		_ = database.Close()
		return fmt.Errorf("initialize migration database driver: %w", err)
	}

	runner, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		_ = source.Close()
		_ = driver.Close()
		return fmt.Errorf("initialize migrator: %w", err)
	}

	err = runner.Up()
	sourceErr, databaseErr := runner.Close()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	if sourceErr != nil {
		return fmt.Errorf("close migration source: %w", sourceErr)
	}
	if databaseErr != nil {
		return fmt.Errorf("close migration database: %w", databaseErr)
	}

	return nil
}
