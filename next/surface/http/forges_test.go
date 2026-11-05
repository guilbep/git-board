package httpapi

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/next/app"
	"github.com/guilbep/git-board/next/forge"
)

// Routes 33 to 36: a forge added with its token, its sync turned off,
// the tokens found listed, a forge removed. The token goes to the
// credential helper (faked), never to the file, the answer or the log.
// It swaps app's doors, so it runs serially.
func TestForges(t *testing.T) {
	dir, _ := newBoard(t)
	var stored []string
	approve, find := app.ApproveToken, app.FindToken
	t.Cleanup(func() { app.ApproveToken, app.FindToken = approve, find })
	app.ApproveToken = func(_ context.Context, baseURL, login, token string) error {
		stored = append(stored, baseURL+" "+login+" "+token)
		return nil
	}
	app.FindToken = func(_ context.Context, f forge.Table, login string) (string, error) {
		if f.Host == "github.com" && login == "ada" {
			return "t", nil
		}
		return "", nil
	}
	a := newAPI(t, dir)
	file := func() string {
		data, err := os.ReadFile(filepath.Join(dir, ".git-board.toml"))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	got := want[forgeNameRes](t, a.do("POST", "/api/forges", map[string]any{"kind": "github", "url": "https://github.com/o/r.git", "token": " s3cret-token "}), http.StatusCreated)
	if got.Name != "gh" || len(stored) != 1 || stored[0] != "https://github.com ada s3cret-token" {
		t.Fatalf("added %q, stored %q", got.Name, stored)
	}
	got = want[forgeNameRes](t, a.do("POST", "/api/forges", map[string]any{"kind": "github", "url": "https://github.example.org/g/p", "name": "gl", "login": "bot", "sync": false}), http.StatusCreated)
	if got.Name != "gl" || len(stored) != 1 {
		t.Fatalf("added %q, stored %q", got.Name, stored)
	}
	text := file()
	for _, w := range []string{"[forges.gh]", `project = "o/r"`, "[forges.gl]", `host = "github.example.org"`, `login = "bot"`} {
		if !strings.Contains(text, w) {
			t.Errorf("the file has no %s:\n%s", w, text)
		}
	}
	for _, bad := range []map[string]any{{"kind": "gitea", "url": "https://x.org/o/r"}, {"kind": "github", "url": "github.com/o/r"},
		{"kind": "github", "url": "https://github.com/o"}, {"kind": "github", "url": "https://github.com/o/s", "name": "gh"}, {"kind": "github", "url": "https://github.com/o/s", "name": "a b"}} {
		want[map[string]any](t, a.do("POST", "/api/forges", bad), http.StatusBadRequest)
	}

	if got := want[forgeSyncRes](t, a.do("PATCH", "/api/forges/gh", map[string]any{"sync": false}), http.StatusOK); got.Sync || got.Name != "gh" {
		t.Errorf("patched: %+v", got)
	}
	want[map[string]any](t, a.do("PATCH", "/api/forges/gh", map[string]any{}), http.StatusBadRequest)
	want[map[string]any](t, a.do("PATCH", "/api/forges/nope", map[string]any{"sync": true}), http.StatusNotFound)

	creds := want[items[app.Credential]](t, a.do("GET", "/api/forges/credentials", nil), http.StatusOK)
	if len(creds.Items) != 1 || creds.Items[0] != (app.Credential{Host: "github.com", Login: "ada", Source: "git's credential helper"}) {
		t.Errorf("credentials: %+v", creds.Items)
	}

	want[struct{}](t, a.do("DELETE", "/api/forges/gl", nil), http.StatusNoContent)
	want[map[string]any](t, a.do("DELETE", "/api/forges/gl", nil), http.StatusNotFound)
	if text := file(); strings.Contains(text, "forges.gl") || !strings.Contains(text, "[forges.gh]") || strings.Contains(text, "s3cret") {
		t.Errorf("the file after:\n%s", text)
	}
	if strings.Contains(a.logs(), "s3cret") {
		t.Errorf("the token is logged:\n%s", a.logs())
	}
}
