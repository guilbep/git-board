package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/git-bug/git-bug/bridge/core"
	"github.com/git-bug/git-bug/repository/repotest"
	"github.com/git-bug/gitconfig"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// Nothing goes to origin unless the user asks: creating an issue numbers
// nothing and pushes nothing, even in a project that numbers its issues;
// the push the user asks for numbers it.
func TestNumbersOnlyOnAPushTheUserAsks(t *testing.T) {
	t.Parallel()
	remote := repotest.NewGoGitRepo(t, true)
	clone := repotest.NewGoGitRepo(t, false)
	repotest.AddRemote(t, clone, "origin", remote.GitDir)
	b, err := gitbugio.NewForRepo(clone)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	id, err := b.TestCache().Identities().NewFromGitUser()
	if err != nil {
		t.Fatal(err)
	}
	if err := b.TestCache().SetUserIdentity(id); err != nil {
		t.Fatal(err)
	}
	if err := b.InitNumbers("origin", "CO2", 1); err != nil {
		t.Fatal(err)
	}
	onOrigin := func() string {
		t.Helper()
		out, err := exec.Command("git", "--git-dir", remote.GitDir, "cat-file", "-p", "refs/board/seq:numbers").CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return string(out)
	}

	s := &apiServer{cfg: &config.Config{}, b: b, me: "guilbep"}
	ts := httptest.NewServer(withCORS(newMux(s, http.NotFoundHandler())))
	t.Cleanup(ts.Close)

	var made struct {
		ID     string `json:"id"`
		Number string `json:"number"`
	}
	if code := call(t, "POST", ts.URL+"/api/issues", map[string]string{"title": "Made here"}, &made); code != http.StatusCreated || made.Number != "" {
		t.Fatalf("create: %d, number %q", code, made.Number)
	}
	time.Sleep(time.Second) // anything numbering on its own has had the time
	if got := onOrigin(); strings.Contains(got, made.ID) {
		t.Fatalf("creating pushed the counter:\n%s", got)
	}

	if code := call(t, "POST", ts.URL+"/api/remotes/origin/push", nil, nil); code != http.StatusOK {
		t.Fatalf("push: %d", code)
	}
	if got := onOrigin(); !strings.Contains(got, "1 "+made.ID) {
		t.Errorf("the push numbered nothing:\n%s", got)
	}
}

// numbersServer serves b, GitHub answering highest (or failing, when
// highest < 0) through asked.
func numbersServer(t *testing.T, b *gitbugio.Backend, highest int, asked *int) *httptest.Server {
	t.Helper()
	s := &apiServer{cfg: &config.Config{}, b: b, githubHighest: func(_ context.Context, repo string) (int, error) {
		*asked++
		if repo != "o/r" {
			t.Errorf("GitHub asked about %q, want o/r", repo)
		}
		if highest < 0 {
			return 0, errors.New("gh: not logged in")
		}
		return highest, nil
	}}
	ts := httptest.NewServer(withCORS(newMux(s, http.NotFoundHandler())))
	t.Cleanup(ts.Close)
	return ts
}

func githubForge(t *testing.T, b *gitbugio.Backend) {
	t.Helper()
	for k, v := range map[string]string{core.ConfigKeyTarget: "github", "owner": "o", "project": "r"} {
		if err := core.SetBridgeConfig(b.TestCache(), "gh", k, v); err != nil {
			t.Fatal(err)
		}
	}
}

// Settings › Numbers: where the numbers stand, what to start them with,
// starting them, and numbering what origin has. GitHub is asked only when
// the form asks, before the switch.
func TestNumbersFromSettings(t *testing.T) {
	t.Parallel()
	mine, theirs := gitbugiotest.NewPair(t)
	githubForge(t, mine)
	asked := 0
	ts := numbersServer(t, mine, 3051, &asked)
	gitbugiotest.NewImportedIssue(t, mine, "Old", "https://github.com/o/r/issues/2518")
	gitbugiotest.NewIssue(t, mine, "Made here")
	if _, err := mine.Push("origin"); err != nil {
		t.Fatal(err)
	}

	var n numbersResponse
	if code := call(t, "GET", ts.URL+"/api/numbers", nil, &n); code != http.StatusOK || n.On || n.Waiting != 2 || n.Highest != 2518 || n.Name != "r" || n.GitHub != nil || asked != 0 {
		t.Fatalf("before: %d %+v, asked GitHub %d times", code, n, asked)
	}
	if n.Origin == nil || len(n.Origin.PushURLs) != 1 {
		t.Fatalf("origin %+v", n.Origin)
	}
	call(t, "GET", ts.URL+"/api/numbers?github=1", nil, &n)
	if n.GitHub == nil || n.GitHub.Highest != 3051 || n.GitHub.Issues != "https://github.com/o/r/issues" || asked != 1 {
		t.Fatalf("the form: GitHub %+v, asked %d", n.GitHub, asked)
	}

	for _, bad := range []map[string]any{{"prefix": "co-2", "start": 3052}, {"prefix": "CO2", "start": 0}} {
		if code, msg := callError(t, "POST", ts.URL+"/api/numbers/init", bad); code != http.StatusBadRequest || msg == "" {
			t.Errorf("init %v: %d %q, want 400", bad, code, msg)
		}
	}
	if code := call(t, "POST", ts.URL+"/api/numbers/init", map[string]any{"prefix": "CO2", "start": 3052}, &n); code != http.StatusOK ||
		!n.On || n.Prefix != "CO2" || n.Start != 3052 || n.Next != 3053 || n.Done != 1 || n.Numbered != 2 || n.Waiting != 0 || n.Error != "" {
		t.Fatalf("init: %d %+v", code, n)
	}
	if code, msg := callError(t, "POST", ts.URL+"/api/numbers/init", map[string]any{"prefix": "XY", "start": 1}); code != http.StatusConflict || !strings.Contains(msg, "CO2, from 3052") {
		t.Errorf("init again: %d %q, want 409", code, msg)
	}
	if code, _ := callError(t, "POST", numbersServer(t, theirs, 0, &asked).URL+"/api/numbers/init", map[string]any{"prefix": "XY", "start": 1}); code != http.StatusConflict {
		t.Errorf("init from the other clone: %d, want 409", code)
	}
	var after numbersResponse
	call(t, "GET", ts.URL+"/api/numbers?github=1", nil, &after)
	if asked != 1 || after.GitHub != nil || after.Numbering.GitHub != nil {
		t.Errorf("after the switch: asked GitHub %d times, %+v", asked, after)
	}

	// one more, pushed on its own: waiting; one only here: to push
	gitbugiotest.NewIssue(t, mine, "Pushed, not numbered")
	if _, err := mine.Push("origin"); err != nil {
		t.Fatal(err)
	}
	gitbugiotest.NewIssue(t, mine, "Only here")
	if call(t, "GET", ts.URL+"/api/numbers", nil, &n); n.Waiting != 1 || n.Unpushed != 1 {
		t.Fatalf("before running: %+v", n)
	}
	if code := call(t, "POST", ts.URL+"/api/numbers/run", nil, &n); code != http.StatusOK || n.Done != 1 || n.Waiting != 0 || n.Unpushed != 1 || n.Next != 3054 {
		t.Fatalf("run: %d %+v", code, n)
	}
}

// Without origin, nothing starts; GitHub unreachable is said, not an
// error; origin's address goes to the page without its credentials.
func TestNumbersWithoutOrigin(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	githubForge(t, b)
	asked := 0
	ts := numbersServer(t, b, -1, &asked)
	var n numbersResponse
	if code := call(t, "GET", ts.URL+"/api/numbers?github=1", nil, &n); code != http.StatusOK || n.Origin != nil || n.GitHub == nil || n.GitHub.Error == "" || n.GitHub.Highest != 0 {
		t.Fatalf("no origin, GitHub unreachable: %d %+v %+v", code, n, n.GitHub)
	}
	for _, path := range []string{"/api/numbers/init", "/api/numbers/run"} {
		if code, msg := callError(t, "POST", ts.URL+path, map[string]any{"prefix": "CO2", "start": 1}); code != http.StatusConflict || !strings.Contains(msg, "origin") {
			t.Errorf("%s without origin: %d %q, want 409", path, code, msg)
		}
		if code := send(t, "POST", ts.URL+path, "https://evil.example"); code != http.StatusForbidden {
			t.Errorf("%s asked by another site: %d, want 403", path, code)
		}
	}

	// GET only: nothing reaches this address
	if err := b.TestCache().Config().Update(func(f *gitconfig.File) error {
		return f.Set("remote.origin.url", "https://oauth2:glpat-secret@gitlab.example.com/g/co2-calculator.git")
	}); err != nil {
		t.Fatal(err)
	}
	call(t, "GET", ts.URL+"/api/numbers", nil, &n)
	if n.Origin == nil || fmt.Sprint(n.Origin.PushURLs) != "[https://gitlab.example.com/g/co2-calculator.git]" {
		t.Errorf("origin %+v", n.Origin)
	}
}

func TestNumbersEvents(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/api/numbers/init", "/api/numbers/run"} {
		if got := eventFor(httptest.NewRequest("POST", path, nil)).Kind; got != "issues" {
			t.Errorf("%s: %s, want issues: the boards show the numbers", path, got)
		}
	}
}
