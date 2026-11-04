package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/pgstore/pgtest"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// pgRepo is a repository whose board is kept in a fresh PostgreSQL
// database, with someone chosen to write as there.
func pgRepo(t *testing.T) (dir, url string) {
	t.Helper()
	url = pgURL(t)
	dir = storetest.Repo(t, false)
	storetest.Git(t, dir, "config", store.DatastoreKey, url)
	storetest.Init(t, dir)
	return dir, url
}

// pgURL is a fresh database as the postgres:// URL board.datastore takes.
func pgURL(t *testing.T) string {
	t.Helper()
	return pgtest.URL(t)
}

// A board kept in PostgreSQL: its issues, comments, search and numbers
// work, and nothing of it is in git.
func TestPostgresBoard(t *testing.T) {
	t.Parallel()
	dir, _ := pgRepo(t)
	b := storetest.Open(t, dir)
	if c := b.Capabilities(); c.Datastore != "PostgreSQL" || c.Replica || !c.Counter {
		t.Fatalf("capabilities %+v", c)
	}
	id, err := b.NewIssue("The crash on save", "it crashes", []string{"bug"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddComment(id[:8], "seen it too"); err != nil {
		t.Fatal(err)
	}
	if th, err := b.Thread(id[:8]); err != nil || th.Title != "The crash on save" || comments(th) != 1 {
		t.Fatalf("issue %+v, %v", th, err)
	}
	if hits, err := b.SearchText("crash"); err != nil || !hits[id] {
		t.Fatalf("search: %v, %v", hits, err)
	}
	if refs := storetest.Git(t, dir, "for-each-ref", "refs/board/"); refs != "" {
		t.Fatalf("refs in git: %s", refs)
	}

	// another clone naming the same database sees the board at once
	other := storetest.Repo(t, false)
	url := storetest.Git(t, dir, "config", store.DatastoreKey)
	storetest.Git(t, other, "config", store.DatastoreKey, strings.TrimSpace(url))
	ob := storetest.Open(t, other)
	if issues, err := ob.ListIssues(); err != nil || len(issues) != 1 || issues[0].ID != id {
		t.Fatalf("the other clone lists %+v, %v", issues, err)
	}

	// numbers come from the database's counter
	if err := b.InitNumbers(store.NumbersRemote, "CO2", 3052); err != nil {
		t.Fatal(err)
	}
	if err := ob.InitNumbers(store.NumbersRemote, "XX", 1); !errors.Is(err, store.ErrNumbered) {
		t.Fatalf("a second start: %v, want ErrNumbered", err)
	}
	if n, err := ob.NumberIssues(store.NumbersRemote); err != nil || n != 1 {
		t.Fatalf("numbered %d, %v; want 1", n, err)
	}
	if is, err := b.GetIssue(id); err != nil || is.Number != "CO2-3052" {
		t.Fatalf("number %q, %v", is.Number, err)
	}

	// what only git does says so
	if err := b.Pull("origin"); !errors.Is(err, datastore.ErrMissing) {
		t.Fatalf("pull: %v, want ErrMissing", err)
	}
	if _, err := b.ResetHard(""); !errors.Is(err, datastore.ErrMissing) {
		t.Fatalf("start over: %v, want ErrMissing", err)
	}

	// who I am is this clone's, in its git config
	if me := strings.TrimSpace(storetest.Git(t, dir, "config", "board.me")); me == "" {
		t.Fatal("board.me unset")
	}
}

// A board in git copies into PostgreSQL with its ids and numbers.
func TestCopyFromGit(t *testing.T) {
	t.Parallel()
	url := pgURL(t)
	dir := storetest.Repo(t, false)
	storetest.Init(t, dir)
	b := storetest.Open(t, dir)
	id, err := b.NewIssue("kept", "in git first", nil)
	if err != nil {
		t.Fatal(err)
	}
	cid, err := b.AddComment(id, "a comment")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CopyFromGit(t.Context()); err == nil {
		t.Fatal("copy with no datastore named: no error")
	}
	storetest.Git(t, dir, "config", store.DatastoreKey, url)
	pb := storetest.Open(t, dir)
	n, err := pb.CopyFromGit(t.Context())
	if err != nil || n != 2 { // the issue and its author
		t.Fatalf("copied %d, %v; want 2", n, err)
	}
	th, err := pb.Thread(id)
	if err != nil || comments(th) != 1 || th.Events[len(th.Events)-1].ID != cid {
		t.Fatalf("copied issue %+v, %v; want comment %s", th, err, cid)
	}
	if n, err := pb.CopyFromGit(t.Context()); err != nil || n != 0 {
		t.Fatalf("copied again %d, %v", n, err)
	}
}

// comments counts a thread's comments.
func comments(th store.Thread) int {
	n := 0
	for _, e := range th.Events {
		if e.Kind == "comment" {
			n++
		}
	}
	return n
}

// No password reaches a page or a message, in any form pgx takes.
func TestRedact(t *testing.T) {
	t.Parallel()
	for _, v := range []string{
		"postgres://u:s3cret@db/b",
		"postgres://u@db/b?password=s3cret&sslmode=disable",
		"host=db user=u password=s3cret dbname=b",
		"host=db password='s 3cret' dbname=b",
	} {
		if got := store.Redact(v); strings.Contains(got, "s3cret") || strings.Contains(got, "3cret") {
			t.Errorf("Redact(%q) = %q", v, got)
		}
	}
	if got := store.Redact("markdown:issues"); got != "markdown:issues" {
		t.Errorf("Redact(markdown:issues) = %q", got)
	}
}
