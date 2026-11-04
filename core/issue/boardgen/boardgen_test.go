package boardgen

import (
	"context"
	"testing"

	"github.com/guilbep/git-board/core/datastore/memstore"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/people"
)

func TestMakeFoldsEveryIssue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ds := memstore.New(t.TempDir())
	b, err := Make(ctx, ds, Options{Issues: 30, Persons: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Issues) != 30 || len(b.Persons) != 3 {
		t.Fatalf("made %d issues, %d persons", len(b.Issues), len(b.Persons))
	}
	closed := 0
	for _, id := range b.Issues {
		is, err := issue.Load(ds, id)
		if err != nil {
			t.Fatal(err)
		}
		if len(is.Labels) < 5 || is.Body == "" {
			t.Fatalf("issue %s: labels %q, body %q", id, is.Labels, is.Body)
		}
		if is.Status == issue.Closed {
			closed++
		}
	}
	if closed == 0 || closed == 30 {
		t.Fatalf("%d of 30 closed", closed)
	}
}

func TestGitWritesTheBoardInARepository(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	b, err := Git(ctx, dir, Options{Issues: 5, Persons: 2})
	if err != nil {
		t.Fatal(err)
	}
	s, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range b.Issues {
		if _, err := issue.Load(s, id); err != nil {
			t.Fatal(err)
		}
	}
	if me, err := people.Me(ctx, s); err != nil || me != b.Persons[0] {
		t.Fatalf("me = %q, %v; want %s", me, err, b.Persons[0])
	}
}
