package gitstore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchPush(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	gitRun(t, root, "init", "-q", "--bare", origin)
	var stores [2]*Store
	for i, n := range []string{"a", "b"} {
		dir := filepath.Join(root, n)
		gitRun(t, root, "init", "-q", dir)
		gitRun(t, dir, "remote", "add", "origin", origin)
		s, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		stores[i] = s
	}
	a, b := stores[0], stores[1]
	ctx := context.Background()

	// Nothing to fetch or push yet: not an error.
	if err := b.Fetch(ctx, "origin"); err != nil {
		t.Fatal(err)
	}
	if err := a.Push(ctx, "origin"); err != nil {
		t.Fatal(err)
	}

	c := commitOf(t, a)
	if err := a.UpdateRef("refs/board/issue/x", c, ZeroHash); err != nil {
		t.Fatal(err)
	}
	if err := a.UpdateRef("refs/board/seq", c, ZeroHash); err != nil {
		t.Fatal(err)
	}
	if err := a.Push(ctx, "origin"); err != nil {
		t.Fatal(err)
	}
	if err := b.Fetch(ctx, "origin"); err != nil {
		t.Fatal(err)
	}
	if h, _ := b.Ref("refs/remotes/origin/board/issue/x"); h != c {
		t.Fatalf("fetched = %v, want %v", h, c)
	}
	if h, _ := b.Ref("refs/remotes/origin/board/seq"); h != ZeroHash {
		t.Fatalf("seq fetched: %v", h)
	}
	o, err := Open(origin)
	if err != nil {
		t.Fatal(err)
	}
	if h, _ := o.Ref("refs/board/seq"); h != ZeroHash {
		t.Fatalf("seq pushed: %v", h)
	}
}

// A remote name that is empty or would read as an option is refused, before
// git runs.
func TestRemoteNameChecked(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	for _, r := range []string{"", "-v", "--upload-pack=touch x"} {
		for name, run := range map[string]func(context.Context, string) error{"fetch": s.Fetch, "push": s.Push} {
			if err := run(ctx, r); err == nil || !strings.Contains(err.Error(), "bad remote name") {
				t.Errorf("%s %q: err = %v", name, r, err)
			}
		}
	}
}

func TestFetchManyObjects(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	gitRun(t, root, "init", "-q", "--bare", origin)
	var stores [2]*Store
	for i, n := range []string{"a", "b"} {
		dir := filepath.Join(root, n)
		gitRun(t, root, "init", "-q", dir)
		gitRun(t, dir, "remote", "add", "origin", origin)
		s, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		stores[i] = s
	}
	a, b := stores[0], stores[1]
	gitRun(t, b.gitDir, "config", "fetch.unpackLimit", "1") // keep the fetched pack
	ctx := context.Background()

	// b has looked for an object, so it has read its pack index.
	_, _ = b.ReadCommit(ZeroHash)

	h := commitOf(t, a)
	first := h
	for range 5 {
		h = commitOf(t, a, h)
	}
	if err := a.UpdateRef("refs/board/issue/x", h, ZeroHash); err != nil {
		t.Fatal(err)
	}
	if err := a.Push(ctx, "origin"); err != nil {
		t.Fatal(err)
	}
	if err := b.Fetch(ctx, "origin"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []Hash{h, first} {
		if _, err := b.ReadCommit(c); err != nil {
			t.Fatalf("read %s: %v", c, err)
		}
	}
}
