package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// A page on another site can't write through the server: browsers send its
// Origin, and a no-preflight form post can't send JSON.
func TestWritesFromAnotherSiteAreRefused(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")
	ts := newTestServer(t, b, cfg)
	url := ts.URL + "/api/issues/" + id

	send := func(origin, contentType string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPatch, url, strings.NewReader(`{"prio":"p1"}`))
		if err != nil {
			t.Fatal(err)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	if got := send("https://evil.example", "application/json"); got != http.StatusForbidden {
		t.Errorf("another site's origin: %d, want 403", got)
	}
	if got := send("", "text/plain"); got != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain: %d, want 415", got)
	}
	if got := send(ts.URL, "application/json"); got != http.StatusOK {
		t.Errorf("same origin: %d, want 200", got)
	}
	if got := send("http://localhost:9000", "application/json; charset=utf-8"); got != http.StatusOK {
		t.Errorf("dev server: %d, want 200", got)
	}
	if got := send("", "application/json"); got != http.StatusOK {
		t.Errorf("no origin (curl): %d, want 200", got)
	}
}

// DNS rebinding: a page on evil.example re-resolves its own name to this
// machine; the browser then sends Host and Origin evil.example, which agree.
// Only this machine's names are served.
func TestRequestsForAnotherHostAreRefused(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")
	ts := newTestServer(t, b, cfg)
	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		req, err := http.NewRequest(method, ts.URL+"/api/issues/"+id, strings.NewReader(`{"prio":"p1"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "evil.example:8080"
		req.Header.Set("Origin", "http://evil.example:8080")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s with Host evil.example: %d, want 403", method, resp.StatusCode)
		}
	}
	// localhost is this machine
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/views", nil)
	req.Host = "localhost:8080"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Host localhost: %d, want 200", resp.StatusCode)
	}
}
