package main

import (
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// git runs git in dir, away from the user's own configuration and hooks.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=dev", "-c", "user.email=dev@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

type branchesResponse struct {
	Suggested string `json:"suggested"`
	Base      string `json:"base"`
	Start     string `json:"start"`
	Command   string `json:"command"`
	Branches  []struct {
		Name       string `json:"name"`
		Remote     string `json:"remote"`
		Ahead      int    `json:"ahead"`
		Behind     int    `json:"behind"`
		Merged     bool   `json:"merged"`
		LastCommit struct {
			SHA     string `json:"sha"`
			Subject string `json:"subject"`
			Author  string `json:"author"`
		} `json:"last_commit"`
	} `json:"branches"`
}

// A repository with code: dev (origin's default branch), a branch merged
// into it, one ahead of it, and branches naming other issues.
func TestBranchesOfAnIssue(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "dev")
	git(t, dir, "config", "user.name", "dev")
	git(t, dir, "config", "user.email", "dev@example.com")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "start")
	git(t, dir, "switch", "-q", "-c", "fix/2518-merged")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "Fix the example")
	git(t, dir, "switch", "-q", "dev")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "Merge fix/2518-merged", "fix/2518-merged")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "Later on dev")
	git(t, dir, "switch", "-q", "-c", "2518-ahead")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "One")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "Two\n\nthe body")
	git(t, dir, "switch", "-q", "dev")
	git(t, dir, "branch", "12518-another-issue")
	git(t, dir, "update-ref", "refs/remotes/origin/dev", "dev")
	git(t, dir, "update-ref", "refs/remotes/origin/fix/2518-merged", "fix/2518-merged")
	git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/dev")

	b, err := gitbugio.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	who, err := b.TestCache().Identities().NewFromGitUser()
	if err != nil {
		t.Fatal(err)
	}
	if err := b.TestCache().SetUserIdentity(who); err != nil {
		t.Fatal(err)
	}
	id := gitbugiotest.NewImportedIssue(t, b, "[BUG](Equipment) Crash on save (#2518)", "https://github.com/o/r/issues/2518")
	git(t, dir, "branch", id[:7]+"-by-id")
	// git-bug's own copy of the issue at origin names it too: not a branch
	git(t, dir, "update-ref", "refs/remotes/origin/bugs/"+id, "refs/bugs/"+id)

	ts := newTestServer(t, b, &config.Config{})
	var res branchesResponse
	if resp := getJSON(t, ts.URL+"/api/issues/"+id[:8]+"/branches", &res); resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if res.Suggested != "2518-crash-on-save" || res.Base != "dev" || res.Start != "origin/dev" || res.Command != "git switch -c 2518-crash-on-save origin/dev" {
		t.Errorf("suggestion %+v", res)
	}
	got := map[string]int{}
	for i, br := range res.Branches {
		got[br.Remote+":"+br.Name] = i
	}
	want := []string{":2518-ahead", ":" + id[:7] + "-by-id", ":fix/2518-merged", "origin:fix/2518-merged"}
	for _, k := range want {
		if _, ok := got[k]; !ok || len(got) != len(want) {
			t.Fatalf("branches %v, want %q", got, want)
		}
	}
	if got[":fix/2518-merged"] > got["origin:fix/2518-merged"] {
		t.Errorf("the remote's before the local branch: %v", got)
	}
	ahead, byID, merged := res.Branches[got[":2518-ahead"]], res.Branches[got[":"+id[:7]+"-by-id"]], res.Branches[got[":fix/2518-merged"]]
	if ahead.Ahead != 2 || ahead.Behind != 0 || ahead.Merged || ahead.LastCommit.Subject != "Two" || ahead.LastCommit.Author != "dev" || len(ahead.LastCommit.SHA) != 40 {
		t.Errorf("ahead %+v", ahead)
	}
	if byID.Ahead != 0 || byID.Behind != 0 || !byID.Merged {
		t.Errorf("by id %+v", byID)
	}
	if merged.Ahead != 0 || merged.Behind != 2 || !merged.Merged || merged.LastCommit.Subject != "Fix the example" {
		t.Errorf("merged %+v", merged)
	}
}

// No code yet, no remote: still a name, and the command from main.
func TestBranchesWithoutCode(t *testing.T) {
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "Write specs")
	ts := newTestServer(t, b, &config.Config{Branches: config.Branches{Name: "{kind}/{number}-{slug}"}})
	var res branchesResponse
	if resp := getJSON(t, ts.URL+"/api/issues/"+id+"/branches", &res); resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if want := id[:7] + "-write-specs"; res.Suggested != want || res.Base != "main" || res.Command != "git switch -c "+want+" origin/main" {
		t.Errorf("suggestion %+v", res)
	}
	if res.Branches == nil || len(res.Branches) != 0 {
		t.Errorf("branches %v, want []", res.Branches)
	}
}

func TestBranchesOfNoSuchIssue(t *testing.T) {
	ts := newTestServer(t, gitbugiotest.New(t), &config.Config{})
	if resp := getJSON(t, ts.URL+"/api/issues/deadbeef/branches", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
}
