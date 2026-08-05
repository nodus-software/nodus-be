// Package migrations exposes the SQL migration files to the migration binary.
package migrations

import "embed"

// Files is embedded so a deployment always runs the migrations that were
// built into the exact same image as the API it is about to start.
//
//go:embed *.up.sql *.down.sql
var Files embed.FS
