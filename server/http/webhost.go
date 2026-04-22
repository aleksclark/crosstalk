package http

import (
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// EmbedHandler returns an http.Handler that serves static files from fsys with
// SPA fallback: if the requested path does not match a file, index.html is
// served instead. Paths starting with /api/ or /ws/ are never handled; the
// handler returns a 404 so the router's earlier routes take precedence.
func EmbedHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never intercept API or WebSocket paths.
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
			http.NotFound(w, r)
			return
		}

		// Try to open the requested file. Strip leading slash for fs.Open.
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		f, err := fsys.Open(path)
		if err != nil {
			// File not found — serve index.html for SPA client-side routing.
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()

		// File exists — serve it directly.
		fileServer.ServeHTTP(w, r)
	})
}

// DevProxyHandler returns an http.Handler that reverse-proxies all requests
// (including WebSocket upgrades for Vite HMR) to the given target URL.
// Paths starting with /api/ or /ws/ are not handled (returns 404).
func DevProxyHandler(targetURL string) http.Handler {
	target, err := url.Parse(targetURL)
	if err != nil {
		slog.Error("invalid dev proxy URL", "url", targetURL, "error", err)
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "invalid dev proxy URL", http.StatusInternalServerError)
		})
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = target.Host
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never intercept API or WebSocket paths.
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
			http.NotFound(w, r)
			return
		}

		proxy.ServeHTTP(w, r)
	})
}
