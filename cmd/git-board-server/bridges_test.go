package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/git-bug/git-bug/bridge/core/auth"
)

// fakeGitLab answers what git-bug's GitLab bridge asks when configured:
// who the token is, and the project.
func fakeGitLab(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Private-Token") != "good-token" && r.Header.Get("Authorization") != "Bearer good-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"message":"401 Unauthorized"}`)
			return
		}
		switch {
		case r.URL.Path == "/api/v4/user":
			_, _ = io.WriteString(w, `{"id":1,"username":"guilbert"}`)
		case strings.HasPrefix(r.URL.Path, "/api/v4/projects/"):
			_, _ = io.WriteString(w, `{"id":42400,"path_with_namespace":"scratchpad/sync-trial"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCredentialsNeverShowTheirToken(t *testing.T) {
	ts, s := newIssueServer(t)
	tok := auth.NewToken("github", "very-secret-value")
	tok.SetMetadata(auth.MetaKeyLogin, "guilbep")
	if err := auth.Store(s.b.TestCache(), tok); err != nil {
		t.Fatal(err)
	}
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
		creds[0].Target != "github" || creds[0].Host != "github.com" || creds[0].Login != "guilbep" || len(creds[0].ID) != 7 {
		t.Errorf("credentials %s (%v)", body, err)
	}
}

func TestAddAndRemoveABridge(t *testing.T) {
	ts, s := newIssueServer(t)
	gl := fakeGitLab(t)
	project := gl.URL + "/scratchpad/sync-trial"

	code, refused := callError(t, "POST", ts.URL+"/api/bridges", map[string]string{"target": "gitlab", "url": project, "token": "wrong"})
	if code != http.StatusBadRequest || !strings.Contains(refused, "refused the token") {
		t.Errorf("a refused token: %d %q, want 400 saying it isn't added", code, refused)
	}
	t.Logf("refused: %s", refused)

	var added struct{ Name string }
	if code := call(t, "POST", ts.URL+"/api/bridges", map[string]string{"target": "gitlab", "url": project, "token": "good-token"}, &added); code != http.StatusOK {
		t.Fatalf("add: %d", code)
	}
	if added.Name != "gl" {
		t.Errorf("default name %q, want gl", added.Name)
	}
	bridges, _ := s.b.Bridges()
	if len(bridges) != 1 || !bridges[0].Sync || bridges[0].Target != "gitlab" {
		t.Errorf("bridges after add: %+v", bridges)
	}
	creds, _ := s.b.Credentials()
	if len(creds) != 1 || creds[0].Login != "guilbert" {
		t.Errorf("the pasted token isn't stored as a credential: %+v", creds)
	}

	// again, with the stored credential: a second name
	if code := call(t, "POST", ts.URL+"/api/bridges", map[string]string{"target": "gitlab", "url": project, "credential": creds[0].ID}, &added); code != http.StatusOK || added.Name != "gl2" {
		t.Errorf("add with the stored credential: %d %q, want gl2", code, added.Name)
	}

	if code := call(t, "DELETE", ts.URL+"/api/bridges/gl", nil, nil); code != http.StatusOK {
		t.Errorf("remove: %d", code)
	}
	if code := call(t, "DELETE", ts.URL+"/api/bridges/nope", nil, nil); code != http.StatusNotFound {
		t.Errorf("remove an unknown bridge: %d, want 404", code)
	}
	bridges, _ = s.b.Bridges()
	if len(bridges) != 1 || bridges[0].Name != "gl2" {
		t.Errorf("bridges after remove: %+v", bridges)
	}

	s.syncing.Store(true)
	if code := call(t, "DELETE", ts.URL+"/api/bridges/gl2", nil, nil); code != http.StatusConflict {
		t.Errorf("during a forge sync: %d, want 409", code)
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
	for raw, want := range map[string]string{
		"GET https://gitlab.epfl.ch/api/v4/user: 401 {message: 401 Unauthorized}":             "refused the token",
		"non-200 OK status code: 401 Unauthorized body: Bad credentials":                      "refused the token",
		"project validation: wrong token scope ou non-existent project: 404":                  "doesn't know this project",
		"project doesn't exist or authentication token has an incorrect scope":                "doesn't know this project",
		`Get "https://nope.example/api/v4/user": dial tcp: lookup nope.example: no such host`: "can't reach the forge",
		"credential doesn't have a login":                                                     "credential doesn't have a login",
	} {
		if got := plainBridgeError(errors.New(raw)).Error(); !strings.Contains(got, want) {
			t.Errorf("%q → %q, want it to say %q", raw, got, want)
		}
	}
}
