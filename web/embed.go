package web

import "embed"

// Files holds the single-page UI served from the same binary.
//
//go:embed index.html app.js i18n.js style.css themes/claude.css themes/plain.css manifest.webmanifest sw.js icon.svg
var Files embed.FS
