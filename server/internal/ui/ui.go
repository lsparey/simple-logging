// Package ui embeds the built frontend so the server binary can serve it.
//
// The frontend build output (frontend/dist) is copied into dist/ before the
// server is compiled; `make build` and the Dockerfile both do this. A plain
// `go build` without that step embeds only the placeholder, and the server
// then answers / with a "frontend not built" message instead of the SPA.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the embedded frontend build, rooted at its index.html.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // "dist" is a valid, embedded path; this cannot fail.
	}
	return sub
}
