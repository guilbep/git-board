package server

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/store/storetest"
)

// /api/me says where pictures come from, and Settings sets it here.
func TestSetAvatarsFromSettings(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	var me meResponse
	if call(t, "GET", ts.URL+"/api/me", nil, &me); me.Avatars != "forge" || me.AvatarsScope != "" {
		t.Errorf("by default: %q (%q)", me.Avatars, me.AvatarsScope)
	}
	if code := call(t, "PUT", ts.URL+"/api/me/avatars", map[string]string{"avatars": "gravatar"}, &me); code != 200 || me.Avatars != "gravatar" || me.AvatarsScope != "local" {
		t.Fatalf("gravatar: %d, %q (%q)", code, me.Avatars, me.AvatarsScope)
	}
	if out := storetest.Git(t, s.b.GitDir(), "config", "--local", "board.avatars"); strings.TrimSpace(out) != "gravatar" {
		t.Errorf("git config says %q", out)
	}
	if code := call(t, "PUT", ts.URL+"/api/me/avatars", map[string]string{"avatars": "faces"}, nil); code != 400 {
		t.Errorf("faces: %d, want 400", code)
	}
	if code := call(t, "PUT", ts.URL+"/api/me/avatars", map[string]string{"avatars": ""}, &me); code != 200 || me.Avatars != "forge" {
		t.Errorf("back to the default: %d, %q", code, me.Avatars)
	}
}

func raw(t *testing.T, url string) string {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("GET %s: %d %s", url, res.StatusCode, b)
	}
	return string(b)
}

// The email stays on the server, whatever the mode: the page gets a URL
// (under gravatar, the email's hash), never the address.
func TestMeNeverSendsEmail(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	const email = "ann.secret@epfl.ch"
	if code := call(t, "POST", ts.URL+"/api/me", map[string]string{"name": "Ann", "email": email}, nil); code != 200 {
		t.Fatalf("POST /api/me: %d", code)
	}
	id := storetest.NewIssue(t, s.b, "Export CSV")
	for _, m := range []string{"forge", "gravatar", "none"} {
		if code := call(t, "PUT", ts.URL+"/api/me/avatars", map[string]string{"avatars": m}, nil); code != 200 {
			t.Fatalf("%s: %d", m, code)
		}
		for _, u := range []string{"/api/me", "/api/issues/" + id, "/api/issues/" + id + "/thread"} {
			body := raw(t, ts.URL+u)
			if strings.Contains(strings.ToLower(body), "secret@epfl") {
				t.Errorf("%s, %s: the email is in %s", m, u, body)
			}
			if gravatar := strings.Contains(body, "gravatar.com"); gravatar != (m == "gravatar") && u == "/api/me" {
				t.Errorf("%s, %s: gravatar.com in the answer: %v", m, u, gravatar)
			}
			if m == "none" && strings.Contains(body, "avatar_url") {
				t.Errorf("none, %s: an avatar_url", u)
			}
		}
	}
}
