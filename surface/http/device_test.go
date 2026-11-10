package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/app"
)

// Routes 62 and 63: the device flow from the page. The page gets the code
// to type and the outcome; the device code and the token stay in the
// server, never answered nor logged. Serial: it swaps package variables.
func TestDeviceRoutes(t *testing.T) {
	const token = "gho_s3cret"
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/device/code", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"device_code": "dev-s3cret", "user_code": "ABCD-1234", "verification_uri": "https://github.com/login/device", "expires_in": 900, "interval": 1})
	})
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": token})
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"login": "ada", "id": 42})
	})
	gh := httptest.NewServer(mux)
	defer gh.Close()
	cid, api, login, approve := app.GitHubClientID, app.GitHubAPI, app.GitHubLogin, app.ApproveToken
	t.Cleanup(func() {
		app.GitHubClientID, app.GitHubAPI, app.GitHubLogin, app.ApproveToken = cid, api, login, approve
	})
	app.GitHubClientID, app.GitHubAPI, app.GitHubLogin = "cid", gh.URL, gh.URL
	stored := make(chan string, 1)
	app.ApproveToken = func(_ context.Context, baseURL, login, token string) error {
		stored <- baseURL + " " + login + " " + token
		return nil
	}

	dir, _ := newBoard(t)
	gitRun(t, dir, "remote", "add", "origin", "https://github.com/o/r.git")
	a := newAPI(t, dir)
	if got := want[deviceRes](t, a.do("GET", "/api/forges/auth/device", nil), http.StatusOK); got.State != "none" {
		t.Fatalf("before: %+v", got)
	}
	auths := want[items[app.ForgeStatus]](t, a.do("GET", "/api/forges/auth", nil), http.StatusOK)
	if len(auths.Items) != 1 || !auths.Items[0].Device {
		t.Fatalf("forges %+v, want one that logs in in the browser", auths.Items)
	}
	res := a.do("POST", "/api/forges/auth/device", map[string]any{"forge": auths.Items[0].Name})
	got := want[deviceRes](t, res, http.StatusAccepted)
	if got.State != "waiting" || got.UserCode != "ABCD-1234" || got.URL == "" {
		t.Fatalf("started: %+v", got)
	}
	select {
	case s := <-stored:
		if s != "https://github.com ada "+token {
			t.Fatalf("stored %q", s)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no token stored")
	}
	deadline := time.Now().Add(5 * time.Second)
	for got.State == "waiting" && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		got = want[deviceRes](t, a.do("GET", "/api/forges/auth/device", nil), http.StatusOK)
	}
	if got.State != "done" || got.Login != "ada" {
		t.Fatalf("after: %+v", got)
	}
	want[map[string]any](t, a.do("POST", "/api/forges/auth/device", map[string]any{"forge": "nope"}), http.StatusNotFound)
	if logs := a.logs(); strings.Contains(logs, "s3cret") {
		t.Errorf("a secret is logged:\n%s", logs)
	}
}
