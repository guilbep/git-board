package issues_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/kernel"
	"github.com/guilbep/git-board/next/store/mem"
	"github.com/guilbep/git-board/next/view"
)

// undoBoard is a board in memory with Ada and Bob, each a Service.
func undoBoard(t *testing.T) (ada, bob *issues.Service) {
	t.Helper()
	ctx := context.Background()
	v, err := view.Open(ctx, filepath.Join(t.TempDir(), "view.db"))
	if err != nil {
		t.Fatal(err)
	}
	kb, err := kernel.Open(ctx, mem.New(t.TempDir()), v, issues.Kind, people.Kind)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { kb.Close() })
	person := func(name string) kernel.ID {
		data, _ := json.Marshal(map[string]string{"name": name, "email": strings.ToLower(name) + "@example.com"})
		p, err := people.Kind.Create(ctx, kb, "", kernel.Op{Kind: "create", Time: 1, Data: data})
		if err != nil {
			t.Fatal(err)
		}
		return p.ID
	}
	return &issues.Service{Board: kb, Schema: &issues.Schema{}, Me: person("Ada")},
		&issues.Service{Board: kb, Schema: &issues.Schema{}, Me: person("Bob")}
}

// Each inverse undo writes, and undoing twice redoing it: labels, status,
// title, the description, a comment's edit, the title and description
// of one edit together.
func TestUndoEachInverse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _ := undoBoard(t)
	is, err := s.New(ctx, issues.NewReq{Title: "First", Body: "body one", Labels: []string{"bug"}})
	if err != nil {
		t.Fatal(err)
	}
	id := is.ID
	show := func() issues.Thread {
		t.Helper()
		th, err := s.Show(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return th
	}
	lastComment := func() string {
		t.Helper()
		e, err := issues.Kind.Get(ctx, s.Board, kernel.ID(id))
		if err != nil || len(e.State.Comments) == 0 {
			t.Fatalf("comments: %v", err)
		}
		return e.State.Comments[len(e.State.Comments)-1].Body
	}
	undo := func(want ...string) {
		t.Helper()
		u, err := s.Undo(ctx, id[:8], false)
		if err != nil {
			t.Fatalf("undo: %v", err)
		}
		if !slices.Equal(u.What, want) || u.Issue != id || len(u.Ops) == 0 {
			t.Fatalf("undo: %+v, want what %q", u, want)
		}
	}

	if _, err := s.ChangeLabels(ctx, id, []string{"ui", "bug"}, nil); err != nil { // bug: there already
		t.Fatal(err)
	}
	undo("labels -ui")
	if l := show().Labels; !slices.Equal(l, []string{"bug"}) {
		t.Fatalf("labels after undo %q", l)
	}
	undo("labels +ui")
	if l := show().Labels; !slices.Equal(l, []string{"bug", "ui"}) {
		t.Fatalf("labels after the second undo %q", l)
	}
	if _, err := s.ChangeLabels(ctx, id, nil, []string{"bug"}); err != nil {
		t.Fatal(err)
	}
	undo("labels +bug")
	if l := show().Labels; !slices.Equal(l, []string{"bug", "ui"}) {
		t.Fatalf("labels after undoing a removal %q", l)
	}

	if err := s.SetStatus(ctx, id, "closed"); err != nil {
		t.Fatal(err)
	}
	undo("status back to open")
	undo("status back to closed")
	if st := show().Status; st != "closed" {
		t.Fatalf("status after the redo %s", st)
	}

	th := show()
	title, body := "Second", "body two"
	if _, err := s.Edit(ctx, id, &title, nil, th); err != nil {
		t.Fatal(err)
	}
	undo(`title back to "First"`)
	if th := show(); th.Title != "First" {
		t.Fatalf("title after undo %q", th.Title)
	}
	undo(`title back to "Second"`)

	th = show()
	title = "Third"
	if _, err := s.Edit(ctx, id, &title, &body, th); err != nil { // edit -e: one change
		t.Fatal(err)
	}
	undo("the description back to its previous version", `title back to "Second"`)
	if e, _ := issues.Kind.Get(ctx, s.Board, kernel.ID(id)); e.State.Title != "Second" || e.State.Body != "body one" {
		t.Fatalf("after undoing edit -e: %q, %q", e.State.Title, e.State.Body)
	}

	c, _, err := s.Comment(ctx, id, "a comment")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EditText(ctx, id, c, "a comment, edited", ""); err != nil {
		t.Fatal(err)
	}
	ref := strings.Replace(string(c), string(c)[:strings.IndexByte(string(c), ':')], string(c)[:7], 1)
	undo("comment " + ref + " back to its previous version")
	if b := lastComment(); b != "a comment" {
		t.Fatalf("comment after undo %q", b)
	}
	undo("comment " + ref + " back to its previous version")
	if b := lastComment(); b != "a comment, edited" {
		t.Fatalf("comment after the redo %q", b)
	}
}

// A new comment's undo deletes it, and that delete is final (ADR 0019):
// the second undo refuses, names the comment and says how to post it
// again; the comment stays out of the issue.
func TestUndoCommentIsFinal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _ := undoBoard(t)
	is, err := s.New(ctx, issues.NewReq{Title: "One"})
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := s.Comment(ctx, is.ID, "hi")
	if err != nil {
		t.Fatal(err)
	}
	ref := string(c)[:7] + string(c)[strings.IndexByte(string(c), ':'):]
	u, err := s.Undo(ctx, is.ID, false)
	if err != nil || !slices.Equal(u.What, []string{"comment " + ref + " deleted"}) {
		t.Fatalf("undo: %+v, %v", u, err)
	}
	_, err = s.Undo(ctx, is.ID, false)
	if !errors.Is(err, issues.ErrCantUndo) || !strings.Contains(err.Error(), "deleted comment "+ref) ||
		!strings.Contains(err.Error(), "never undone") || !strings.Contains(err.Error(), "git board comment "+is.ID[:8]) {
		t.Fatalf("second undo: %v", err)
	}
	if e, _ := issues.Kind.Get(ctx, s.Board, kernel.ID(is.ID)); len(e.State.Comments) != 0 {
		t.Fatalf("comments %+v", e.State.Comments)
	}
	// a delete by hand: refused the same
	c2, _, err := s.Comment(ctx, is.ID, "again")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteComment(ctx, is.ID, c2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Undo(ctx, is.ID, false); !errors.Is(err, issues.ErrCantUndo) {
		t.Fatalf("undo of a delete: %v", err)
	}
}

// What undo refuses: no change of yours, the create, a deleted issue, a
// change of metadata, a change undone already; someone else's write after
// yours unless --force, a forge's pull too.
func TestUndoRefuses(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, bob := undoBoard(t)
	if _, err := s.Undo(ctx, "", false); !errors.Is(err, issues.ErrCantUndo) {
		t.Fatalf("nothing written yet: %v", err)
	}
	is, err := s.New(ctx, issues.NewReq{Title: "One"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Undo(ctx, is.ID, false); !errors.Is(err, issues.ErrCantUndo) || !strings.Contains(err.Error(), "created the issue") {
		t.Fatalf("the create: %v", err)
	}
	if _, err := bob.Undo(ctx, is.ID, false); !errors.Is(err, issues.ErrCantUndo) {
		t.Fatalf("bob, no change of his: %v", err)
	}
	if err := s.SetMeta(ctx, is.ID, map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Undo(ctx, is.ID, false); !errors.Is(err, issues.ErrCantUndo) || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("set-meta: %v", err)
	}

	// labels set, then the same labels set back by hand: nothing to undo
	if _, err := s.ChangeLabels(ctx, is.ID, []string{"a"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := bob.ChangeLabels(ctx, is.ID, []string{"b"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Undo(ctx, is.ID, false); !errors.Is(err, issues.ErrWroteAfter) || !strings.Contains(err.Error(), "by Bob") {
		t.Fatalf("bob wrote after: %v", err)
	}
	if _, err := s.Undo(ctx, is.ID, true); err != nil {
		t.Fatalf("--force: %v", err)
	}
	if th, _ := s.Show(ctx, is.ID); !slices.Equal(th.Labels, []string{"b"}) {
		t.Fatalf("labels after --force %q", th.Labels)
	}

	// a forge's pull after it
	if _, err := s.ChangeLabels(ctx, is.ID, []string{"c"}, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"add": []string{"d"}, "remove": []string{}})
	if _, err := issues.Kind.Do(ctx, s.Board, kernel.ID(is.ID), func(kernel.Entity[issues.State]) ([]kernel.Op, error) {
		return []kernel.Op{{Kind: "change-labels", Author: string(bob.Me), Time: 2, Origin: &kernel.Origin{Forge: "github", Event: "1"}, Data: raw}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Undo(ctx, is.ID, false); !errors.Is(err, issues.ErrWroteAfter) || !strings.Contains(err.Error(), "a pull from github") {
		t.Fatalf("a pull after: %v", err)
	}
	// the pull isn't Bob's to undo, though it is under his name: his
	// last change is the label b, and Ada and the pull wrote after it
	if _, err := bob.Undo(ctx, is.ID, false); !errors.Is(err, issues.ErrWroteAfter) {
		t.Fatalf("bob's undo: %v", err)
	}

	// undone already: the change and its undo cancel
	two, err := s.New(ctx, issues.NewReq{Title: "Two"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeLabels(ctx, two.ID, []string{"x"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := bob.ChangeLabels(ctx, two.ID, nil, []string{"x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Undo(ctx, two.ID, true); !errors.Is(err, issues.ErrCantUndo) || !strings.Contains(err.Error(), "undone already") {
		t.Fatalf("undone already: %v", err)
	}

	// no name: the latest change by its time, on any issue (a later
	// second than the others, which all share this one)
	raw, _ = json.Marshal(map[string]any{"status": "closed"})
	if _, err := issues.Kind.Do(ctx, s.Board, kernel.ID(two.ID), func(kernel.Entity[issues.State]) ([]kernel.Op, error) {
		return []kernel.Op{{Kind: "set-status", Author: string(s.Me), Time: time.Now().Unix() + 60, Data: raw}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if u, err := s.Undo(ctx, "", false); err != nil || u.Issue != two.ID {
		t.Fatalf("undo with no name: %+v, %v", u, err)
	}

	// a deleted issue: by name, and with no name when its delete is your
	// latest change. lastWritten dates a change by its time, to the
	// second, and breaks a tie by id: a delete written now ties with
	// the first issue's changes, written this second too, and the tie
	// went to either issue (the first one's refusal is its pull). So the
	// delete is dated two minutes on, after the set-status above.
	raw, _ = json.Marshal(map[string]any{"target": string(mustGet(t, s, two.ID).State.BodyOp)})
	if _, err := issues.Kind.Do(ctx, s.Board, kernel.ID(two.ID), func(kernel.Entity[issues.State]) ([]kernel.Op, error) {
		return []kernel.Op{{Kind: "delete", Author: string(s.Me), Time: time.Now().Unix() + 120, Data: raw}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	// by name it isn't found, as every lookup (ADR 0019)
	if _, err := s.Undo(ctx, two.ID, false); !errors.Is(err, issues.ErrNotFound) {
		t.Fatalf("a deleted issue, by name: %v", err)
	}
	if _, err := s.Undo(ctx, "", false); !errors.Is(err, issues.ErrCantUndo) || !strings.Contains(err.Error(), "deleted") {
		t.Fatalf("a deleted issue: %v", err)
	}
}

func mustGet(t *testing.T, s *issues.Service, id string) kernel.Entity[issues.State] {
	t.Helper()
	e, err := issues.Kind.Get(context.Background(), s.Board, kernel.ID(id))
	if err != nil {
		t.Fatal(err)
	}
	return e
}
