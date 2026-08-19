package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"

	"github.com/jackc/pgx/v5"
)

var roleNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func main() {
	dsn := os.Getenv("MIGRATION_DB_URL")
	role := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	if dsn == "" || !roleNamePattern.MatchString(role) || password == "" {
		log.Fatal("MIGRATION_DB_URL, DB_USER, and DB_PASSWORD are required")
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer conn.Close(ctx)

	var exists bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, role).Scan(&exists); err != nil {
		log.Fatalf("inspect runtime role: %v", err)
	}
	identifier := pgx.Identifier{role}.Sanitize()
	if !exists {
		if _, err := conn.Exec(ctx, `CREATE ROLE `+identifier+` LOGIN`); err != nil {
			log.Fatalf("create runtime role: %v", err)
		}
	}

	statements := []string{
		fmt.Sprintf(`ALTER ROLE %s PASSWORD %s NOSUPERUSER NOBYPASSRLS`, identifier, quoteLiteral(password)),
		`GRANT USAGE ON SCHEMA public TO ` + identifier,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO ` + identifier,
		`GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO ` + identifier,
		`GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO ` + identifier,
		`ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ` + identifier,
		`ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO ` + identifier,
		`ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT EXECUTE ON FUNCTIONS TO ` + identifier,
	}
	for _, statement := range statements {
		if _, err := conn.Exec(ctx, statement); err != nil {
			log.Fatalf("configure runtime role: %v", err)
		}
	}

	log.Printf("runtime database role %q configured", role)
}

func quoteLiteral(value string) string {
	return "'" + regexp.MustCompile(`'`).ReplaceAllString(value, "''") + "'"
}
