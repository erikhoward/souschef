// Package migrations embeds the SQL migration files so they can be applied
// by internal/store without reaching outside its own package directory —
// Go's embed directive cannot reference paths above the package it's in.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
