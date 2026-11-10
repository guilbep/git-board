package httpapi

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// The web app's files: gzipped to a browser that takes it, kept for good
// when named by their content, asked again otherwise.
func TestStatic(t *testing.T) {
	t.Parallel()
	js := strings.Repeat("console.log('git-board');\n", 200)
	h := static(fstest.MapFS{
		"index.html":       {Data: []byte("<!doctype html><title>x</title>")},
		"assets/app-1a.js": {Data: []byte(js)},
	})
	get := func(path, enc string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if enc != "" {
			r.Header.Set("Accept-Encoding", enc)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := get("/assets/app-1a.js", "gzip, deflate, br")
	if w.Code != 200 || w.Header().Get("Content-Encoding") != "gzip" || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/javascript") {
		t.Fatalf("gzipped asset: %d %v", w.Code, w.Header())
	}
	zr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := io.ReadAll(zr); string(got) != js || w.Body.Len() >= len(js) {
		t.Errorf("the gzipped body: %d bytes, unzipped right %v", w.Body.Len(), string(got) == js)
	}
	if w := get("/assets/app-1a.js", ""); w.Header().Get("Content-Encoding") != "" || w.Body.String() != js {
		t.Errorf("no gzip asked: %v", w.Header())
	}
	if w := get("/assets/app-1a.js", "gzip;q=0"); w.Header().Get("Content-Encoding") != "" {
		t.Errorf("gzip refused, sent gzipped")
	}
	// index.html: too small to gain, and asked again each time
	if w := get("/", "gzip"); w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-cache" || !strings.Contains(w.Body.String(), "<title>") {
		t.Errorf("index: %d %v", w.Code, w.Header())
	}
	if w := get("/assets/none.js", "gzip"); w.Code != http.StatusNotFound {
		t.Errorf("a missing asset: %d", w.Code)
	}
}
