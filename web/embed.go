package web

import (
	"embed"
	"io/fs"
)

//go:embed all:static
var staticEmbed embed.FS

// Static returns the embedded web UI filesystem (contents of static/).
func Static() (fs.FS, error) {
	return fs.Sub(staticEmbed, "static")
}
