package issue

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/guilbep/git-board/core/oplog"
)

func TestMergeConverges(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	gitRun(t, root, "init", "-q", "--bare", origin)
	var dirs []string
	for _, n := range []string{"a", "b"} {
		dir := filepath.Join(root, n)
		gitRun(t, root, "clone", "-q", origin, dir)
		dirs = append(dirs, dir)
	}
	a, b := open(t, dirs[0]), open(t, dirs[1])
	ctx := context.Background()
	must2 := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	c, err := Create(a, NewCreate("ann", "T", "B"))
	c = must(t, c, err)
	must2(a.Push(ctx, "origin"))
	must2(b.Fetch(ctx, "origin"))
	_, err = oplog.MergeFetched(b, "origin")
	must2(err)

	_, err = Do(a, c.ID, SetTitle("ann", "from A"), ChangeLabels("ann", []string{"a"}, nil))
	must2(err)
	_, err = Do(b, c.ID, NewComment("bob", "hi"), SetTitle("bob", "from B"))
	must2(err)

	must2(a.Push(ctx, "origin"))
	must2(b.Fetch(ctx, "origin"))
	_, err = oplog.MergeFetched(b, "origin")
	must2(err)
	must2(b.Push(ctx, "origin"))
	must2(a.Fetch(ctx, "origin"))
	_, err = oplog.MergeFetched(a, "origin")
	must2(err)

	ia, err := Load(a, c.ID)
	ia = must(t, ia, err)
	ib, err := Load(b, c.ID)
	ib = must(t, ib, err)
	if !reflect.DeepEqual(ia, ib) {
		t.Fatalf("A = %+v\nB = %+v", ia, ib)
	}
	// The title is that of the last set-title in the shared order.
	l, err := oplog.Load(a, "issue", c.ID)
	must2(err)
	var last string
	for _, e := range l.Entries {
		if e.Kind == "set-title" {
			last = map[bool]string{true: "from A", false: "from B"}[e.Author == "ann"]
		}
	}
	if ia.Title != last || len(ia.Comments) != 1 || !reflect.DeepEqual(ia.Labels, []string{"a"}) {
		t.Fatalf("issue = %+v, want title %q", ia, last)
	}
}
