package api

import (
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
)

// mountWebApps registers static file serving for each embedded SPA. Each app is
// served under its path prefix (e.g. "/admin") with history-API fallback: any
// unmatched sub-path returns the app's index.html so client-side routing works.
func (s *Server) mountWebApps() {
	for prefix, fsys := range s.services.WebApps {
		mountSPA(s.router, prefix, fsys)
	}
}

func mountSPA(r chi.Router, prefix string, fsys fs.FS) {
	// Redirect the bare prefix to the trailing-slash form so relative asset
	// URLs resolve correctly.
	r.Get(prefix, func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, prefix+"/", http.StatusMovedPermanently)
	})

	r.Get(prefix+"/*", func(w http.ResponseWriter, req *http.Request) {
		rel := strings.TrimPrefix(req.URL.Path, prefix+"/")
		if rel == "" {
			rel = "index.html"
		}
		serveFileWithFallback(w, req, fsys, rel)
	})
}

// serveFileWithFallback serves rel from fsys, falling back to index.html for any
// missing path (SPA history-API routing). It writes files directly via
// http.ServeContent to avoid http.FileServer's implicit index.html redirects.
func serveFileWithFallback(w http.ResponseWriter, req *http.Request, fsys fs.FS, rel string) {
	f, err := fsys.Open(rel)
	if err == nil {
		if st, statErr := f.Stat(); statErr == nil && st.IsDir() {
			_ = f.Close()
			f, err = nil, fs.ErrNotExist
		}
	}
	if err != nil {
		rel = "index.html"
		f, err = fsys.Open(rel)
		if err != nil {
			http.NotFound(w, req)
			return
		}
	}
	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil {
		http.Error(w, "stat error", http.StatusInternalServerError)
		return
	}

	if ctype := mime.TypeByExtension(path.Ext(rel)); ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}

	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, req, rel, st.ModTime(), rs)
		return
	}
	_, _ = io.Copy(w, f)
}
