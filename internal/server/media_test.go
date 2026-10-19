package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

func TestMediaAllowsForgeImagesOnly(t *testing.T) {
	t.Parallel()
	gitlab := []string{"gitlab.example.com"}
	for raw, want := range map[string]bool{
		"https://github.com/user-attachments/assets/abc":                         true,
		"https://github.com/EPFL-ENAC/wiki/assets/123/abc":                       true,
		"https://avatars.githubusercontent.com/u/1":                              true,
		"https://github-production-user-asset-6210df.s3.amazonaws.com/1/abc.png": true,
		"https://gitlab.example.com/-/project/1/uploads/abc/x.png":               true,
		"http://github.com/user-attachments/assets/abc":                          false, // not https
		"https://github.com/settings/tokens":                                     false,
		"https://gitlab.example.com/api/v4/user":                                 false,
		"https://evil.example/x.png":                                             false,
		"https://169.254.169.254/latest/meta-data":                               false,
	} {
		u, _ := url.Parse(raw)
		if got := forgeMedia(u, gitlab); got != want {
			t.Errorf("forgeMedia(%s) = %v, want %v", raw, got, want)
		}
	}
}

// fakeForge serves one image and one page, counting requests and keeping
// the last Authorization header.
func fakeForge(t *testing.T) (*httptest.Server, *atomic.Int32, *atomic.Value) {
	var hits atomic.Int32
	var auth atomic.Value
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		auth.Store(r.Header.Get("Authorization"))
		if strings.HasSuffix(r.URL.Path, ".png") {
			w.Header().Set("Content-Type", "image/png")
			_, _ = io.WriteString(w, "PNGDATA")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html>")
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, &auth
}

func TestMediaProxiesAndCaches(t *testing.T) {
	t.Parallel()
	forge, hits, auth := fakeForge(t)
	b := gitbugiotest.New(t)
	m := &mediaProxy{storage: b.Storage(), client: forge.Client(), token: func(string) string { return "secret" },
		allowed: func(*url.URL) bool { return true }}
	get := func(u string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/media?url="+url.QueryEscape(u), nil))
		return rec
	}

	img := forge.URL + "/a/shot.png"
	for i := range 2 {
		rec := get(img)
		if rec.Code != http.StatusOK || rec.Body.String() != "PNGDATA" || rec.Header().Get("Content-Type") != "image/png" {
			t.Fatalf("request %d: %d %q %q", i, rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
		}
	}
	if hits.Load() != 1 {
		t.Errorf("forge hit %d times, want once (cached)", hits.Load())
	}
	if auth.Load() != "Bearer secret" {
		t.Errorf("Authorization = %v, want the forge token", auth.Load())
	}
	if rec := get(forge.URL + "/a/page"); rec.Code != http.StatusBadGateway {
		t.Errorf("a page, not an image: %d, want 502", rec.Code)
	}

	m.allowed = func(*url.URL) bool { return false }
	if rec := get(forge.URL + "/b/other.png"); rec.Code != http.StatusBadRequest {
		t.Errorf("not a forge image: %d, want 400", rec.Code)
	}
}
