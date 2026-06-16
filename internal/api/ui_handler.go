package api

import (
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
)

// uiHandler serves the embedded UI filesystem with SPA fallback.
// Unknown paths fall back to index.html for client-side routing.
func uiHandler(uiFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(uiFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Strip trailing slash for directory check (fs.Open requires clean paths).
		cleanPath := strings.TrimRight(path, "/")
		if cleanPath == "" {
			cleanPath = "."
		}

		f, err := uiFS.Open(cleanPath)
		if err != nil {
			if filepath.Ext(cleanPath) != "" {
				http.NotFound(w, r)
				return
			}
			// SPA fallback: serve index.html for unknown paths.
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		_ = f.Close()
		fileServer.ServeHTTP(w, r)
	})
}
