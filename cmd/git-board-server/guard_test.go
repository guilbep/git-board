package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// A page on another site can't write through the server: browsers send its
// Origin, and a no-preflight form post can't send JSON.
func TestWritesFromAnotherSiteAreRefused(t *testing.T) {
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
