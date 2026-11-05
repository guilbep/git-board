package app

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/issues/issuesync"
)

// Two clones number their issues through origin's counter: a push numbers
// what origin then has, a pull takes the numbers.
func TestNumbersThroughOrigin(t *testing.T) {
	t.Parallel()
	origin := filepath.Join(t.TempDir(), "origin.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", origin).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	open := func() *Board {
		dir := filepath.Join(t.TempDir(), "clone")
		if out, err := exec.Command("git", "clone", "-q", origin, dir).CombinedOutput(); err != nil {
			t.Fatalf("git clone: %v\n%s", err, out)
		}
		b, err := Open(t.Context(), dir, Options{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { b.Close() })
		return b
	}
	numbers := func(b *Board) map[string]string {
		t.Helper()
		list, err := b.Issues.List(t.Context(), issues.ListReq{})
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]string{}
		for _, is := range list {
			m[is.Title] = is.Number
		}
		return m
	}
	a, b := open(), open()
	if where, err := a.InitNumbers(t.Context(), "GB", 10); err != nil || where != "on origin" {
		t.Fatalf("init: %q, %v", where, err)
	}
	if _, err := b.InitNumbers(t.Context(), "XY", 1); err == nil {
		t.Fatal("a second start")
	}
	if _, err := issuesync.CreateIssue(t.Context(), a.Kernel, issuesync.NewCreate("a", "One", "")); err != nil {
		t.Fatal(err)
	}
	if got := numbers(a)["One"]; got != "" {
		t.Fatalf("numbered before its push: %q", got)
	}
	if _, err := a.Push(t.Context(), "origin"); err != nil {
		t.Fatal(err)
	}
	if got := numbers(a)["One"]; got != "GB-10" {
		t.Fatalf("after the push: %q", got)
	}
	if _, err := b.Pull(t.Context(), "origin"); err != nil {
		t.Fatal(err)
	}
	if _, err := issuesync.CreateIssue(t.Context(), b.Kernel, issuesync.NewCreate("b", "Two", "")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Push(t.Context(), "origin"); err != nil {
		t.Fatal(err)
	}
	if got := numbers(b); got["One"] != "GB-10" || got["Two"] != "GB-11" {
		t.Fatalf("b reads %v", got)
	}
	if _, err := a.Pull(t.Context(), "origin"); err != nil {
		t.Fatal(err)
	}
	if got := numbers(a); got["Two"] != "GB-11" {
		t.Fatalf("a reads %v", got)
	}
}
