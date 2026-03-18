package webui

import "embed"

// distFS contains the built frontend assets copied into dist.
//
//go:embed all:dist
var distFS embed.FS
