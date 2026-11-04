package pgstore_test

import (
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/syncer"
)

// A board kept in PostgreSQL syncs with a forge as a git one does: its
// bookkeeping is sync entities, packs like any other (ADR 0002, section 2).
func TestSyncWithAForge(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := t.Context()
	f := fake.New("example.com/p")
	ann := forge.Account{ID: "7", Login: "ann"}
	f.Add(forge.Issue{ID: "100", Number: "1", URL: "https://example.com/p/1", Author: ann, Created: 1000,
		Title: "T", Body: "B", Status: "open",
		Comments: []forge.Comment{{ID: "c1", Body: "hi", Author: ann, Time: 1100}}})
	if _, err := syncer.Sync(ctx, s, syncer.Options{}, f); err != nil {
		t.Fatal(err)
	}
	ids, err := issue.List(s)
	if err != nil || len(ids) != 1 {
		t.Fatalf("issues %v, %v; want the one pulled", ids, err)
	}
	is, err := issue.Load(s, ids[0])
	if err != nil || is.Title != "T" || len(is.Comments) != 1 {
		t.Fatalf("pulled %+v, %v", is, err)
	}

	// a comment written here goes to the forge, once
	if _, err := issue.Do(ctx, s, is.ID, issue.NewComment(is.Author, "from git-board")); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := syncer.Sync(ctx, s, syncer.Options{}, f); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := f.Get("1")
	if len(got.Comments) != 2 || got.Comments[1].Body == "" {
		t.Fatalf("forge comments %+v; want the one written here, once", got.Comments)
	}
	if is, _ = issue.Load(s, is.ID); len(is.Comments) != 2 {
		t.Fatalf("comments here %d; want 2, no echo", len(is.Comments))
	}
	if ids, _ := issue.List(s); len(ids) != 1 {
		t.Fatalf("issues %v; want no duplicate", ids)
	}
}
