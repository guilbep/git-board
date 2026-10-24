package issue

import (
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/oplog"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func open(t *testing.T, dir string) *gitstore.Store {
	t.Helper()
	s, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newStore(t *testing.T) *gitstore.Store {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	return open(t, dir)
}

func must(t *testing.T, is *Issue, err error) *Issue {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return is
}

func TestCreateAndLoad(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c, err := Create(t.Context(), s, NewCreate("ann", "T", "B"))
	c = must(t, c, err)
	l, err := oplog.Load(s, "issue", c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != l.ID || c.Title != "T" || c.Body != "B" || c.Status != Open || c.Author != "ann" || c.BodyOp != c.ID+":0" {
		t.Fatalf("issue = %+v", c)
	}
	g, err := Load(s, c.ID)
	if g = must(t, g, err); !reflect.DeepEqual(g, c) {
		t.Fatalf("load = %+v, want %+v", g, c)
	}
	ids, err := List(s)
	if err != nil || len(ids) != 1 || ids[0] != c.ID {
		t.Fatalf("list = %v, %v", ids, err)
	}
}

func TestEveryOp(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c, err := Create(t.Context(), s, NewCreate("ann", "T", "B"))
	c = must(t, c, err)
	c, err = Do(t.Context(), s, c.ID, NewComment("bob", "first"))
	c = must(t, c, err)
	cid := c.Comments[0].ID
	c, err = Do(t.Context(), s, c.ID,
		EditText("ann", c.BodyOp, "B2"),
		EditText("bob", cid, "first2"),
		SetTitle("ann", "T2"),
		SetStatus("ann", Closed),
		ChangeLabels("ann", []string{"bug", "ui", "assignee:p1"}, nil),
		ChangeLabels("ann", nil, []string{"ui"}),
	)
	c = must(t, c, err)
	if c.Title != "T2" || c.Body != "B2" || c.Status != Closed || c.Author != "ann" {
		t.Fatalf("issue = %+v", c)
	}
	if want := []string{"assignee:p1", "bug"}; !reflect.DeepEqual(c.Labels, want) {
		t.Fatalf("labels = %v, want %v", c.Labels, want)
	}
	if len(c.Comments) != 1 || c.Comments[0].Body != "first2" || c.Comments[0].Author != "bob" || c.Comments[0].ID != cid {
		t.Fatalf("comments = %+v", c.Comments)
	}
}

func TestFirstOpMustBeCreate(t *testing.T) {
	t.Parallel()
	if _, err := Create(t.Context(), newStore(t), NewComment("a", "x")); err == nil {
		t.Fatal("want an error")
	}
}

func TestSecondCreateAndBadStatus(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c, err := Create(t.Context(), s, NewCreate("a", "T", ""))
	c = must(t, c, err)
	if _, err := Do(t.Context(), s, c.ID, NewCreate("a", "U", "")); err == nil {
		t.Fatal("second create: want an error")
	}
	rejected(t, s, c, NewCreate("a", "U", ""))
	rejected(t, s, c, SetStatus("a", "wip"))
	rejected(t, s, c, EditText("a", "nope:0", "x"))

	s = newStore(t)
	if _, err := Create(t.Context(), s, NewCreate("a", "T", ""), SetStatus("a", "wip")); err == nil {
		t.Fatal("bad status: want an error")
	}
	if ids, err := List(s); err != nil || len(ids) != 0 {
		t.Fatalf("rejected Create left %v, %v", ids, err)
	}
}

// rejected checks that Do refuses op and leaves the issue as it was.
func rejected(t *testing.T, s *gitstore.Store, want *Issue, op oplog.Op) {
	t.Helper()
	if _, err := Do(t.Context(), s, want.ID, op); err == nil {
		t.Fatalf("%s: want an error", op.Kind)
	}
	got, err := Load(s, want.ID)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("after rejected %s: %+v, %v", op.Kind, got, err)
	}
}

func TestUnknownOpIsAnError(t *testing.T) {
	t.Parallel()
	for _, data := range []string{"", `{"title":5}`} {
		s := newStore(t)
		l, err := oplog.Create(t.Context(), s, "issue", "", []oplog.Op{NewCreate("a", "T", ""), {Kind: "set-priority", Author: "a", Time: 1, Data: []byte(data)}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = Apply(l)
		if !errors.Is(err, ErrUnknownOp) || !strings.Contains(err.Error(), "set-priority") {
			t.Fatalf("data %q: err = %v", data, err)
		}
	}
}

func withOrigin(op oplog.Op, forge, event string) oplog.Op {
	op.Origin = &oplog.Origin{Forge: forge, Event: event}
	return op
}

func bodies(is *Issue) []string {
	var out []string
	for _, c := range is.Comments {
		out = append(out, c.Body)
	}
	return out
}

// Two comments with the same origin, as when two clones pull one forge
// comment, are one comment: the first. An edit of either id edits it.
func TestDuplicateOriginCommentIsOne(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c, err := Create(t.Context(), s, NewCreate("a", "T", ""),
		withOrigin(NewComment("bob", "one"), "f", "comment:1"),
		withOrigin(NewComment("bob", "two"), "f", "comment:1"),
		NewComment("ann", "plain"), NewComment("ann", "plain"), // no origin: never one
		withOrigin(NewComment("bob", "x"), "f", "comment:2"), // another event
		withOrigin(NewComment("bob", "y"), "g", "comment:2"), // another forge
	)
	c = must(t, c, err)
	first, second := c.ID+":1", c.ID+":2"
	if want := []string{"one", "plain", "plain", "x", "y"}; !reflect.DeepEqual(bodies(c), want) || c.Comments[0].ID != first {
		t.Fatalf("comments = %+v, want bodies %v", c.Comments, want)
	}
	c, err = Do(t.Context(), s, c.ID, EditText("bob", second, "edited"))
	if c = must(t, c, err); c.Comments[0].Body != "edited" || len(c.Comments) != 5 {
		t.Fatalf("after an edit of the second id: %+v", c.Comments)
	}
	c, err = Do(t.Context(), s, c.ID, EditText("bob", first, "again"))
	if c = must(t, c, err); c.Comments[0].Body != "again" || len(c.Comments) != 5 {
		t.Fatalf("after an edit of the first id: %+v", c.Comments)
	}
}

// A target naming an operation not written yet is refused, before anything
// is written: once stored, it would name nothing.
func TestPendingTargetRefused(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	if _, err := Create(t.Context(), s, NewCreate("a", "T", ""), EditText("a", "pending:0", "x")); !errors.Is(err, ErrBadTarget) {
		t.Fatalf("create: err = %v", err)
	}
	if ids, err := List(s); err != nil || len(ids) != 0 {
		t.Fatalf("rejected Create left %v, %v", ids, err)
	}
	c, err := Create(t.Context(), s, NewCreate("a", "T", ""))
	c = must(t, c, err)
	if _, err := Do(t.Context(), s, c.ID, NewComment("a", "x"), EditText("a", "pending:0", "y")); !errors.Is(err, ErrBadTarget) {
		t.Fatalf("do: err = %v", err)
	}
	if got, err := Load(s, c.ID); err != nil || !reflect.DeepEqual(got, c) {
		t.Fatalf("after rejected Do: %+v, %v", got, err)
	}
}

// A label is a non-empty string with no line feed, in add and in remove.
func TestBadLabelRefused(t *testing.T) {
	t.Parallel()
	for _, l := range []string{"", "a\nb"} {
		s := newStore(t)
		if _, err := Create(t.Context(), s, NewCreate("a", "T", ""), ChangeLabels("a", []string{l}, nil)); err == nil {
			t.Fatalf("create with label %q: want an error", l)
		}
		if ids, err := List(s); err != nil || len(ids) != 0 {
			t.Fatalf("rejected Create left %v, %v", ids, err)
		}
		c, err := Create(t.Context(), s, NewCreate("a", "T", ""))
		c = must(t, c, err)
		rejected(t, s, c, ChangeLabels("a", []string{l}, nil))
		rejected(t, s, c, ChangeLabels("a", nil, []string{l}))
	}
}

// A linked worktree shares its main repository's refs and objects.
func TestOpenLinkedWorktree(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	main, wt := filepath.Join(root, "main"), filepath.Join(root, "wt")
	gitRun(t, root, "init", "-q", main)
	gitRun(t, main, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "--no-gpg-sign", "-m", "x")
	first, err := Create(t.Context(), open(t, main), NewCreate("a", "T", ""))
	first = must(t, first, err)
	gitRun(t, main, "worktree", "add", "-q", wt)

	w := open(t, wt)
	if ids, err := List(w); err != nil || len(ids) != 1 || ids[0] != first.ID {
		t.Fatalf("list from the worktree = %v, %v", ids, err)
	}
	second, err := Create(t.Context(), w, NewCreate("b", "U", ""))
	second = must(t, second, err)
	got, err := Load(open(t, main), second.ID)
	if got = must(t, got, err); !reflect.DeepEqual(got, second) {
		t.Fatalf("from main = %+v, want %+v", got, second)
	}
}

func TestEditUnknownTarget(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	_, err := Create(t.Context(), s, NewCreate("a", "T", ""), EditText("a", "nope:0", "x"))
	if !errors.Is(err, ErrBadTarget) {
		t.Fatalf("err = %v", err)
	}
}

func at(op oplog.Op, t int64) oplog.Op {
	op.Time = t
	return op
}

// Updated is the last operation's time in the order, not the largest time;
// Edited is the last edit's, by any id of the comment.
func TestUpdatedAndEdited(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c, err := Create(t.Context(), s,
		at(NewCreate("a", "T", ""), 100),
		at(withOrigin(NewComment("bob", "one"), "f", "comment:1"), 500),
		at(withOrigin(NewComment("bob", "dup"), "f", "comment:1"), 110), // same comment, alias id
		at(NewComment("bob", "other"), 120),
	)
	c = must(t, c, err)
	if c.Updated != 120 || c.Comments[0].Edited != 0 || c.Comments[1].Edited != 0 {
		t.Fatalf("updated = %d, comments = %+v", c.Updated, c.Comments)
	}
	alias := c.ID + ":2"
	c, err = Do(t.Context(), s, c.ID,
		at(EditText("bob", c.Comments[0].ID, "e1"), 900),
		at(EditText("bob", alias, "e2"), 300), // later in the order, earlier in time
	)
	c = must(t, c, err)
	if c.Comments[0].Edited != 300 || c.Comments[1].Edited != 0 || c.Updated != 300 {
		t.Fatalf("updated = %d, comments = %+v", c.Updated, c.Comments)
	}
	c, err = Do(t.Context(), s, c.ID, at(SetTitle("a", "U"), 50))
	if c = must(t, c, err); c.Updated != 50 || c.Comments[0].Edited != 300 {
		t.Fatalf("updated = %d, comments = %+v", c.Updated, c.Comments)
	}
}

func TestAddRemoveSameLabelInOneOp(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c, err := Create(t.Context(), s, NewCreate("a", "T", ""), ChangeLabels("a", []string{"x", "y"}, []string{"x"}))
	if c = must(t, c, err); !reflect.DeepEqual(c.Labels, []string{"y"}) {
		t.Fatalf("labels = %v", c.Labels)
	}
}

func TestBodyEditAfterCommentEdit(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c, err := Create(t.Context(), s, NewCreate("a", "T", "B"), NewComment("b", "c"))
	c = must(t, c, err)
	c, err = Do(t.Context(), s, c.ID, EditText("b", c.Comments[0].ID, "c2"), EditText("a", c.BodyOp, "B2"))
	if c = must(t, c, err); c.Body != "B2" || c.Comments[0].Body != "c2" {
		t.Fatalf("issue = %+v", c)
	}
}

// Rule 8: an operation with no author is an error, on a write and on a load.
func TestEmptyAuthorIsRejected(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	if _, err := Create(t.Context(), s, NewCreate("", "T", "")); err == nil {
		t.Fatal("create: want an error")
	}
	c, err := Create(t.Context(), s, NewCreate("a", "T", ""))
	c = must(t, c, err)
	rejected(t, s, c, NewComment("", "x"))
	l, err := oplog.Create(t.Context(), newStore(t), "issue", "", []oplog.Op{NewCreate("a", "T", ""), NewComment("", "x")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(l); err == nil {
		t.Fatal("apply: want an error")
	}
}
