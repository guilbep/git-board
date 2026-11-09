package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// FakeGitHubLogin serves github.com's device flow and the API's /user on
// loopback: a code, one pending poll, then token for login. Tests that
// use it swap package variables, so they stay serial.
func fakeGitHubLogin(t *testing.T, token, login string) *httptest.Server {
	t.Helper()
	polls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/device/code", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"device_code": "dc", "user_code": "ABCD-1234", "verification_uri": "https://github.com/login/device", "expires_in": 900, "interval": 1})
	})
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		if polls++; polls == 1 {
			json.NewEncoder(w).Encode(map[string]any{"error": "authorization_pending"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": token})
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token && r.Header.Get("Authorization") != "token "+token {
			http.Error(w, "bad credentials", 401)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"login": login, "id": 42})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// The device flow stores the token GitHub gives under the login GitHub
// says it is, with no login typed; with no OAuth App in the build, the
// forge says it cannot.
func TestDeviceLogin(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://github.com/o/r.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if _, err := b.StartDevice(t.Context(), "github"); err == nil {
		t.Fatal("a build with no OAuth App started a device flow")
	}

	srv := fakeGitHubLogin(t, "gho_secret", "ada")
	cid, api, login, approve := GitHubClientID, GitHubAPI, GitHubLogin, ApproveToken
	t.Cleanup(func() { GitHubClientID, GitHubAPI, GitHubLogin, ApproveToken = cid, api, login, approve })
	var stored string
	GitHubClientID, GitHubAPI, GitHubLogin = "cid", srv.URL, srv.URL
	ApproveToken = func(_ context.Context, baseURL, login, token string) error {
		stored = baseURL + " " + login + " " + token
		return nil
	}
	sts, err := b.ForgeStatuses(t.Context(), false)
	if err != nil || len(sts) != 1 || !sts[0].Device {
		t.Fatalf("statuses %+v, %v: want one that logs in in the browser", sts, err)
	}
	d, err := b.StartDevice(t.Context(), sts[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	if d.UserCode != "ABCD-1234" {
		t.Fatalf("code %q", d.UserCode)
	}
	who, err := d.Wait(t.Context())
	if err != nil || who != "ada" || stored != "https://github.com ada gho_secret" {
		t.Fatalf("logged in as %q (%v), stored %q", who, err, stored)
	}
}

// A forge found in origin gets its table under the name GitHub gives it,
// as a sync keys its records, not as origin spells it; with no token, as
// origin spells it. Serial: it sets the environment and GitHubAPI.
func TestFoundForgeWrittenAsGitHubNamesIt(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"full_name": "O/R"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	api := GitHubAPI
	t.Cleanup(func() { GitHubAPI = api })
	GitHubAPI = srv.URL

	for _, tc := range []struct{ token, want string }{{"", `project = "o/r"`}, {"t", `project = "O/R"`}} {
		t.Setenv("GITHUB_TOKEN", tc.token)
		dir := t.TempDir()
		for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://github.com/o/r.git"}} {
			if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		b, err := Open(t.Context(), dir, Options{})
		if err != nil {
			t.Fatal(err)
		}
		err = b.SetForgeWay(t.Context(), "github", "pull")
		b.Close()
		if err != nil {
			t.Fatalf("token %q: %v", tc.token, err)
		}
		data, _ := os.ReadFile(filepath.Join(dir, ".git-board.toml"))
		if !strings.Contains(string(data), tc.want) || !strings.Contains(string(data), `way = "pull"`) {
			t.Errorf("token %q: want %s:\n%s", tc.token, tc.want, data)
		}
	}
}
