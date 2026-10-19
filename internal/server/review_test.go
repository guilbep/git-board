package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// reviewRepo: dev (origin's default), fix/2518-x on origin one commit
// ahead (a.txt line 3, a generated file), the local fix/2518-x one more
// commit ahead (not pushed), fix/9-y naming another issue; the issue #2518.
type reviewRepo struct {
	s            *apiServer
	ts           *httptest.Server
	dir, id      string
	base, c1, c2 string
}

func newReviewRepo(t *testing.T) reviewRepo {
	t.Helper()
	dir := t.TempDir()
	put := func(name, content string) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, dir, "init", "-q", "-b", "dev")
	put("a.txt", "1\n2\n3\n4\n5\n")
	put(".gitattributes", "gen/* linguist-generated\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "start")
	base := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "update-ref", "refs/remotes/origin/dev", "dev")
	git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/dev")
	git(t, dir, "branch", "fix/9-y")

	git(t, dir, "switch", "-q", "-c", "fix/2518-x")
	put("a.txt", "1\n2\nthree\n4\n5\n")
	put("gen/out.txt", "x\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "c1")
	c1 := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "update-ref", "refs/remotes/origin/fix/2518-x", c1)
	put("local.txt", "not pushed\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "c2")
	c2 := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "switch", "-q", "dev")

	b, err := gitbugio.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	who, err := b.TestCache().Identities().NewRaw("dev", "dev@example.com", "dev", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.TestCache().SetUserIdentity(who); err != nil {
		t.Fatal(err)
	}
	id := gitbugiotest.NewImportedIssue(t, b, "Crash on save (#2518)", "https://github.com/o/r/issues/2518")
	s := &apiServer{cfg: &config.Config{}, b: b, me: "guilbep"}
	return reviewRepo{s: s, ts: serve(t, s), dir: dir, id: id, base: base, c1: c1, c2: c2}
}

type changesAnswer struct {
	Branch   string `json:"branch"`
	BaseName string `json:"base_name"`
	Base     string `json:"base"`
	Head     string `json:"head"`
	From     string `json:"from"`
	To       string `json:"to"`
	Commits  []struct {
		SHA string `json:"sha"`
	} `json:"commits"`
	Files []gitbugio.FileStat `json:"files"`
}

func (r reviewRepo) url(path string, q url.Values) string {
	u := r.ts.URL + "/api/issues/" + r.id[:8] + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// The changes are origin's branch, what the pull request shows, from where
// it left the base.
func TestChanges(t *testing.T) {
	r := newReviewRepo(t)
	var c changesAnswer
	if code := call(t, "GET", r.url("/changes", nil), nil, &c); code != http.StatusOK {
		t.Fatalf("changes: %d", code)
	}
	if c.Branch != "fix/2518-x" || c.BaseName != "dev" || c.Base != r.base || c.Head != r.c1 || c.From != r.base || c.To != r.c1 || len(c.Commits) != 1 || c.Commits[0].SHA != r.c1 {
		t.Fatalf("changes = %+v", c)
	}
	gen := map[string]bool{}
	for _, f := range c.Files {
		gen[f.Path] = f.Generated
	}
	if len(gen) != 2 || gen["a.txt"] || !gen["gen/out.txt"] {
		t.Fatalf("files = %+v", c.Files)
	}

	if code := call(t, "GET", r.url("/changes", url.Values{"branch": {"fix/9-y"}}), nil, nil); code != http.StatusBadRequest {
		t.Errorf("a branch naming another issue: %d, want 400", code)
	}
	// one commit's view, against its parent: the branch's first commit's is
	// the base, as the server answers it (the old side commentable there)
	c = changesAnswer{}
	if code := call(t, "GET", r.url("/changes", url.Values{"from": {r.c1 + "^"}, "to": {r.c1}}), nil, &c); code != http.StatusOK || c.From != r.base || c.To != r.c1 {
		t.Errorf("a commit against its parent: %d, from %q, to %q", code, c.From, c.To)
	}
	if code := call(t, "GET", r.url("/changes", url.Values{"from": {"--output=x"}}), nil, nil); code != http.StatusBadRequest {
		t.Errorf("an option for from: %d, want 400", code)
	}

	var f struct {
		Path      string `json:"path"`
		Diff      string `json:"diff"`
		Truncated bool   `json:"truncated"`
	}
	if code := call(t, "GET", r.url("/changes/file", url.Values{"path": {"a.txt"}}), nil, &f); code != http.StatusOK || f.Truncated || !strings.Contains(f.Diff, "+three") {
		t.Fatalf("file: %d %+v", code, f)
	}
	if code := call(t, "GET", r.url("/changes/file", url.Values{"path": {"a.txt"}, "limit": {"3"}}), nil, &f); code != http.StatusOK || !f.Truncated || strings.Count(f.Diff, "\n") != 3 {
		t.Fatalf("file, limit 3: %d %+v", code, f)
	}
	if code := call(t, "GET", r.url("/changes/file", nil), nil, nil); code != http.StatusBadRequest {
		t.Errorf("file without a path: %d, want 400", code)
	}
}

func TestChangesWithoutBranch(t *testing.T) {
	ts, s := newIssueServer(t)
	id := gitbugiotest.NewIssue(t, s.b, "no code")
	if code := call(t, "GET", ts.URL+"/api/issues/"+id[:8]+"/changes", nil, nil); code != http.StatusNotFound {
		t.Errorf("changes of an issue without a branch: %d, want 404", code)
	}
}

// eventOf is the thread event with that id.
func eventOf(t *testing.T, th gitbugio.Thread, id string) gitbugio.Event {
	t.Helper()
	for _, e := range th.Events {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("no event %s", id)
	return gitbugio.Event{}
}

// last is the thread's newest event.
func last(th gitbugio.Thread) gitbugio.Event { return th.Events[len(th.Events)-1] }

func TestReviewWrites(t *testing.T) {
	r := newReviewRepo(t)
	var th gitbugio.Thread
	if code := call(t, "POST", r.url("/review", nil), map[string]any{"body": "why?", "path": "a.txt", "line": 3, "side": "new", "commit": r.c1[:7]}, &th); code != http.StatusCreated {
		t.Fatalf("line comment: %d", code)
	}
	line := last(th)
	if rv := line.Review; rv == nil || rv.Path != "a.txt" || rv.Line != 3 || rv.Commit != r.c1 || rv.PR != "" || rv.Text != "three" || rv.HeadLine != 3 || rv.Outdated {
		t.Fatalf("line comment = %+v", line.Review)
	}

	if code := call(t, "POST", r.url("/review", nil), map[string]any{"body": "because", "reply_to": line.ID}, &th); code != http.StatusCreated {
		t.Fatalf("reply: %d", code)
	}
	reply := last(th)
	if rv := reply.Review; rv == nil || rv.ReplyTo != line.ID || rv.Path != "a.txt" || rv.Line != 3 || rv.Side != "new" || rv.Commit != r.c1 {
		t.Fatalf("reply = %+v", reply.Review)
	}
	// a reply to a reply answers the thread's first comment, as on GitHub
	if code := call(t, "POST", r.url("/review", nil), map[string]any{"body": "ok", "reply_to": reply.ID}, &th); code != http.StatusCreated || last(th).Review.ReplyTo != line.ID {
		t.Fatalf("reply to a reply: %d %+v", code, last(th).Review)
	}

	if code := call(t, "POST", r.url("/review", nil), map[string]any{"state": "APPROVE", "commit": r.c1, "pr": "github.com/o/r#26"}, &th); code != http.StatusCreated {
		t.Fatalf("approve: %d", code)
	}
	if rv := last(th).Review; rv == nil || rv.State != "APPROVE" || rv.PR != "github.com/o/r#26" || rv.Commit != r.c1 {
		t.Fatalf("approve = %+v", rv)
	}

	var plain gitbugio.Thread
	if code := call(t, "POST", r.url("/comments", nil), map[string]any{"body": "plain"}, &plain); code != http.StatusOK {
		t.Fatal(code)
	}
	for name, req := range map[string]map[string]any{
		"reply to a plain comment":  {"body": "x", "reply_to": last(plain).ID},
		"reply to no comment":       {"body": "x", "reply_to": "nope"},
		"empty reply":               {"body": " ", "reply_to": line.ID},
		"unknown state":             {"state": "LGTM", "commit": r.c1},
		"review without commit":     {"state": "APPROVE"},
		"change request, no text":   {"state": "REQUEST_CHANGES", "commit": r.c1},
		"line comment without side": {"body": "x", "path": "a.txt", "line": 3, "commit": r.c1},
		"line 0":                    {"body": "x", "path": "a.txt", "line": 0, "side": "new", "commit": r.c1},
		"no such commit":            {"body": "x", "path": "a.txt", "line": 3, "side": "new", "commit": "nope"},
		"pr not a pull request":     {"body": "x", "path": "a.txt", "line": 3, "side": "new", "commit": r.c1, "pr": "o/r#1"},
		"nothing":                   {"body": "x"},
		"review with a path":        {"state": "COMMENT", "body": "x", "commit": r.c1, "path": "a.txt", "line": 3, "side": "new"},
		"review as a reply":         {"state": "COMMENT", "body": "x", "reply_to": line.ID},
	} {
		if code := call(t, "POST", r.url("/review", nil), req, nil); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}

	if code := call(t, "POST", r.url("/review/"+line.ID+"/resolve", nil), map[string]any{"resolved": true}, &th); code != http.StatusOK || !eventOf(t, th, line.ID).Review.Resolved {
		t.Fatalf("resolve: %d", code)
	}
	th = gitbugio.Thread{} // resolved: false is left out, keeping a decoded true
	if code := call(t, "POST", r.url("/review/"+line.ID+"/resolve", nil), map[string]any{"resolved": false}, &th); code != http.StatusOK || eventOf(t, th, line.ID).Review.Resolved {
		t.Fatalf("unresolve: %d", code)
	}
	if code := call(t, "POST", r.url("/review/"+reply.ID+"/resolve", nil), map[string]any{"resolved": true}, nil); code != http.StatusBadRequest {
		t.Errorf("resolve a reply: %d, want 400", code)
	}

	// a reply to a pull request's comment on a commit never fetched here
	gone := strings.Repeat("e", 40)
	pulled, err := r.s.b.CommentWith(r.id, "from the PR", map[string]string{"review:path": "a.txt", "review:line": "1", "review:side": "new", "review:commit": gone, "review:pr": "github.com/o/r#26"})
	if err != nil {
		t.Fatal(err)
	}
	if code := call(t, "POST", r.url("/review", nil), map[string]any{"body": "seen", "reply_to": pulled}, &th); code != http.StatusCreated || last(th).Review.Commit != gone {
		t.Fatalf("reply on a commit not here: %d %+v", code, last(th).Review)
	}

	r.s.syncing.Store(true)
	if code := call(t, "POST", r.url("/review", nil), map[string]any{"state": "APPROVE", "commit": r.c1}, nil); code != http.StatusConflict {
		t.Errorf("while syncing: %d, want 409", code)
	}
}

// A push to the branch moves the lines comments were written on, and
// outdates the ones it changes.
func TestAnnotateFollowsHead(t *testing.T) {
	r := newReviewRepo(t)
	var th gitbugio.Thread
	call(t, "POST", r.url("/review", nil), map[string]any{"body": "on 3", "path": "a.txt", "line": 3, "side": "new", "commit": r.c1}, &th)
	on3 := last(th).ID
	call(t, "POST", r.url("/review", nil), map[string]any{"body": "on 5", "path": "a.txt", "line": 5, "side": "new", "commit": r.c1}, &th)
	on5 := last(th).ID
	call(t, "POST", r.url("/review", nil), map[string]any{"body": "old 3", "path": "a.txt", "line": 3, "side": "old", "commit": r.c1}, &th)
	old3 := last(th).ID

	git(t, r.dir, "switch", "-q", "--detach", r.c1)
	if err := os.WriteFile(filepath.Join(r.dir, "a.txt"), []byte("zero\n1\n2\nTHREE\n4\n5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, r.dir, "commit", "-q", "-am", "c3")
	git(t, r.dir, "update-ref", "refs/remotes/origin/fix/2518-x", "HEAD")

	if code := call(t, "GET", r.url("/thread", nil), nil, &th); code != http.StatusOK {
		t.Fatal(code)
	}
	if rv := eventOf(t, th, on3).Review; !rv.Outdated || rv.Text != "three" {
		t.Errorf("on 3 = %+v, want outdated", rv)
	}
	if rv := eventOf(t, th, on5).Review; rv.Outdated || rv.HeadLine != 6 || rv.Text != "5" {
		t.Errorf("on 5 = %+v, want line 6 at head", rv)
	}
	if rv := eventOf(t, th, old3).Review; rv.Outdated || rv.Text != "3" {
		t.Errorf("old 3 = %+v, want the base's line", rv)
	}
}

// Without a repository the thread still answers, unannotated.
func TestAnnotateSkipsInMemory(t *testing.T) {
	ts, s := newIssueServer(t)
	id := gitbugiotest.NewIssue(t, s.b, "review")
	cid, err := s.b.CommentWith(id, "x", map[string]string{"review:path": "a.txt", "review:line": "1", "review:side": "new", "review:commit": "abc"})
	if err != nil {
		t.Fatal(err)
	}
	var th gitbugio.Thread
	if code := call(t, "GET", ts.URL+"/api/issues/"+id[:8]+"/thread", nil, &th); code != http.StatusOK {
		t.Fatal(code)
	}
	if rv := eventOf(t, th, cid).Review; rv == nil || rv.Outdated || rv.Text != "" || rv.HeadLine != 0 {
		t.Fatalf("review = %+v", rv)
	}
}

// Each comment is read on the branch it was written on, which it names;
// its quote stays once the branch is gone; after the merge, an old side
// comment has no base left to quote.
func TestAnnotateOwnBranch(t *testing.T) {
	r := newReviewRepo(t)
	git(t, r.dir, "switch", "-q", "-c", "2518-other", r.base)
	if err := os.WriteFile(filepath.Join(r.dir, "a.txt"), []byte("1\n2\n3\n4\nfive\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, r.dir, "commit", "-q", "-am", "o1")
	o1 := git(t, r.dir, "rev-parse", "HEAD")
	git(t, r.dir, "switch", "-q", "dev")

	var th gitbugio.Thread
	call(t, "POST", r.url("/review", nil), map[string]any{"body": "on five", "path": "a.txt", "line": 5, "side": "new", "commit": o1}, &th)
	other := last(th).ID
	call(t, "POST", r.url("/review", nil), map[string]any{"body": "on three", "path": "a.txt", "line": 3, "side": "new", "commit": r.c1}, &th)
	on3 := last(th).ID
	call(t, "POST", r.url("/review", nil), map[string]any{"body": "old 3", "path": "a.txt", "line": 3, "side": "old", "commit": r.c1}, &th)
	old3 := last(th).ID
	call(t, "POST", r.url("/review", nil), map[string]any{"state": "APPROVE", "commit": o1}, &th)
	approve := last(th).ID

	if code := call(t, "GET", r.url("/thread", nil), nil, &th); code != http.StatusOK {
		t.Fatal(code)
	}
	if rv := eventOf(t, th, other).Review; rv.Branch != "2518-other" || rv.Outdated || rv.HeadLine != 5 || rv.Text != "five" {
		t.Errorf("on 2518-other = %+v, want its own branch's line", rv)
	}
	if rv := eventOf(t, th, on3).Review; rv.Branch != "fix/2518-x" {
		t.Errorf("on fix/2518-x = %+v", rv)
	}
	if rv := eventOf(t, th, approve).Review; rv.Branch != "2518-other" {
		t.Errorf("the approval = %+v, want its branch", rv)
	}

	// merged (origin's dev has c1), then the branch deleted
	git(t, r.dir, "update-ref", "refs/remotes/origin/dev", r.c1)
	git(t, r.dir, "update-ref", "-d", "refs/remotes/origin/fix/2518-x")
	git(t, r.dir, "branch", "-q", "-D", "fix/2518-x")
	th = gitbugio.Thread{} // fields left out would keep the decoded ones
	if code := call(t, "GET", r.url("/thread", nil), nil, &th); code != http.StatusOK {
		t.Fatal(code)
	}
	if rv := eventOf(t, th, on3).Review; rv.Text != "three" || rv.Outdated {
		t.Errorf("on 3, branch gone = %+v, want its quote", rv)
	}
	if rv := eventOf(t, th, old3).Review; rv.Text != "" {
		t.Errorf("old 3, merged = %+v, want no quote (the new side's line isn't the old one)", rv)
	}
}
