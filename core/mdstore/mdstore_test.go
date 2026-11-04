package mdstore_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/datastore/dstest"
	"github.com/guilbep/git-board/core/internal/hermetic"
	"github.com/guilbep/git-board/core/mdstore"
	"github.com/guilbep/git-board/core/oplog"
)

func TestMain(m *testing.M) { hermetic.Run(m) }

func open(t *testing.T, dir string) *mdstore.Store {
	t.Helper()
	s, err := mdstore.Open(dir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDatastore(t *testing.T) {
	t.Parallel()
	dstest.Run(t, func(t *testing.T) datastore.Datastore { return open(t, t.TempDir()) })
}

// The packs are files a merge joins: two writers' packs on one version,
// unioned by git, are both the entity's, in oplog's order.
func TestUnionMergedPacks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	s := open(t, dir)
	if data, err := os.ReadFile(filepath.Join(dir, mdstore.Board, ".gitattributes")); err != nil || string(data) != "*.jsonl merge=union\n" {
		t.Fatalf(".gitattributes %q, %v", data, err)
	}
	l, err := oplog.Create(ctx, s, "issue", "", []oplog.Op{{Kind: "create", Time: 1}})
	if err != nil {
		t.Fatal(err)
	}
	// another branch wrote a pack on the same version; git's union kept both
	other := t.TempDir()
	o := open(t, other)
	packs, _, _ := s.Packs(ctx, "issue", l.ID)
	if _, err := o.Add(ctx, "issue", l.ID, packs[0], ""); err != nil {
		t.Fatal(err)
	}
	theirs, err := oplog.Load(o, "issue", l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := theirs.Append(ctx, o, []oplog.Op{{Kind: "theirs", Time: 2}}); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(ctx, s, []oplog.Op{{Kind: "ours", Time: 2}}); err != nil {
		t.Fatal(err)
	}
	ourFile := filepath.Join(dir, mdstore.Board, "issue", l.ID+".jsonl")
	theirFile := filepath.Join(other, mdstore.Board, "issue", l.ID+".jsonl")
	a, _ := os.ReadFile(ourFile)
	b, _ := os.ReadFile(theirFile)
	if err := os.WriteFile(ourFile, append(a, b...), 0o644); err != nil { // the union, the shared line twice
		t.Fatal(err)
	}
	got, err := oplog.Load(s, "issue", l.ID)
	if err != nil || len(got.Entries) != 3 {
		t.Fatalf("merged %+v, %v; want create, ours and theirs", got, err)
	}
	if err := got.Append(ctx, s, []oplog.Op{{Kind: "after", Time: 3}}); err != nil {
		t.Fatal(err)
	}
}

// Two branches' files of as many packs are two versions: a switch shows.
// A file a hand edit left without its last line feed takes the next pack
// on a line of its own.
func TestVersionIsTheFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	s := open(t, dir)
	l, err := oplog.Create(ctx, s, "issue", "", []oplog.Op{{Kind: "create", Time: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append(ctx, s, []oplog.Op{{Kind: "on-a", Time: 2}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, mdstore.Board, "issue", l.ID+".jsonl")
	onA, _ := os.ReadFile(path)
	heads, _ := s.Heads(ctx, "issue")
	a := heads[l.ID]
	other := strings.Replace(string(onA), `"on-a"`, `"on-b"`, 1) // branch b, as many packs
	if err := os.WriteFile(path, []byte(strings.TrimSuffix(other, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	heads, _ = s.Heads(ctx, "issue")
	if heads[l.ID] == a {
		t.Fatal("another file, the same version")
	}
	if _, err := s.Add(ctx, "issue", l.ID, []byte(`{"clock":3,"ops":[{"kind":"x","time":3}]}`), a); err == nil {
		t.Fatal("a write on the other branch's version landed")
	}
	got, err := oplog.Load(s, "issue", l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := got.Append(ctx, s, []oplog.Op{{Kind: "after", Time: 3}}); err != nil {
		t.Fatal(err)
	}
	if got, err := oplog.Load(s, "issue", l.ID); err != nil || len(got.Entries) != 3 || got.Entries[1].Kind != "on-b" {
		t.Fatalf("%+v, %v", got, err)
	}
}
