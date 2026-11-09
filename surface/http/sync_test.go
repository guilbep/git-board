package httpapi

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/app"
)

// The git remotes at once, a whole sync in the background, its steps as
// progress events, one run at a time; the project's numbers.
func TestSyncAndRemotes(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	origin := filepath.Join(t.TempDir(), "origin.git")
	gitRun(t, "", "init", "-q", "--bare", origin)
	gitRun(t, dir, "remote", "add", "origin", origin)
	a := newAPI(t, dir)

	st := want[syncRes](t, a.do("GET", "/api/sync", nil), 200)
	if len(st.Remotes) != 1 || st.Remotes[0] != "origin" || st.Run != nil {
		t.Fatalf("GET /api/sync = %+v", st)
	}
	r := want[remoteRes](t, a.do("POST", "/api/remotes/origin/push", nil), 200)
	if r.Action != "push" {
		t.Fatalf("push = %+v", r)
	}
	r = want[remoteRes](t, a.do("POST", "/api/remotes/origin/fetch", nil), 200)
	if r.Issues == nil || r.Issues.Receive != 0 || r.Issues.Send != 0 {
		t.Fatalf("fetch after a push = %+v", r)
	}
	if e := want[apiError](t, a.do("POST", "/api/remotes/nope/pull", nil), 500); e.Message == "" {
		t.Fatalf("a remote that isn't = %+v", e)
	}

	ch := stream(t, a)
	run := want[Run](t, a.do("POST", "/api/sync", map[string]any{"scope": "all"}), 202)
	if run.State != "running" || run.ID == "" {
		t.Fatalf("POST /api/sync = %+v", run)
	}
	if e := next(t, ch, "progress"); e.ID != run.ID || e.Step == nil {
		t.Fatalf("a progress event = %+v", e)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		st = want[syncRes](t, a.do("GET", "/api/sync", nil), 200)
		if st.Run != nil && st.Run.State != "running" || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if st.Run == nil || st.Run.State != "done" || len(st.Run.Steps) == 0 {
		t.Fatalf("the run = %+v", st.Run)
	}
	if e := want[apiError](t, a.do("POST", "/api/sync", map[string]any{"scope": "everything"}), 400); e.Field != "scope" {
		t.Fatalf("a scope that isn't = %+v", e)
	}
	if e := want[apiError](t, a.do("POST", "/api/sync", map[string]any{"scope": "forges"}), 202); e.Code != "" {
		t.Fatalf("a forge run = %+v", e)
	}

	n := want[numbersRes](t, a.do("POST", "/api/numbers", map[string]any{"prefix": "T", "start": 1}), 200)
	if n.Where != "on origin" {
		t.Fatalf("numbers init = %+v", n)
	}
	n = want[numbersRes](t, a.do("POST", "/api/numbers", map[string]any{}), 200)
	if n.Numbered != 2 {
		t.Fatalf("numbering = %+v, want the 2 issues", n)
	}
	page := want[issuePage](t, a.do("GET", "/api/issues?filter=T-1", nil), 200)
	if page.Total != 1 {
		t.Fatalf("T-1 = %+v", page)
	}
}

// One run at a time: a second while one runs is busy.
func TestSyncBusy(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	a.s.runMu.Lock()
	a.s.run = &Run{ID: "1", State: "running"}
	a.s.runMu.Unlock()
	if e := want[apiError](t, a.do("POST", "/api/sync", map[string]any{}), 409); e.Code != "busy" {
		t.Fatalf("a second run = %+v", e)
	}
}

// A copy into another datastore, on loopback: a Markdown folder.
func TestDatastoreCopy(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	k := want[keptRes](t, a.do("GET", "/api/datastore", nil), 200)
	if k.Name != "git" || !k.Replica {
		t.Fatalf("GET /api/datastore = %+v", k)
	}
	folder := filepath.Join(t.TempDir(), "board")
	c := want[copiedRes](t, a.do("POST", "/api/datastore/copy", map[string]any{"to": "markdown:" + folder}), 200)
	if c.Entities != 4 { // Ada and three issues, the deleted one too: history keeps it
		t.Fatalf("copy = %+v", c)
	}
	if e := want[apiError](t, a.do("POST", "/api/datastore/copy", map[string]any{}), 400); e.Field != "to" {
		t.Fatalf("a copy into git, from git = %+v", e)
	}
}

// Route 28: the datastore chosen from the page. A value no datastore
// opens is refused, and the clone keeps its board; a good one is kept
// in the clone's git config, and git is its default: the key unset.
func TestDatastoreChoose(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	key := func() string {
		out, _ := exec.Command("git", "-C", dir, "config", "--local", "--get", "board.datastore").Output()
		return strings.TrimSpace(string(out))
	}
	if e := want[apiError](t, a.do("PUT", "/api/datastore", map[string]any{"value": "nope:x"}), 400); e.Field != "value" {
		t.Fatalf("a bad value = %+v", e)
	}
	if k := want[keptRes](t, a.do("GET", "/api/datastore", nil), 200); k.Name != "git" || key() != "" {
		t.Fatalf("after a refusal: %+v, key %q", k, key())
	}
	folder := filepath.Join(t.TempDir(), "board")
	want[copiedRes](t, a.do("POST", "/api/datastore/copy", map[string]any{"to": "markdown:" + folder}), 200)
	if k := want[keptRes](t, a.do("PUT", "/api/datastore", map[string]any{"value": " markdown:" + folder + " "}), 200); k.Name != "Markdown files" || key() != "markdown:"+folder {
		t.Fatalf("kept in markdown: %+v, key %q", k, key())
	}
	if k := want[keptRes](t, a.do("PUT", "/api/datastore", map[string]any{"value": "git"}), 200); k.Name != "git" || key() != "" {
		t.Fatalf("back to git: %+v, key %q", k, key())
	}
}

// Route 56: the default file, written once; never over one that is there.
func TestConfigInit(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	path := filepath.Join(dir, ".git-board.toml")
	a := newAPI(t, dir)
	before := read(t, path)
	if e := want[apiError](t, a.do("POST", "/api/config/init", map[string]any{}), 409); !strings.Contains(e.Message, "already exists") {
		t.Fatalf("over a file = %+v", e)
	}
	if read(t, path) != before {
		t.Fatal("init overwrote the file")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	got := want[initRes](t, a.do("POST", "/api/config/init", map[string]any{}), 201)
	if !strings.HasSuffix(got.Path, ".git-board.toml") || !strings.Contains(read(t, path), "[") {
		t.Fatalf("init = %+v:\n%s", got, read(t, path))
	}
	want[apiError](t, a.do("POST", "/api/config/init", map[string]any{}), 409)
}

// Route 57: a git remote added from the Sync page; a name there already,
// or a URL with a space, refused.
func TestAddRemote(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	want[struct{}](t, a.do("POST", "/api/remotes/backup", map[string]any{"url": " https://example.org/o/r.git "}), 204)
	if url := strings.TrimSpace(gitRun(t, dir, "remote", "get-url", "backup")); url != "https://example.org/o/r.git" {
		t.Fatalf("the remote's URL = %q", url)
	}
	for _, bad := range []struct{ name, url string }{{"backup", "https://example.org/o/s.git"}, {"other", "https://example.org/o r.git"}, {"other", ""}} {
		if e := want[apiError](t, a.do("POST", "/api/remotes/"+bad.name, map[string]any{"url": bad.url}), 400); e.Field != "url" {
			t.Errorf("%s %q = %+v", bad.name, bad.url, e)
		}
	}
}

// Route 60: the board's state, named, with its next actions; a way of
// "both", the forge table's word, starts a run as "sync" does.
func TestState(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	got := want[app.State](t, a.do("GET", "/api/state", nil), 200)
	if got.Phase != "ready" || got.Me.State != "chosen" || got.Issues == 0 || got.Datastore.State != "git" || got.Run.State != "idle" || got.Next == nil {
		t.Fatalf("state = %+v", got)
	}
	for _, x := range got.Next {
		if x.ID == "" || x.Say == "" {
			t.Errorf("action %+v", x)
		}
	}
	if r := want[Run](t, a.do("POST", "/api/sync", map[string]any{"way": "both", "dry_run": true}), 202); r.Way != "sync" {
		t.Fatalf("run = %+v", r)
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if st := want[syncRes](t, a.do("GET", "/api/sync", nil), 200); st.Run != nil && st.Run.State != "running" {
			break
		}
	}
}
