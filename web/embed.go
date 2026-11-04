package web

import "embed"

// Files holds the single-page UI served from the same binary.
//
//go:embed index.html app.js style.css
var Files embed.FS
