// Package web embeds the built React app so it can be served by
// internal/httpapi without reaching outside its own package directory —
// Go's embed directive cannot reference paths above the package it's in.
package web

import "embed"

//go:embed all:dist
var DistFS embed.FS
