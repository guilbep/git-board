package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/domain/issues/issuesync"
	"github.com/guilbep/git-board/domain/releases"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
	bsync "github.com/guilbep/git-board/sync"
)

// A forge run on an open board: what it pushes and pulls is the board's,
// and ls shows where each issue is.
func TestSyncForges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	me, err := issuesync.CreatePerson(t.Context(), b.Kernel, issuesync.NewPerson("", "me", ""))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issuesync.CreateIssue(t.Context(), b.Kernel, issuesync.NewCreate(me.ID, "Here", "made here")); err != nil {
		t.Fatal(err)
	}
	f := fake.New("example.com/p")
	f.Add(forge.Issue{Author: forge.Account{ID: "7", Login: "ann"}, Title: "There", Body: "made there"})
	var events []string
	run := ForgeRun{Forges: []NamedForge{{Name: "p", Forge: f}}, See: func(e SyncEvent) { events = append(events, e.Forge+" "+e.What) }}

	// a first sync creates nothing without consent
	if _, err := b.SyncForges(t.Context(), WaySync, run); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(events, "p held") || !slices.Contains(events, "p created") || len(f.All()) != 1 {
		t.Fatalf("events %q, %d forge issues", events, len(f.All()))
	}
	run.AllowCreate = true
	if _, err := b.SyncForges(t.Context(), WayPush, run); err != nil {
		t.Fatal(err)
	}
	if len(f.All()) != 2 || !slices.Contains(events, "p posted") {
		t.Fatalf("events %q, %d forge issues", events, len(f.All()))
	}
	list, err := b.Issues.List(t.Context(), issues.ListReq{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || len(list[0].Forges) != 1 || len(list[1].Forges) != 1 {
		t.Fatalf("ls = %+v", list)
	}
}

// With no [forges] table, the forges are origin's: a local path is none,
// and a URL naming none is told, with why. None is read on the network.
func TestForgesFromRemotes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "/elsewhere/repo.git"},
		{"remote", "set-url", "--add", "--push", "origin", "https://user:secret@example.com/x/y.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var said []string
	fs, err := b.Forges(t.Context(), func(s string) { said = append(said, s) })
	if err != nil || len(fs) != 0 {
		t.Fatalf("forges %+v, %v", fs, err)
	}
	if want := []string{"skipped https://example.com/x/y.git: not github.com, nor a GitLab host"}; !slices.Equal(said, want) {
		t.Errorf("said %q, want %q", said, want)
	}
}

// A request gone while the forges are read from the remotes keeps no
// empty list: the next request finds them.
func TestFoundForgesCancelledKeepsNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "git@github.com:o/r.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if _, err := b.Remotes(t.Context()); err != nil { // kept, as a request before left them
		t.Fatal(err)
	}
	gone, cancel := context.WithCancel(t.Context())
	cancel()
	_, _ = b.FoundForges(gone, func(string) {})
	fs, err := b.FoundForges(t.Context(), func(string) {})
	if err != nil || len(fs) != 1 {
		t.Fatalf("after a request gone mid-read: forges %+v, %v, want o/r", fs, err)
	}
}

// A forge origin fetches from syncs; one that only a push URL names (a
// mirror the code is pushed to) is found with its sync off: the file alone
// turns it on (ADR 0008). FoundForges asks no forge.
func TestFoundForgesPushOnlyOff(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "git@github.com:acme/app.git"},
		{"remote", "set-url", "--add", "--push", "origin", "git@github.com:acme/app.git"},
		{"remote", "set-url", "--add", "--push", "origin", "git@gitlab-ssh.example.org:acme/app.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	fs, err := b.FoundForges(t.Context(), func(string) {})
	if err != nil || len(fs) != 2 {
		t.Fatalf("found %+v, %v", fs, err)
	}
	on := map[string]bool{}
	for _, f := range fs {
		on[f.Kind] = f.Sync
	}
	if !on["github"] || on["gitlab"] {
		t.Errorf("sync on: %v, want github on, gitlab (push only) off", on)
	}
}

// On a Markdown folder, what a forge sync pulls is rendered into the
// folder's files.
func TestSyncForgesRendersMarkdown(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "board.datastore", "markdown:issues"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if b.Files == nil {
		t.Fatal("no Markdown folder")
	}
	f := fake.New("example.com/p")
	f.Add(forge.Issue{Author: forge.Account{ID: "7", Login: "ann"}, Title: "Pulled into a file", Body: "made there"})
	if _, err := b.SyncForges(t.Context(), WayPull, ForgeRun{Forges: []NamedForge{{Name: "p", Forge: f}}}); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "issues", "*.md"))
	var found bool
	for _, p := range files {
		data, _ := os.ReadFile(p)
		found = found || strings.Contains(string(data), "Pulled into a file")
	}
	if !found {
		t.Errorf("no file holds the pulled issue: %v", files)
	}
}

// A check runs on a copy: it says what it would do, and writes nothing,
// here or on the forge.
func TestSyncForgesCheck(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	me, err := issuesync.CreatePerson(t.Context(), b.Kernel, issuesync.NewPerson("", "me", ""))
	if err != nil {
		t.Fatal(err)
	}
	here, err := issuesync.CreateIssue(t.Context(), b.Kernel, issuesync.NewCreate(me.ID, "Here", "made here"))
	if err != nil {
		t.Fatal(err)
	}
	f := fake.New("example.com/p")
	f.Add(forge.Issue{Author: forge.Account{ID: "7", Login: "ann"}, Title: "There", Body: "made there"})
	before, err := gitRun(t.Context(), dir, "for-each-ref", "refs/board/")
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	run := ForgeRun{Forges: []NamedForge{{Name: "p", Forge: f}}, DryRun: true, See: func(e SyncEvent) {
		if l := e.Text(true, e.Issue[:min(7, len(e.Issue))]); l != "" {
			lines = append(lines, l)
		}
	}}
	if _, err := b.SyncForges(t.Context(), WaySync, run); err != nil {
		t.Fatal(err)
	}
	after, _ := gitRun(t.Context(), dir, "for-each-ref", "refs/board/")
	if before != after || len(f.All()) != 1 || len(f.Writes()) != 0 {
		t.Fatalf("a check wrote: refs moved %t, %d forge issues, writes %v", before != after, len(f.All()), f.Writes())
	}
	// without consent a check holds the creates as a sync would: it says
	// what Sync does, never what a consent would do
	if w := "1 issues to create need consent: the first sync with this forge"; !slices.Contains(lines, w) {
		t.Errorf("lines %q lack %q", lines, w)
	}
	if slices.Contains(lines, "would create "+here.ID[:7]) {
		t.Errorf("a check without consent creates: %q", lines)
	}
	if !slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, "would pull a new issue, ") }) {
		t.Errorf("lines %q lack the pull", lines)
	}
	// with consent asked, it shows the create
	lines = nil
	run.AllowCreate = true
	if _, err := b.SyncForges(t.Context(), WaySync, run); err != nil {
		t.Fatal(err)
	}
	if w := "would create " + here.ID[:7]; !slices.Contains(lines, w) {
		t.Errorf("lines %q lack %q", lines, w)
	}
	if len(f.Writes()) != 0 {
		t.Fatalf("a check wrote %v", f.Writes())
	}
}

// completes is a fake whose issue reads (Complete) are told, by title; one
// titled failOn fails, as a dropped connection.
type completes struct {
	*fake.Forge
	mu     sync.Mutex
	read   []string
	failOn string
}

func (c *completes) Complete(ctx context.Context, is forge.Issue) (forge.Issue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if is.Title == c.failOn {
		return is, errors.New("the connection dropped")
	}
	c.read = append(c.read, is.Title)
	return c.Forge.Complete(ctx, is)
}

// then is the reads since the last, which it forgets, with the next to
// fail.
func (c *completes) then(failOn string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	got := c.read
	c.read, c.failOn = nil, failOn
	return got
}

// A sync right after a check doesn't read again what the check found
// the board holding: it reads what changed alone, and pulls it. A check
// that failed hands nothing on.
func TestASyncAfterACheckReadsWhatChanged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	f := &completes{Forge: fake.New("example.com/p")}
	var added []forge.Issue
	for i := range 5 {
		added = append(added, f.Add(forge.Issue{Author: forge.Account{ID: "7", Login: "ann"}, Title: fmt.Sprint("i", i), Status: "open", Created: int64(1000 + i)}))
	}
	run := ForgeRun{Forges: []NamedForge{{Name: "p", Forge: f}}}
	// pulled by a pull alone, which records no run, its journal emptied:
	// each run after lists all five
	if _, err := b.SyncForges(t.Context(), WayPull, run); err != nil {
		t.Fatal(err)
	}
	if err := bsync.Forget(b.Kernel.Store()); err != nil {
		t.Fatal(err)
	}
	if err := f.SetTitle(t.Context(), added[2].Ref(), "i2, renamed"); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(b.Kernel.Store().Folder(), "sync.lock")
	journal := func() string { data, _ := os.ReadFile(lock); return string(data) }

	f.then("i3")
	before := journal()
	check := run
	check.DryRun = true
	if _, err := b.SyncForges(t.Context(), WaySync, check); err == nil {
		t.Fatal("the failing check went through")
	}
	if journal() != before {
		t.Fatalf("a failed check handed on:\n%s", journal())
	}
	f.then("")
	if _, err := b.SyncForges(t.Context(), WaySync, check); err != nil {
		t.Fatal(err)
	}
	// a set: a read the failed check started ahead may land late
	if got := slices.Compact(slices.Sorted(slices.Values(f.then("")))); !slices.Equal(got, []string{"i0", "i1", "i2, renamed", "i3", "i4"}) {
		t.Fatalf("the check read %q, want the five", got)
	}
	if _, err := b.SyncForges(t.Context(), WaySync, run); err != nil {
		t.Fatal(err)
	}
	if got := f.then(""); !slices.Equal(got, []string{"i2, renamed"}) {
		t.Fatalf("the sync after the check read %q, want what changed alone", got)
	}
	list, err := b.Issues.List(t.Context(), issues.ListReq{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(list, func(is issues.Issue) bool { return is.Title == "i2, renamed" }) {
		t.Fatal("the change the check saw wasn't pulled")
	}
}

// A reply reaches a forge with threads as a reply, through the wrapper
// that hears the run's writes (ADR 0021: v1 posted it as a top-level
// comment); a forge without threads, a counted one, gets none.
func TestSyncForgesReplies(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	me, err := issuesync.CreatePerson(t.Context(), b.Kernel, issuesync.NewPerson("", "me", ""))
	if err != nil {
		t.Fatal(err)
	}
	is, err := issuesync.CreateIssue(t.Context(), b.Kernel, issuesync.NewCreate(me.ID, "Here", "made here"), issuesync.NewComment(me.ID, "root"))
	if err != nil {
		t.Fatal(err)
	}
	root := is.Comments[0].ID
	if _, err := issuesync.DoIssue(t.Context(), b.Kernel, is.ID, issuesync.NewCommentMeta(me.ID, "answer", map[string]string{"thread:reply-to": root})); err != nil {
		t.Fatal(err)
	}
	f := fake.New("example.com/p")
	var events []string
	run := ForgeRun{Forges: []NamedForge{{Name: "p", Forge: f}}, AllowCreate: true, See: func(e SyncEvent) { events = append(events, e.What) }}
	if _, err := b.SyncForges(t.Context(), WayPush, run); err != nil {
		t.Fatal(err)
	}
	var ops []string
	for _, w := range f.Writes() {
		if w.Op == "reply" || w.Op == "create-comment" {
			ops = append(ops, w.Op)
		}
	}
	if strings.Join(ops, ",") != "create-comment,reply" {
		t.Errorf("writes %v, want a comment then a reply", ops)
	}
	if n := strings.Count(strings.Join(events, ","), "comment"); n != 2 {
		t.Errorf("events %q: want both comments heard", events)
	}
	if _, ok := (&counted{f: flat{f}}).wrap().(forge.Replier); ok {
		t.Error("a forge without threads is offered Reply")
	}
	if _, ok := (&counted{f: f}).wrap().(forge.Replier); !ok {
		t.Error("a forge with threads is not offered Reply")
	}
}

// flat is a forge without threads: Reply hidden.
type flat struct{ forge.Forge }

// A board that publishes binaries publishes its records on a forge push,
// as releases of their tags; a dry run hears it and writes none.
func TestSyncForgesPublishesReleases(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=a", "-c", "user.email=a@x", "commit", "-q", "--allow-empty", "-m", "start"}, {"tag", "v1.0.0"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte("[release]\nbinaries = [\"./cmd/x\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	me, err := b.People.New(t.Context(), "me", "")
	if err != nil {
		t.Fatal(err)
	}
	b.Issues.Me = me.ID
	commit, _ := gitRun(t.Context(), dir, "rev-parse", "HEAD")
	book := b.Book()
	r, err := book.Create(t.Context(), releases.Record{Commit: strings.TrimSpace(commit)})
	if err != nil {
		t.Fatal(err)
	}
	if err := book.SetNotes(t.Context(), r.ID, "The first."); err != nil {
		t.Fatal(err)
	}
	f := fake.New("example.com/p")
	var events []string
	run := ForgeRun{Forges: []NamedForge{{Name: "p", Forge: f}}, DryRun: true, See: func(e SyncEvent) {
		if e.What == "release" {
			events = append(events, e.Text(true, ""))
		}
	}}
	if _, err := b.SyncForges(t.Context(), WayPush, run); err != nil {
		t.Fatal(err)
	}
	if rs, _ := f.Releases(t.Context()); len(rs) != 0 || strings.Join(events, ",") != "would publish the release v1.0.0" {
		t.Fatalf("dry run: releases %+v, events %q", rs, events)
	}
	run.DryRun = false
	if _, err := b.SyncForges(t.Context(), WayPush, run); err != nil {
		t.Fatal(err)
	}
	if rs, _ := f.Releases(t.Context()); len(rs) != 1 || rs[0].Tag != "v1.0.0" || rs[0].Body != "The first." {
		t.Errorf("releases %+v", rs)
	}
}

// A server asks the forges on every GET /api/sync and /api/state: what
// git says is kept while its config files are unchanged. Serial: it
// empties PATH, so that git can't answer what is kept.
func TestFoundForgesKept(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("remote", "add", "origin", "https://github.com/o/r.git")
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	project := func() string {
		t.Helper()
		fs, err := b.FoundForges(t.Context(), func(string) {})
		if err != nil || len(fs) != 1 {
			t.Fatalf("forges %+v, %v", fs, err)
		}
		return fs[0].Project
	}
	if p := project(); p != "o/r" {
		t.Fatalf("project %q, want o/r", p)
	}
	path, _ := exec.LookPath("git")
	t.Setenv("PATH", "")
	if p := project(); p != "o/r" {
		t.Errorf("kept: project %q, want o/r", p)
	}
	if rs, err := b.Remotes(t.Context()); err != nil || !slices.Equal(rs, []string{"origin"}) {
		t.Errorf("kept: remotes %v, %v", rs, err)
	}
	t.Setenv("PATH", filepath.Dir(path))
	git("remote", "set-url", "origin", "https://github.com/o/moved.git")
	if p := project(); p != "o/moved" {
		t.Errorf("after the remote moved: project %q, want o/moved", p)
	}
}
