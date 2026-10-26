package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// fakeGitLab answers what adding a GitLab forge asks: who the token is,
// and the project; and who can be assigned.
func fakeGitLab(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Private-Token") != "good-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"message":"401 Unauthorized"}`)
			return
		}
		switch {
		case r.URL.Path == "/api/v4/user":
			_, _ = io.WriteString(w, `{"id":1,"username":"guilbert"}`)
		case strings.HasSuffix(r.URL.Path, "/users") && strings.HasPrefix(r.URL.Path, "/api/v4/projects/"):
			_, _ = io.WriteString(w, `[{"username":"bob"},{"username":"alice"}]`)
		case r.URL.EscapedPath() == "/api/v4/projects/scratchpad%2Fsync-trial" || r.URL.Path == "/api/v4/projects/42400":
			_, _ = io.WriteString(w, `{"id":42400,"path_with_namespace":"scratchpad/sync-trial"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCredentialsNeverShowTheirToken(t *testing.T) {
	t.Parallel()
	ts, s := forgeServer(t, ghForge, glForge)
	storetest.Token(t, s.b, "https://github.com", "guilbep", "very-secret-value")
	resp, err := http.Get(ts.URL + "/api/credentials")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "very-secret-value") {
		t.Fatalf("a token value was served: %s", body)
	}
	var creds []struct{ ID, Target, Host, Login string }
	if err := json.Unmarshal(body, &creds); err != nil || len(creds) != 1 ||
		creds[0].ID != "gh" || creds[0].Target != "github" || creds[0].Host != "github.com" || creds[0].Login != "guilbep" {
		t.Errorf("credentials %s (%v)", body, err)
	}
}

func TestAddAndRemoveABridge(t *testing.T) {
	t.Parallel()
	ts, s := forgeServer(t)
	gl := fakeGitLab(t)
	project := gl.URL + "/scratchpad/sync-trial"

	code, refused := callError(t, "POST", ts.URL+"/api/bridges", map[string]string{"target": "gitlab", "url": project, "token": "wrong"})
	if code != http.StatusBadRequest || !strings.Contains(refused, "refused the token") {
		t.Errorf("a refused token: %d %q, want 400 saying it isn't added", code, refused)
	}
	if code, msg := callError(t, "POST", ts.URL+"/api/bridges", map[string]string{"target": "gitlab", "url": project}); code != http.StatusBadRequest || !strings.Contains(msg, "no token") {
		t.Errorf("no token anywhere: %d %q", code, msg)
	}

	var added struct{ Name string }
	if code := call(t, "POST", ts.URL+"/api/bridges", map[string]string{"target": "gitlab", "url": project, "token": "good-token"}, &added); code != http.StatusOK {
		t.Fatalf("add: %d", code)
	}
	if added.Name != "gl" {
		t.Errorf("default name %q, want gl", added.Name)
	}
	host := strings.TrimPrefix(gl.URL, "http://")
	want := config.Forge{Name: "gl", Kind: "gitlab", Host: host, Project: "42400", Login: "guilbert", Sync: true}
	if f := s.config().Forges; len(f) != 1 || f[0] != want {
		t.Errorf("forges after add: %+v, want %+v", f, want)
	}
	if creds := s.b.Credentials(s.config().Forges); len(creds) != 1 || creds[0].Login != "guilbert" {
		t.Errorf("the pasted token isn't kept: %+v", creds)
	}
	if text, _ := os.ReadFile(s.cfgPath); strings.Contains(string(text), "good-token") {
		t.Errorf("the token is in the settings:\n%s", text)
	}

	// again, with the stored credential and sync off: a second name
	if code := call(t, "POST", ts.URL+"/api/bridges", map[string]any{"target": "gitlab", "url": project, "credential": "gl", "sync": false}, &added); code != http.StatusOK || added.Name != "gl2" {
		t.Errorf("add with the stored credential: %d %q, want gl2", code, added.Name)
	}
	if f := s.config().Forges; len(f) != 2 || f[1].Name != "gl2" || f[1].Sync {
		t.Errorf("a bridge added with sync off: %+v", f)
	}

	if code := call(t, "DELETE", ts.URL+"/api/bridges/gl", nil, nil); code != http.StatusOK {
		t.Errorf("remove: %d", code)
	}
	if code := call(t, "DELETE", ts.URL+"/api/bridges/nope", nil, nil); code != http.StatusNotFound {
		t.Errorf("remove an unknown bridge: %d, want 404", code)
	}
	if f := s.config().Forges; len(f) != 1 || f[0].Name != "gl2" {
		t.Errorf("bridges after remove: %+v", f)
	}

	s.syncing.Store(true)
	if code := call(t, "DELETE", ts.URL+"/api/bridges/gl2", nil, nil); code != http.StatusConflict {
		t.Errorf("during a forge sync: %d, want 409", code)
	}
}

// Adding a forge while the forges are found in the remotes writes theirs
// too, as GitHub spells it: a table turns the finding off. A project a
// remote names already is refused. Serial: it answers for GitHub.
func TestAddAForgeKeepsTheFoundOnes(t *testing.T) {
	storetest.GitHub(t, map[string]string{"o/r": "O/R"}) // what a run reads of the found one
	ts, s := forgeServer(t)
	storetest.ForgeOrigin(t, s.b.GitDir(), "git@github.com:o/r.git")
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path { // what adding asks: who the token is, and the project
		case "/user":
			_, _ = io.WriteString(w, `{"login":"me"}`)
		case "/repos/o/r":
			_, _ = io.WriteString(w, `{"id":1,"full_name":"O/R"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(github.Close)
	s.githubAPI = github.URL

	code, msg := callError(t, "POST", ts.URL+"/api/bridges", map[string]string{"target": "github", "url": "https://github.com/o/r", "token": "tok"})
	if code != http.StatusBadRequest || !strings.Contains(msg, "the git remotes name github.com o/r already: it is the forge github") {
		t.Errorf("a project a remote names: %d %q", code, msg)
	}
	gl := fakeGitLab(t)
	if code := call(t, "POST", ts.URL+"/api/bridges", map[string]string{"target": "gitlab", "url": gl.URL + "/scratchpad/sync-trial", "token": "good-token"}, nil); code != http.StatusOK {
		t.Fatalf("add: %d", code)
	}
	text, _ := os.ReadFile(s.cfgPath)
	for _, want := range []string{"[forges.github]\nkind = \"github\"\nproject = \"O/R\"\nsync = true\n", "[forges.gl]\nkind = \"gitlab\""} {
		if !strings.Contains(string(text), want) {
			t.Errorf("the file lacks %q:\n%s", want, text)
		}
	}
	if f := s.config().Forges; len(f) != 2 || s.config().Discover {
		t.Errorf("forges %+v, discover %v", f, s.config().Discover)
	}
}

// callError sends a JSON request and returns the status and the error the
// server answered.
func callError(t *testing.T, method, url string, body any) (int, string) {
	t.Helper()
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, url, strings.NewReader(string(data)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct{ Error string }
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.Error
}

func TestBridgeErrorsInPlainWords(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"GET /user: 401 Unauthorized":                                                         "refused the token",
		"non-200 OK status code: 401 Unauthorized body: Bad credentials":                      "refused the token",
		"GET /projects/scratchpad%2Fnope: 404 Not Found":                                      "doesn't know this project",
		"project doesn't exist or authentication token has an incorrect scope":                "doesn't know this project",
		`Get "https://nope.example/api/v4/user": dial tcp: lookup nope.example: no such host`: "can't reach the forge",
		"a forge is named gl already":                                                         "a forge is named gl already",
	} {
		if got := plainBridgeError(errors.New(raw)).Error(); !strings.Contains(got, want) {
			t.Errorf("%q → %q, want it to say %q", raw, got, want)
		}
	}
}
