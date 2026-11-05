package httpapi

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
)

// newExposedAPI is a test server that listens, as --addr 0.0.0.0 would,
// beyond loopback.
func newExposedAPI(t testing.TB, dir string) *api {
	t.Helper()
	sw := &syncWriter{w: new(bytes.Buffer)}
	log, _ := NewLogger(sw, "debug", "text")
	s, err := New(ctx, Options{Dir: dir, Log: log, Version: "test", Exposed: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(func() {
		s.Close()
		srv.Close()
	})
	return &api{t: t, srv: srv, s: s, log: sw, dir: dir}
}

// The code gate (ADR 0013, Amendment 2026-11-05): a config write that
// sets a command is refused on a server exposed beyond loopback, even
// from its own page, and asks the board's own page on localhost; one
// that sets none goes through, and a dry run is only a check.
func TestCodeGateConfig(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	path := dir + "/.git-board.toml"
	exposed := newExposedAPI(t, dir)
	own := "Origin"
	for _, c := range []struct {
		method string
		body   map[string]any
	}{
		{"PATCH", map[string]any{"set": map[string]any{"release.build": "id > /tmp/pwned", "release.binaries": []string{"./x"}}}},
		{"PATCH", map[string]any{"set": map[string]any{"release": map[string]any{"environments": []string{"prod"}, "checks": "id"}}}},
		{"PATCH", map[string]any{"set": map[string]any{"cycle.checks.unit.run": []string{"sh", "-c", "id"}}}},
		{"PATCH", map[string]any{"set": map[string]any{"cycle.signers": "keys"}}},
		{"PUT", map[string]any{"text": boardToml + "\n[release]\nbinaries = [\"./x\"]\nbuild = \"id\"\n"}},
	} {
		e := want[apiError](t, exposed.do(c.method, "/api/config", c.body, own, exposed.srv.URL), 403)
		if e.Code != "forbidden" || !strings.Contains(e.Message, "beyond this machine") {
			t.Errorf("%s %v on an exposed server = %+v", c.method, c.body, e)
		}
		if got := read(t, path); got != boardToml {
			t.Fatalf("%s %v on an exposed server wrote:\n%s", c.method, c.body, got)
		}
	}
	// what isn't code goes through; a dry run of code too
	want[configRes](t, exposed.do("PATCH", "/api/config", map[string]any{"set": map[string]any{"views.mine.sort": "updated"}}, own, exposed.srv.URL), 200)
	want[configRes](t, exposed.do("PATCH", "/api/config?dry_run=1", map[string]any{"set": map[string]any{"release.build": "id", "release.binaries": []string{"./x"}}}, own, exposed.srv.URL), 200)
	if strings.Contains(read(t, path), "build") {
		t.Fatal("a dry run wrote")
	}
}

// On loopback, a config write that sets a command asks the board's own
// page: a script without an Origin is refused, the page's write goes.
func TestCodeGateConfigLocal(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	set := map[string]any{"set": map[string]any{"release.build": "true", "release.binaries": []string{"./x"}}}
	if e := want[apiError](t, a.do("PATCH", "/api/config", set), 403); e.Code != "forbidden" || !strings.Contains(e.Message, "own page") {
		t.Fatalf("no Origin = %+v", e)
	}
	c := want[configRes](t, a.do("PATCH", "/api/config", set, "Origin", a.srv.URL), 200)
	if !strings.Contains(c.Text, `build = "true"`) {
		t.Fatalf("the page's write:\n%s", c.Text)
	}
	// the same value again changes nothing: no gate
	want[configRes](t, a.do("PATCH", "/api/config", set), 200)
	// a stale write is stale, whatever it sets
	if e := want[apiError](t, a.do("PUT", "/api/config", map[string]any{"text": boardToml}, "If-Match", `"nope"`), 412); e.Code != "stale" {
		t.Fatalf("a stale PUT = %+v", e)
	}
}

// Every route that runs code, or changes what runs, passes the gate:
// refused on an exposed server, and on loopback without the board's own
// page. Notes and yanks are records, not code.
func TestCodeGateRoutes(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	exposed := newExposedAPI(t, dir)
	local := newAPI(t, dir)
	routes := []struct {
		method, path string
		body         map[string]any
	}{
		{"POST", "/api/releases", map[string]any{"version": "0.0.1", "final": true}},
		{"PATCH", "/api/releases/v1.0.0", map[string]any{"state": "promoted"}},
		{"PATCH", "/api/releases/v1.0.0", map[string]any{"adopt": []map[string]any{{"env": "prod"}}}},
		{"POST", "/api/environments/prod/deploy", map[string]any{"tag": "v1.0.0"}},
		{"POST", "/api/environments/prod/deploy", map[string]any{"tag": "v1.0.0", "reason": "broken"}},
	}
	for _, r := range routes {
		if e := want[apiError](t, exposed.do(r.method, r.path, r.body, "Origin", exposed.srv.URL), 403); !strings.Contains(e.Message, "beyond this machine") {
			t.Errorf("%s %s %v on an exposed server = %+v", r.method, r.path, r.body, e)
		}
		if e := want[apiError](t, local.do(r.method, r.path, r.body), 403); !strings.Contains(e.Message, "own page") {
			t.Errorf("%s %s %v without an Origin = %+v", r.method, r.path, r.body, e)
		}
		// the page passes the gate: what refuses it now is the board, without [release]
		if res := local.do(r.method, r.path, r.body, "Origin", local.srv.URL); res.StatusCode == 403 {
			t.Errorf("%s %s %v from the page: 403", r.method, r.path, r.body)
		}
	}
	if res := exposed.do("PATCH", "/api/releases/v1.0.0", map[string]any{"notes": "x"}, "Origin", exposed.srv.URL); res.StatusCode == 403 {
		t.Error("notes on an exposed server: 403")
	}
}
