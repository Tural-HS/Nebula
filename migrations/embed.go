// Package migrations embeds the SQL schema migrations into the binary so a
// release always ships with exactly the schema it was built and tested against.
package migrations

import "embed"

// FS holds the embedded migration files.
//
//go:embed *.sql
var FS embed.FS
