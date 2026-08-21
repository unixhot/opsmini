// Package web embeds frontend assets into the binary via go:embed to enable single-file delivery.
package web

import "embed"

// FS is the frontend asset filesystem (index.html + static/).
//
//go:embed index.html error.html static
var FS embed.FS
