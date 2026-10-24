package issue

import (
	"errors"
	"os/exec"
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
	c, err := Create(s, NewCreate("ann", "T", "B"))
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
	c, err := Create(s, NewCreate("ann", "T", "B"))
	c = must(t, c, err)
	c, err = Do(s, c.ID, NewComment("bob", "first"))
	c = must(t, c, err)
	cid := c.Comments[0].ID
	c, err = Do(s, c.ID,
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
	if _, err := Create(newStore(t), NewComment("a", "x")); err == nil {
		t.Fatal("want an error")
	}
}

func TestSecondCreateAndBadStatus(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c, err := Create(s, NewCreate("a", "T", ""))
	c = must(t, c, err)
	if _, err := Do(s, c.ID, NewCreate("a", "U", "")); err == nil {
		t.Fatal("second create: want an error")
	}
	s = newStore(t)
	if _, err := Create(s, NewCreate("a", "T", ""), SetStatus("a", "wip")); err == nil {
		t.Fatal("bad status: want an error")
	}
}

func TestUnknownOpIsAnError(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	l, err := oplog.Create(s, "issue", "", []oplog.Op{NewCreate("a", "T", ""), {Kind: "set-priority"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Apply(l)
	if !errors.Is(err, ErrUnknownOp) || !strings.Contains(err.Error(), "set-priority") {
		t.Fatalf("err = %v", err)
	}
}

func TestEditUnknownTarget(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	_, err := Create(s, NewCreate("a", "T", ""), EditText("a", "nope:0", "x"))
	if !errors.Is(err, ErrBadTarget) {
		t.Fatalf("err = %v", err)
	}
}
