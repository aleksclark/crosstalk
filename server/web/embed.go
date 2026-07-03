// Package web embeds the built frontend single-page apps and exposes them
// as filesystems keyed by their URL path prefix.
//
// Each app's production build output (Vite dist/) is copied into the matching
// subdirectory here at container build time (see deploy/Dockerfile). For local
// Go builds without a frontend build, placeholder index.html files keep the
// embed directive satisfied.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:admin all:broadcast all:translator
var assets embed.FS

// appNames lists the embedded SPAs. Each is served at /<name>/.
var appNames = []string{"admin", "broadcast", "translator"}

// Apps returns a map of URL path prefix ("/admin") to a filesystem rooted at
// that app's build output.
func Apps() (map[string]fs.FS, error) {
	out := make(map[string]fs.FS, len(appNames))
	for _, name := range appNames {
		sub, err := fs.Sub(assets, name)
		if err != nil {
			return nil, err
		}
		out["/"+name] = sub
	}
	return out, nil
}
