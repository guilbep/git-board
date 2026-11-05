package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// codeGateServer is a server on a board whose .git-board.toml says
// nothing of code, exposed beyond loopback or not.
func codeGateServer(t *testing.T, exposed bool) (*apiServer, string) {
	t.Helper()
	s := &apiServer{cfg: &config.Config{}, b: storetest.New(t), cfgPath: filepath.Join(t.TempDir(), ".git-board.toml")}
	if exposed {
		s.exposed = []string{"*"}
	}
	if err := os.WriteFile(s.cfgPath, []byte(codeGateToml), 0o644); err != nil {
		t.Fatal(err)
	}
	return s, s.cfgPath
}

const codeGateToml = "[fields.status]\noptions = [\"todo\", \"done\"]\n"

// A request that runs code, or changes what code runs, is refused on a
// server exposed beyond loopback, and asks the board's own page on
// localhost (ADR 0013, Amendment 2026-11-05; GHSA: a config write set
// [release] build and a cut ran it with sh -c, from any client of a
// server started with --addr 0.0.0.0).
func TestCodeGate(t *testing.T) {
	t.Parallel()
	hash := func(text string) string { sum := sha256.Sum256([]byte(text)); return hex.EncodeToString(sum[:]) }
	withBuild := codeGateToml + "\n[release]\nbinaries = [\"./x\"]\nbuild = \"id > /tmp/pwned\"\n"
	routes := []struct {
		method, path string
		body         any
	}{
		{"PUT", "/api/config", map[string]string{"text": withBuild, "base": hash(codeGateToml)}},
		{"PUT", "/api/config/section/release?base=" + hash(codeGateToml), map[string]any{"binaries": []string{"./x"}, "build": "id"}},
		{"POST", "/api/releases/setup", map[string]any{"binaries": []string{"./x"}, "build": "id"}},
		{"POST", "/api/releases", map[string]any{"version": "0.0.1", "final": true}},
		{"POST", "/api/releases/v1.0.0/promote", map[string]any{}},
		{"POST", "/api/releases/adopt", map[string]any{"proposals": []any{}}},
		{"POST", "/api/environments/prod/deploy", map[string]any{"tag": "v1.0.0"}},
		{"POST", "/api/environments/prod/rollback", map[string]any{"tag": "v1.0.0", "reason": "x"}},
	}
	for _, exposed := range []bool{true, false} {
		s, path := codeGateServer(t, exposed)
		ts := serve(t, s)
		for _, r := range routes {
			origin, want := ts.URL, "exposed"
			if !exposed {
				origin, want = "", "origin"
			}
			code, got, out := sendFrom(t, r.method, ts.URL+r.path, origin, r.body)
			if code != http.StatusForbidden || got != want {
				t.Errorf("exposed %v: %s %s: %d %s, want 403 %s", exposed, r.method, r.path, code, out, want)
			}
		}
		if data, _ := os.ReadFile(path); string(data) != codeGateToml {
			t.Errorf("exposed %v: the file was written:\n%s", exposed, data)
		}
	}
	// from the page on localhost: the gate lets it through
	s, path := codeGateServer(t, false)
	ts := serve(t, s)
	if code, _, out := sendFrom(t, "PUT", ts.URL+"/api/config", ts.URL, routes[0].body); code != http.StatusOK {
		t.Fatalf("the page's write: %d %s", code, out)
	}
	if data, _ := os.ReadFile(path); string(data) != withBuild {
		t.Fatalf("the page's write wrote:\n%s", data)
	}
	// on an exposed server, a write that sets no command goes through
	s, _ = codeGateServer(t, true)
	ts = serve(t, s)
	if code, _, out := sendFrom(t, "PUT", ts.URL+"/api/config", ts.URL, map[string]string{"text": codeGateToml + "\n[views.all]\nfilter = \"\"\n", "base": hash(codeGateToml)}); code != http.StatusOK {
		t.Fatalf("a write without a command, exposed: %d %s", code, out)
	}
}
