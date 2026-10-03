// Package db embeds the goose SQL migrations so the binary can migrate itself.
package db

import "embed"

// Migrations holds the goose migration files.
//
//go:embed migrations/*.sql
var Migrations embed.FS
