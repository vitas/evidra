package api

import (
	"embed"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
)

//go:embed ui/*
var embeddedUI embed.FS

func uiHandler() http.Handler {
	sub, err := fs.Sub(embeddedUI, "ui")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(sub))
}

func serveUI(w http.ResponseWriter, r *http.Request, handler http.Handler) {
	switch r.URL.Path {
	case "/ui":
		http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
		return
	case "/ui/":
		w.Header().Set("Cache-Control", "no-store")
		r = cloneRequestWithPath(r, "/")
	default:
		p := strings.TrimPrefix(r.URL.Path, "/ui/")
		if p == "" || strings.Contains(p, "..") {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(p, ".") {
			r = cloneRequestWithPath(r, "/"+p)
		} else {
			// SPA route fallback: /ui/explorer/change/{id}
			w.Header().Set("Cache-Control", "no-store")
			r = cloneRequestWithPath(r, "/")
		}
	}
	handler.ServeHTTP(w, r)
}

func cloneRequestWithPath(r *http.Request, path string) *http.Request {
	cp := r.Clone(r.Context())
	cp.URL = &url.URL{
		Path:     path,
		RawQuery: r.URL.RawQuery,
	}
	return cp
}
