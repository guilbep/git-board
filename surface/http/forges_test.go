package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/forge"
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

	if got := want[forgeNameRes](t, a.do("POST", "/api/forges", map[string]any{"kind": "github", "url": "https://github.com/o/b", "name": "bk", "way": "push"}), http.StatusCreated); got.Name != "bk" {
		t.Fatalf("added %q", got.Name)
	}
	if text := file(); !strings.Contains(text, "[forges.bk]") || !strings.Contains(text, `way = "push"`) {
		t.Errorf("the backup's table:\n%s", text)
	}
	want[map[string]any](t, a.do("POST", "/api/forges", map[string]any{"kind": "github", "url": "https://github.com/o/x", "name": "x", "way": "send"}), http.StatusBadRequest)

	if got := want[forgeSyncRes](t, a.do("PATCH", "/api/forges/gh", map[string]any{"sync": false}), http.StatusOK); got.Sync || got.Name != "gh" {
		t.Errorf("patched: %+v", got)
	}
	if got := want[forgeSyncRes](t, a.do("PATCH", "/api/forges/gh", map[string]any{"way": "pull"}), http.StatusOK); got.Way != "pull" || got.Sync {
		t.Errorf("way: %+v", got)
	}
	if text := file(); !strings.Contains(text, `way = "pull"`) {
		t.Errorf("the table's way:\n%s", text)
	}
	if got := want[forgeSyncRes](t, a.do("PATCH", "/api/forges/gh", map[string]any{"way": "both"}), http.StatusOK); got.Way != "both" {
		t.Errorf("way: %+v", got)
	}
	now := file()
	gh := now[:strings.Index(now, "[forges.gl]")]
	if strings.Contains(gh, "way =") || !strings.Contains(file(), `way = "push"`) {
		t.Errorf("both is the default, gh's table says no way:\n%s", file())
	}
	want[map[string]any](t, a.do("PATCH", "/api/forges/gh", map[string]any{"way": "send"}), http.StatusBadRequest)
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

// Routes 59 and 55: the forges' tokens as forge status reads them, and a
// token stored in the credential helper (faked) from the body alone,
// never answered nor logged.
func TestForgeAuth(t *testing.T) {
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
	owner := app.ForgeOwner
	t.Cleanup(func() { app.ForgeOwner = owner })
	app.ForgeOwner = func(_ context.Context, f forge.Table, login string) (string, error) {
		if f.Host == "github.com" && login == "ada" {
			return "ada", nil
		}
		return "", errors.New("the forge refused it")
	}
	if _, err := app.SetConfigKeys(dir, map[string]any{"forges.gh": map[string]any{"kind": "github", "project": "o/r", "login": "ada"}}, "", false); err != nil {
		t.Fatal(err)
	}
	a := newAPI(t, dir)

	st := want[items[app.ForgeStatus]](t, a.do("GET", "/api/forges/auth", nil), http.StatusOK)
	if len(st.Items) != 1 || st.Items[0].Name != "gh" || len(st.Items[0].Tokens) != 1 {
		t.Fatalf("auth: %+v", st.Items)
	}
	tk := st.Items[0].Tokens[0]
	if !tk.Found || tk.Source != "git's credential helper" || tk.Owner != "" {
		t.Errorf("token: %+v", tk)
	}
	st = want[items[app.ForgeStatus]](t, a.do("GET", "/api/forges/auth?check=1", nil), http.StatusOK)
	if tk = st.Items[0].Tokens[0]; tk.Owner != "ada" {
		t.Errorf("checked: %+v", tk)
	}

	got := want[tokenStored](t, a.do("POST", "/api/forges/auth", map[string]any{"forge": "gh", "token": " s3cret-token "}), http.StatusCreated)
	if got != (tokenStored{Forge: "gh", Host: "github.com", Login: "ada"}) || len(stored) != 1 || stored[0] != "https://github.com ada s3cret-token" {
		t.Fatalf("stored %+v, %q", got, stored)
	}
	want[map[string]any](t, a.do("POST", "/api/forges/auth", map[string]any{"forge": "gh", "token": " "}), http.StatusBadRequest)
	want[map[string]any](t, a.do("POST", "/api/forges/auth", map[string]any{"forge": "nope", "token": "t"}), http.StatusNotFound)
	want[map[string]any](t, a.do("POST", "/api/forges/auth", map[string]any{"forge": "gh"}), http.StatusBadRequest)
	if strings.Contains(a.logs(), "s3cret") {
		t.Errorf("the token is logged:\n%s", a.logs())
	}
}

// The routes that write a secret, or say where the board goes, answer on
// loopback alone: from another address each is refused, and writes
// nothing.
func TestLocalRoutes(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	before := read(t, filepath.Join(dir, ".git-board.toml"))
	for _, route := range localRoutes {
		method, path, _ := strings.Cut(route, " ")
		path = strings.ReplaceAll(path, "{name}", "origin")
		for _, from := range []string{"192.0.2.1:1234", "[2001:db8::1]:1234"} {
			req := httptest.NewRequest(method, path, strings.NewReader(`{"value":"git","token":"t","forge":"gh","url":"https://x.org/o/r","kind":"github"}`))
			req.RemoteAddr, req.Host = from, "127.0.0.1:8080" // a remote client, past the Host check
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			a.s.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "localhost only") {
				t.Errorf("%s from %s = %d %s", route, from, rec.Code, rec.Body)
			}
		}
	}
	if got := read(t, filepath.Join(dir, ".git-board.toml")); got != before {
		t.Errorf("a refused request wrote:\n%s", got)
	}
}

// A forge found in the git remotes has no table: PATCH writes one, with
// the other found ones, so one click makes it read-only.
func TestForgeFoundMadeReadOnly(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	gitRun(t, dir, "remote", "add", "origin", "https://github.com/o/r.git")
	a := newAPI(t, dir)
	st := want[app.State](t, a.do("GET", "/api/state", nil), http.StatusOK)
	if len(st.Forges) != 1 || !st.Forges[0].Discovered {
		t.Fatalf("forges %+v, want one found in origin", st.Forges)
	}
	name := st.Forges[0].Name
	got := want[forgeSyncRes](t, a.do("PATCH", "/api/forges/"+name, map[string]any{"way": "pull"}), http.StatusOK)
	if got.Way != "pull" || !got.Sync {
		t.Fatalf("patched: %+v", got)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".git-board.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"[forges." + name + "]", `project = "o/r"`, `way = "pull"`} {
		if !strings.Contains(string(data), w) {
			t.Errorf("the file has no %s:\n%s", w, data)
		}
	}
	if st := want[app.State](t, a.do("GET", "/api/state", nil), http.StatusOK); len(st.Forges) != 1 || st.Forges[0].Discovered || st.Forges[0].Way != "pull" {
		t.Errorf("forges after: %+v", st.Forges)
	}
}

// Route 56 from another address: a template's path or URL is refused (it
// would read this machine's files, or the network), a built-in name
// answers.
func TestInitTemplateFromElsewhere(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	if err := os.Remove(filepath.Join(dir, ".git-board.toml")); err != nil {
		t.Fatal(err)
	}
	a := newAPI(t, dir)
	init := func(template string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/config/init", strings.NewReader(`{"template":"`+template+`"}`))
		req.RemoteAddr, req.Host = "192.0.2.1:1234", "127.0.0.1:8080"
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		a.s.Handler().ServeHTTP(rec, req)
		return rec
	}
	for _, tpl := range []string{"/etc/hosts", "https://example.org/t.toml", "../x.toml"} {
		if rec := init(tpl); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "localhost only") {
			t.Errorf("template %s from elsewhere = %d %s", tpl, rec.Code, rec.Body)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".git-board.toml")); !os.IsNotExist(err) {
		t.Fatalf("a refused init wrote the file: %v", err)
	}
	if rec := init("enac"); rec.Code != http.StatusCreated {
		t.Errorf("the built-in enac from elsewhere = %d %s", rec.Code, rec.Body)
	}
}
