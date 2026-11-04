package memstore_test

import (
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/datastore/dstest"
	"github.com/guilbep/git-board/core/datastore/memstore"
)

func TestDatastore(t *testing.T) {
	t.Parallel()
	dstest.Run(t, func(t *testing.T) datastore.Datastore { return memstore.New(t.TempDir()) })
}

// A copy cut short is finished by the next: the packs to lacks, and no
// others.
func TestCopyFinishesACutCopy(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	from, to := memstore.New(t.TempDir()), memstore.New(t.TempDir())
	id := strings.Repeat("a", 40)
	v := ""
	var err error
	for _, p := range []string{`{"clock":1}`, `{"clock":2}`, `{"clock":3}`} {
		if v, err = from.Add(ctx, "issue", id, []byte(p), v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := to.Add(ctx, "issue", id, []byte(`{"clock":1}`), ""); err != nil { // the copy stopped here
		t.Fatal(err)
	}
	if n, err := datastore.Copy(ctx, from, to); err != nil || n != 1 {
		t.Fatalf("copied %d, %v", n, err)
	}
	if packs, _, _ := to.Packs(ctx, "issue", id); len(packs) != 3 {
		t.Fatalf("%d packs, want 3", len(packs))
	}
	if n, err := datastore.Copy(ctx, from, to); err != nil || n != 0 {
		t.Fatalf("again: %d, %v", n, err)
	}
}
