package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
		case strings.HasSuffix(r.URL.Path, "/users") && strings.HasPrefix(r.URL.Path, "/api/v4/projects/"):
			_, _ = io.WriteString(w, `[{"username":"bob"},{"username":"alice"}]`)
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
	t.Skip("M5 phase 2: bridges and tokens")
}

func TestAddAndRemoveABridge(t *testing.T) {
	t.Skip("M5 phase 2: bridges and tokens")
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
	t.Skip("M5 phase 2: bridges and tokens")
}
