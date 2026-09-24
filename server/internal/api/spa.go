package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler serves the built frontend from ui. Paths that don't match a file
// get index.html, so client-side routes (e.g. /ns/default/Deployment/web)
// survive a page reload. Vite fingerprints everything under /assets/, so
// those files are cached indefinitely; index.html is always revalidated so a
// new release is picked up immediately.
func spaHandler(ui fs.FS) http.Handler {
	fileServer := http.FileServerFS(ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if info, err := fs.Stat(ui, name); err == nil && !info.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		index, err := fs.ReadFile(ui, "index.html")
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				http.Error(w, "frontend not built: run `make build` or build the Docker image", http.StatusNotFound)
				return
			}
			http.Error(w, "read index.html", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index) //nolint:errcheck
	})
}

// configJSHandler serves the frontend's runtime configuration. It is rendered
// once from the server's own configuration and never cached, so a changed
// setting takes effect on the next pod restart.
func configJSHandler(apiURL string) http.Handler {
	cfg, err := json.Marshal(map[string]string{"apiUrl": apiURL})
	if err != nil {
		panic(err) // marshalling a map of strings cannot fail
	}
	body := []byte(fmt.Sprintf("window.__CONFIG__ = %s;\n", cfg))
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(body) //nolint:errcheck
	})
}
