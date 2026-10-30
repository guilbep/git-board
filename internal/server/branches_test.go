package server

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
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
		InBase     bool   `json:"in_base"`
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
	t.Parallel()
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

	b, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if _, err := b.NewIdentity("dev", "dev@example.com"); err != nil {
		t.Fatal(err)
	}
	id := storetest.NewImportedIssue(t, b, "[BUG](Equipment) Crash on save (#2518)", "https://github.com/o/r/issues/2518")
	git(t, dir, "branch", id[:7]+"-by-id")
	// git-bug's own copy of an issue at origin, an older git-board's: not a branch
	git(t, dir, "update-ref", "refs/remotes/origin/bugs/"+strings.Repeat("ab", 32), "dev")

	ts := newTestServer(t, b, &config.Config{})
	var res branchesResponse
	if resp := getJSON(t, ts.URL+"/api/issues/"+id[:8]+"/branches", &res); resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if res.Suggested != "2518/1-crash-on-save" || res.Base != "dev" || res.Start != "origin/dev" || res.Command != "git switch -c 2518/1-crash-on-save origin/dev" {
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

// A branch squashed into dev isn't its ancestor, but its changes are in
// dev: in_base says so, from what git merge-tree makes of the two.
func TestBranchSquashedIntoTheBase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		git(t, dir, "add", name)
	}
	git(t, dir, "init", "-q", "-b", "dev")
	git(t, dir, "config", "user.name", "dev")
	git(t, dir, "config", "user.email", "dev@example.com")
	write("a.txt")
	git(t, dir, "commit", "-q", "-m", "start")
	git(t, dir, "switch", "-q", "-c", "2518-squashed")
	write("b.txt")
	git(t, dir, "commit", "-q", "-m", "Fix it")
	git(t, dir, "switch", "-q", "-c", "2518-open", "dev")
	write("c.txt")
	git(t, dir, "commit", "-q", "-m", "Not landed")
	git(t, dir, "switch", "-q", "dev")
	git(t, dir, "merge", "-q", "--squash", "2518-squashed")
	git(t, dir, "commit", "-q", "-m", "Fix it (#2518)")
	git(t, dir, "update-ref", "refs/remotes/origin/dev", "dev")
	git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/dev")

	b, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if _, err := b.NewIdentity("dev", "dev@example.com"); err != nil {
		t.Fatal(err)
	}
	id := storetest.NewImportedIssue(t, b, "Crash on save (#2518)", "https://github.com/o/r/issues/2518")

	ts := newTestServer(t, b, &config.Config{})
	var res branchesResponse
	if resp := getJSON(t, ts.URL+"/api/issues/"+id[:8]+"/branches", &res); resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	by := map[string]int{}
	for i, br := range res.Branches {
		by[br.Name] = i
	}
	if len(res.Branches) != 2 {
		t.Fatalf("branches %+v", res.Branches)
	}
	squashed, open := res.Branches[by["2518-squashed"]], res.Branches[by["2518-open"]]
	if squashed.Merged || !squashed.InBase || squashed.Ahead != 1 {
		t.Errorf("squashed %+v", squashed)
	}
	if open.Merged || open.InBase || open.Ahead != 1 {
		t.Errorf("open %+v", open)
	}
}

// No code yet, no remote: still a name, and the command from main.
func TestBranchesWithoutCode(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "Write specs")
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
	t.Parallel()
	ts := newTestServer(t, storetest.New(t), &config.Config{})
	if resp := getJSON(t, ts.URL+"/api/issues/deadbeef/branches", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
}

type createAnswer struct {
	branchesResponse
	Created   string `json:"created"`
	Pushed    bool   `json:"pushed"`
	PushError string `json:"push_error"`
}

// A clone of origin (a bare repository) on dev, and issue #2518.
func cloneWithOrigin(t *testing.T) (dir, origin, id string, b *store.Backend) {
	t.Helper()
	root := t.TempDir()
	origin, dir = filepath.Join(root, "origin.git"), filepath.Join(root, "clone")
	git(t, root, "init", "-q", "--bare", "-b", "dev", origin)
	git(t, root, "init", "-q", "-b", "dev", dir)
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "start")
	git(t, dir, "remote", "add", "origin", origin)
	git(t, dir, "push", "-q", "origin", "dev")
	git(t, dir, "fetch", "-q", "origin")
	git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/dev")
	b, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if _, err := b.NewIdentity("dev", "dev@example.com"); err != nil {
		t.Fatal(err)
	}
	id = storetest.NewImportedIssue(t, b, "Crash on save (#2518)", "https://github.com/o/r/issues/2518")
	return dir, origin, id, b
}

// Create makes the suggested branch in the issue's folder, pushes it, and
// leaves the clone's checkout alone; the next one is /2-.
func TestCreateBranch(t *testing.T) {
	t.Parallel()
	dir, origin, id, b := cloneWithOrigin(t)
	ts := newTestServer(t, b, &config.Config{})
	url := ts.URL + "/api/issues/" + id[:8] + "/branches"

	code, c, body := sendFrom(t, "POST", url, ts.URL, map[string]string{})
	var res createAnswer
	if err := json.Unmarshal(body, &res); err != nil || code != http.StatusOK {
		t.Fatalf("create: %d %q %s", code, c, body)
	}
	if res.Created != "2518/1-crash-on-save" || !res.Pushed || res.PushError != "" {
		t.Fatalf("create: %+v", res)
	}
	git(t, origin, "rev-parse", "--verify", "-q", "refs/heads/2518/1-crash-on-save")
	git(t, dir, "rev-parse", "--verify", "-q", "refs/heads/2518/1-crash-on-save")
	if head := git(t, dir, "symbolic-ref", "--short", "HEAD"); head != "dev" {
		t.Errorf("the checkout moved to %q", head)
	}
	if res.Suggested != "2518/2-crash-on-save" {
		t.Errorf("next suggestion %q, want /2-", res.Suggested)
	}
	found := false
	for _, br := range res.Branches {
		found = found || br.Name == res.Created
	}
	if !found {
		t.Errorf("the new branch isn't listed: %+v", res.Branches)
	}

	code, c, body = sendFrom(t, "POST", url, ts.URL, map[string]string{"name": "2518/2-reference-examples"})
	if err := json.Unmarshal(body, &res); err != nil || code != http.StatusOK || res.Created != "2518/2-reference-examples" || !res.Pushed {
		t.Fatalf("a named one: %d %q %s", code, c, body)
	}
	for _, tc := range []struct {
		name, origin string
		status       int
		code         string
	}{
		{"2518/1-crash-on-save", ts.URL, http.StatusConflict, "exists"},
		{"2600/1-another-issue", ts.URL, http.StatusBadRequest, "not-this-issue"},
		{"2518/3 bad name", ts.URL, http.StatusBadRequest, "bad-name"},
		{"-2518", ts.URL, http.StatusBadRequest, "bad-name"},
		{"2518/3-x", "", http.StatusForbidden, "origin"},
		{"2518/3-x", "https://elsewhere.example", http.StatusForbidden, ""}, // the guard's, before ours
	} {
		if code, c, body := sendFrom(t, "POST", url, tc.origin, map[string]string{"name": tc.name}); code != tc.status || c != tc.code {
			t.Errorf("%q from %q: %d %q, want %d %q (%s)", tc.name, tc.origin, code, c, tc.status, tc.code, body)
		}
	}
	if out := git(t, dir, "branch", "--list", "2518/3*"); out != "" {
		t.Errorf("a refused name made a branch: %q", out)
	}
}

// A server listening beyond this machine makes no branch.
func TestCreateBranchRefusedExposed(t *testing.T) {
	t.Parallel()
	_, _, id, b := cloneWithOrigin(t)
	s := &apiServer{cfg: &config.Config{}, b: b, me: "guilbep", exposed: []string{"*"}}
	ts := serve(t, s)
	if code, c, _ := sendFrom(t, "POST", ts.URL+"/api/issues/"+id[:8]+"/branches", ts.URL, map[string]string{}); code != http.StatusForbidden || c != "exposed" {
		t.Errorf("exposed: %d %q", code, c)
	}
}

// origin out of reach: the branch stays local, and the answer says why.
func TestCreateBranchPushRefused(t *testing.T) {
	t.Parallel()
	dir, _, id, b := cloneWithOrigin(t)
	git(t, dir, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
	ts := newTestServer(t, b, &config.Config{})
	code, c, body := sendFrom(t, "POST", ts.URL+"/api/issues/"+id[:8]+"/branches", ts.URL, map[string]string{})
	var res createAnswer
	if err := json.Unmarshal(body, &res); err != nil || code != http.StatusOK {
		t.Fatalf("create: %d %q %s", code, c, body)
	}
	if res.Pushed || res.PushError == "" || res.Created != "2518/1-crash-on-save" {
		t.Fatalf("push refused: %+v", res)
	}
	git(t, dir, "rev-parse", "--verify", "-q", "refs/heads/2518/1-crash-on-save")
}
