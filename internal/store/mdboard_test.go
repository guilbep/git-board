package store_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/mdstore"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// A board kept in Markdown files: its packs in issues/.board, a file per
// issue rendered after each write, a hand edit read in with the next write
// or an import; a git board copies in with its ids.
func TestMarkdownBoard(t *testing.T) {
	t.Parallel()
	dir := storetest.Repo(t, false)
	storetest.Init(t, dir) // dev, in git first
	g := storetest.Open(t, dir)
	first, err := g.NewIssue("Kept in git first", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res := g.TestDatastore(t.Context(), "markdown:issues"); !res.OK {
		t.Fatalf("test: %+v", res)
	}
	if n, err := g.CopyGitTo(t.Context(), "markdown:issues"); err != nil || n != 2 {
		t.Fatalf("copy: %d, %v; want the issue and dev", n, err)
	}
	folder := filepath.Join(dir, "issues")
	if files, _ := filepath.Glob(filepath.Join(folder, "*.md")); len(files) != 1 {
		t.Fatalf("rendered %v, want one file", files)
	}

	storetest.Git(t, dir, "config", store.DatastoreKey, "markdown:issues")
	b := storetest.Open(t, dir)
	if c := b.Capabilities(); c.Datastore != "Markdown files" || c.Replica || !c.Counter {
		t.Fatalf("capabilities %+v", c)
	}
	if _, err := b.GetIssue(first); err != nil {
		t.Fatalf("the copied issue: %v", err)
	}
	id, err := b.NewIssue("Kept in files", "the body", []string{"prio:p1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(folder, mdstore.Board, "issue", id+".jsonl")); err != nil {
		t.Fatalf("the packs: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(folder, "*kept-in-files.md"))
	if len(files) != 1 {
		t.Fatalf("no file rendered for the new issue")
	}
	// a hand edit, read in with the next write
	edit(t, files[0], "the body", "the body, by hand")
	if _, err := b.AddComment(first, "a write elsewhere"); err != nil {
		t.Fatal(err)
	}
	if th, err := b.Thread(id); err != nil || th.Events[0].Body != "the body, by hand" {
		t.Fatalf("after the next write: %+v, %v", th.Events, err)
	}
	if refs := storetest.Git(t, dir, "for-each-ref", "refs/board/issue/"); !strings.Contains(refs, first) || strings.Contains(refs, id) {
		t.Fatalf("git refs %q: the first issue's, untouched, and nothing of the new one", refs)
	}
	if err := b.Pull("origin"); err == nil || !strings.Contains(err.Error(), "Markdown files") {
		t.Fatalf("pull: %v", err)
	}
	if _, err := b.ResetHard(""); err == nil {
		t.Fatal("start over: no error")
	} else if !isMissing(err) {
		t.Fatalf("start over: %v", err)
	}
}

func isMissing(err error) bool {
	for e := err; e != nil; {
		if e == datastore.ErrMissing {
			return true
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}

// Two writes to one issue in one second both stay: the files are read in
// before a write, while they still say what the board held. A hand edit is
// read in by the next write to its own issue.
func TestMarkdownBoardWritesInARow(t *testing.T) {
	t.Parallel()
	dir := storetest.Repo(t, false)
	storetest.Git(t, dir, "config", store.DatastoreKey, "markdown:issues")
	storetest.Init(t, dir)
	b := storetest.Open(t, dir)
	id, err := b.NewIssue("Title", "body", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.ApplyLabelChange(id, []string{"prio:p1"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := b.SetTitle(id, "Title 2"); err != nil {
		t.Fatal(err)
	}
	is, err := b.GetIssue(id)
	if err != nil || is.Title != "Title 2" || len(is.Labels) != 1 {
		t.Fatalf("issue %+v, %v; want both writes", is, err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "issues", "*.md"))
	if len(files) != 1 {
		t.Fatalf("files %v", files)
	}
	edit(t, files[0], "body", "body, by hand")
	if _, err := b.AddComment(id, "a write to it"); err != nil {
		t.Fatal(err)
	}
	if evs := eventsOf(t, b, id); evs[0].Body != "body, by hand" {
		t.Fatalf("description %q, want the hand edit", evs[0].Body)
	}
	if data, _ := os.ReadFile(files[0]); !strings.Contains(string(data), "a write to it") || !strings.Contains(string(data), "body, by hand") {
		t.Fatalf("the file after:\n%s", data)
	}
}

// The forge sync writes the board past the backend's writes: the files
// follow it too, so a hand edit made after the sync is read in, not lost.
func TestMarkdownBoardFollowsAForgeSync(t *testing.T) {
	t.Parallel()
	dir := storetest.Repo(t, false)
	storetest.Git(t, dir, "config", store.DatastoreKey, "markdown:issues")
	storetest.Init(t, dir)
	b := storetest.Open(t, dir)
	other, err := b.NewIssue("Other", "x", nil)
	if err != nil {
		t.Fatal(err)
	}
	f := fake.New("github.com/o/r")
	f.Add(forge.Issue{Author: forge.Account{ID: "7", Login: "ann"}, Title: "Pulled", Body: "pulled body", Status: "open", Created: 1000})
	if _, err := b.SyncForges(t.Context(), run(f)); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "issues", "*pulled.md"))
	if len(files) != 1 {
		t.Fatalf("no file rendered for the pulled issue")
	}
	edit(t, files[0], "pulled body", "pulled body, by hand")
	if _, err := b.AddComment(other, "a write elsewhere"); err != nil {
		t.Fatal(err)
	}
	list, err := b.ListIssues()
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range list {
		if is.Title != "Pulled" {
			continue
		}
		th, err := b.Thread(is.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got := th.Events[0].Body; got != "pulled body, by hand" {
			t.Errorf("body %q, want the hand edit", got)
		}
	}
}

// Two processes on one Markdown board (a server and a command) read the
// hand edits in, write and render one at a time: every write lands, every
// file stays whole, and no hand edit is read in twice (issue #85).
func TestMarkdownBoardTwoProcesses(t *testing.T) {
	t.Parallel()
	dir := storetest.Repo(t, false)
	storetest.Git(t, dir, "config", store.DatastoreKey, "markdown:issues")
	storetest.Init(t, dir)
	one, two := storetest.Open(t, dir), storetest.Open(t, dir) // a backend each, as two processes have
	folder := filepath.Join(dir, "issues")
	var ids [3]string
	for i, title := range []string{"One's", "Two's", "Edited by hand"} {
		id, err := one.NewIssue(title, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = id
	}
	const rounds = 15
	for r := range rounds {
		// by hand, while neither writes: a new comment, and a new issue
		files, _ := filepath.Glob(filepath.Join(folder, "*edited-by-hand.md"))
		if len(files) != 1 {
			t.Fatalf("round %d: files %v", r, files)
		}
		data, err := os.ReadFile(files[0])
		if err != nil {
			t.Fatal(err)
		}
		hand := fmt.Sprintf("\n### dev · 2026-11-04T10:00:00Z\n\nby hand %d\n", r)
		if err := os.WriteFile(files[0], append(data, hand...), 0o644); err != nil {
			t.Fatal(err)
		}
		file := fmt.Sprintf("---\ntitle: Hand %d\nstate: open\npeople:\n  author: dev\n---\n\nnew by hand\n", r)
		if err := os.WriteFile(filepath.Join(folder, fmt.Sprintf("hand-%d.md", r)), []byte(file), 0o644); err != nil {
			t.Fatal(err)
		}
		// then both write at once, each reading the edits in first
		var wg sync.WaitGroup
		for i, b := range []*store.Backend{one, two} {
			wg.Go(func() {
				if _, err := b.AddComment(ids[i], fmt.Sprintf("write %d", r)); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
	}

	for i := range 2 {
		if got := bodies(eventsOf(t, one, ids[i]), "comment"); len(got) != rounds {
			t.Errorf("issue %d has %d comments, want each of the %d writes: %q", i, len(got), rounds, got)
		}
	}
	got := bodies(eventsOf(t, one, ids[2]), "comment")
	for r := range rounds {
		if n := count(got, fmt.Sprintf("by hand %d", r)); n != 1 {
			t.Errorf("the comment by hand %d read in %d times, want once", r, n)
		}
	}
	list, err := one.ListIssues()
	if err != nil {
		t.Fatal(err)
	}
	titles := map[string]int{}
	for _, is := range list {
		titles[is.Title]++
	}
	for r := range rounds {
		if n := titles[fmt.Sprintf("Hand %d", r)]; n != 1 {
			t.Errorf("issue Hand %d made %d times, want once", r, n)
		}
	}

	// every pack line is whole, and every file reads as the board has it
	packs, _ := filepath.Glob(filepath.Join(folder, mdstore.Board, "*", "*.jsonl"))
	for _, p := range packs {
		data, _ := os.ReadFile(p)
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if !json.Valid([]byte(line)) {
				t.Errorf("%s: a torn line %q", p, line)
			}
		}
	}
	rep, err := one.ImportMarkdown(folder)
	if err != nil || len(rep.Skipped)+len(rep.Created)+len(rep.Updated)+len(rep.Conflicts) > 0 {
		t.Errorf("the files read again: %+v, %v; want them all as the board has them", rep, err)
	}
}

func count(list []string, s string) int {
	n := 0
	for _, x := range list {
		if x == s {
			n++
		}
	}
	return n
}

// Issue #84: a Markdown board numbers its issues on the default branch,
// through .board/numbers; on another branch the counter is read, and
// numbering says why it waits, without failing the sync.
func TestMarkdownBoardNumbers(t *testing.T) {
	t.Parallel()
	dir := storetest.Repo(t, false)
	storetest.Git(t, dir, "switch", "-q", "-c", "main") // an unborn main, whatever git's default
	storetest.Git(t, dir, "config", store.DatastoreKey, "markdown:issues")
	storetest.Init(t, dir)
	b := storetest.Open(t, dir)
	if c := b.Capabilities(); !c.Counter {
		t.Fatalf("capabilities %+v: a Markdown board counts", c)
	}
	if err := b.InitNumbers(store.NumbersRemote, "GB", 1); err != nil {
		t.Fatalf("start the numbers on main: %v", err)
	}
	first, err := b.NewIssue("Made on main", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := b.NumberIssues(store.NumbersRemote); err != nil || n != 1 {
		t.Fatalf("numbering on main: %d, %v", n, err)
	}
	c, ok, err := b.Counter()
	if err != nil || !ok || c.ByID[first] != 1 {
		t.Fatalf("counter %+v, %v, %v; want %s as GB-1", c, ok, err, first)
	}
	data, err := os.ReadFile(filepath.Join(dir, "issues", mdstore.Board, "numbers"))
	if err != nil || string(data) != "prefix GB\nstart 1\n1 "+first+"\n" {
		t.Fatalf(".board/numbers = %q, %v: refs/board/seq's format", data, err)
	}
	if files, _ := filepath.Glob(filepath.Join(dir, "issues", "*made-on-main.md")); len(files) != 1 || filepath.Base(files[0]) != "1-made-on-main.md" {
		t.Errorf("the issue's file, named by its number: %v", files)
	}
	if w := b.NumbersWait(); w != "" {
		t.Errorf("on main, NumbersWait = %q", w)
	}

	storetest.Git(t, dir, "add", "-A")
	storetest.Git(t, dir, "commit", "-q", "-m", "the board")
	storetest.Git(t, dir, "switch", "-q", "-c", "feat/csv")
	second, err := b.NewIssue("Made on a branch", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.NumberIssues(store.NumbersRemote)
	if !errors.Is(err, datastore.ErrMissing) || !strings.Contains(err.Error(), "feat/csv") {
		t.Fatalf("numbering on feat/csv: %v; want why it waits", err)
	}
	if w := b.NumbersWait(); !strings.Contains(w, "main") || !strings.Contains(w, "feat/csv") {
		t.Errorf("NumbersWait = %q", w)
	}
	if c, _, _ := b.Counter(); c.ByID[first] != 1 || c.ByID[second] != 0 {
		t.Errorf("on the branch, the counter reads %+v", c)
	}
	var heard []string
	w := store.Whole{Remote: "origin", Step: func(name, state, note string) {
		if name == store.StepPush {
			heard = append(heard, state+": "+note)
		}
	}}
	if err := b.SyncWhole(t.Context(), w); err != nil || len(heard) != 2 || !strings.HasPrefix(heard[1], "skipped: ") || !strings.Contains(heard[1], "feat/csv") {
		t.Fatalf("sync on the branch: %v, the push step %q", err, heard)
	}

	storetest.Git(t, dir, "add", "-A")
	storetest.Git(t, dir, "commit", "-q", "-m", "on the branch")
	storetest.Git(t, dir, "switch", "-q", "main")
	storetest.Git(t, dir, "merge", "-q", "--no-edit", "feat/csv")
	if n, err := b.NumberIssues(store.NumbersRemote); err != nil || n != 1 {
		t.Fatalf("numbering on main after the merge: %d, %v", n, err)
	}
	if c, _, _ := b.Counter(); c.ByID[second] != 2 {
		t.Errorf("after the merge, %+v; want %s as GB-2", c, second)
	}
}

// A hand edit of a file's labels is read in by the next write: under the
// lock the write holds, not waiting on it.
func TestMarkdownBoardLabelsByHand(t *testing.T) {
	t.Parallel()
	dir := storetest.Repo(t, false)
	storetest.Git(t, dir, "config", store.DatastoreKey, "markdown:issues")
	storetest.Init(t, dir)
	b := storetest.Open(t, dir)
	id, err := b.NewIssue("Title", "body", []string{"prio:p1"})
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "issues", "*.md"))
	if len(files) != 1 {
		t.Fatalf("files %v", files)
	}
	edit(t, files[0], "    - p1", "    - p2")
	done := make(chan error, 1)
	go func() { _, err := b.AddComment(id, "a write to it"); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the write waits on its own lock")
	}
	is, err := b.GetIssue(id)
	if err != nil || !slices.Equal(is.Labels, []string{"prio:p2"}) {
		t.Fatalf("labels %v, %v; want the hand edit's", is.Labels, err)
	}
}
