package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"nodus-health/internal/clinical/icd11"
	"nodus-health/internal/platform/db"
)

func main() {
	file := flag.String("file", "", "path to WHO ICD-11 MMS simple-tabulation workbook")
	commit := flag.Bool("commit", false, "commit the validated release")
	flag.Parse()
	if *file == "" {
		log.Fatal("--file is required")
	}
	wb, err := icd11.ParseFile(*file)
	if err != nil {
		log.Fatalf("validate workbook: %v", err)
	}
	fmt.Printf("validated ICD-11 %s: %d source rows, %d standalone concepts, sha256=%s\n", wb.Version, wb.TotalRows, len(wb.Concepts), wb.Checksum)
	if !*commit {
		fmt.Println("dry run only; pass --commit to import")
		return
	}
	dsn := os.Getenv("MIGRATION_DB_URL")
	if dsn == "" {
		log.Fatal("MIGRATION_DB_URL is required with --commit")
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err = icd11.Commit(ctx, pool, wb); err != nil {
		log.Fatalf("import ICD-11: %v", err)
	}
	fmt.Printf("activated ICD-11 %s with %d concepts\n", wb.Version, len(wb.Concepts))
}
