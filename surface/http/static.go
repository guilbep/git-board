package httpapi

import (
	"bytes"
	"compress/gzip"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"
)

// static serves the web app's files, which are embedded and so never
// change while the server runs: what nginx does in front of a deployed
// app. /assets/ files are named by their content (Vite's hash), so a
// browser keeps them for good; the rest (index.html, theme.js) it asks
// again each time. A text file goes as brotli to a browser that takes it,
// from the .br the build wrote beside it (web/vite.config.ts); else
// gzipped, compressed on its first request and kept. A .br itself is no
// page of the app: asked by its name, it isn't found.
func static(fsys fs.FS) http.Handler {
	files := http.FileServerFS(fsys)
	var gz sync.Map // name -> []byte, or nil: not worth it
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		w.Header().Add("Vary", "Accept-Encoding")
		if strings.HasSuffix(name, ".br") {
			http.NotFound(w, r)
			return
		}
		read := r.Method == http.MethodGet || r.Method == http.MethodHead
		if read && compressible(name) && accepts(r, "br") {
			if body, err := fs.ReadFile(fsys, name+".br"); err == nil {
				h := w.Header()
				h.Set("Content-Type", mime.TypeByExtension(path.Ext(name)))
				h.Set("Content-Encoding", "br")
				w.WriteHeader(http.StatusOK)
				if r.Method == http.MethodGet {
					w.Write(body)
				}
				return
			}
		}
		if !read || !acceptsGzip(r) || !compressible(name) {
			files.ServeHTTP(w, r)
			return
		}
		v, ok := gz.Load(name)
		if !ok {
			v = gzipped(fsys, name)
			gz.Store(name, v)
		}
		body, _ := v.([]byte)
		if body == nil {
			files.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		h.Set("Content-Type", mime.TypeByExtension(path.Ext(name)))
		h.Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			w.Write(body)
		}
	})
}

func acceptsGzip(r *http.Request) bool { return accepts(r, "gzip") }

// accepts says whether r's Accept-Encoding takes coding.
func accepts(r *http.Request, coding string) bool {
	for part := range strings.SplitSeq(r.Header.Get("Accept-Encoding"), ",") {
		if enc, q, _ := strings.Cut(strings.TrimSpace(part), ";"); strings.TrimSpace(enc) == coding && strings.TrimSpace(q) != "q=0" {
			return true
		}
	}
	return false
}

func compressible(name string) bool {
	switch path.Ext(name) {
	case ".js", ".css", ".html", ".svg", ".json", ".txt", ".map":
		return true
	}
	return false
}

// gzipped is the file compressed; nil when it can't be read, or is too
// small to gain.
func gzipped(fsys fs.FS, name string) []byte {
	data, err := fs.ReadFile(fsys, name)
	if err != nil || len(data) < 1024 {
		return nil
	}
	var b bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
	zw.Write(data)
	if zw.Close() != nil || b.Len() >= len(data) {
		return nil
	}
	return b.Bytes()
}
