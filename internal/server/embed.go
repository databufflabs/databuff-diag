package server

import (
	"bytes"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	webassets "github.com/databufflabs/databuff-diag/internal/web"
)

func webFS() fs.FS {
	return webassets.FS
}

// staticHandler serves embedded static files with SPA fallback to index.html.
func staticHandler() http.Handler {
	fsys := webFS()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			data, err = fs.ReadFile(fsys, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			path = "index.html"
		}

		if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		http.ServeContent(w, r, path, time.Time{}, bytes.NewReader(data))
	})
}
