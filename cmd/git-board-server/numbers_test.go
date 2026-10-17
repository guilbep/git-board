package main

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/git-bug/git-bug/repository/repotest"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// Nothing goes to origin unless the user asks: creating an issue numbers
// nothing and pushes nothing, even in a project that numbers its issues;
// the push the user asks for numbers it.
func TestNumbersOnlyOnAPushTheUserAsks(t *testing.T) {
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
