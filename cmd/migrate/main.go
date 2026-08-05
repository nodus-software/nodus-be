package main

import (
	"log"
	"os"

	platformmigrations "nodus-health/internal/platform/migrations"
)

func main() {
	dsn := os.Getenv("MIGRATION_DB_URL")
	if dsn == "" {
		log.Fatal("MIGRATION_DB_URL is required")
	}

	if err := platformmigrations.Up(dsn); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	log.Println("database migrations are up to date")
}
