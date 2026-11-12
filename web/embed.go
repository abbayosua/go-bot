package web

import "embed"

// Files holds the single-page UI served from the same binary.
//
//go:embed index.html app.js bootstrap.js i18n.js style.css themes/claude.css themes/plain.css themes/registry.js components vendor manifest.webmanifest sw.js icon.svg
var Files embed.FS
