package main

import (
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/store/storetest"
)

// The git remote alone: A pushes, B fetches and sees what waits, then
// pulls it. Serial: they chdir.
func TestPushThenPull(t *testing.T) {
	pathA, pathB := twoReposWithRemote(t)
	seedBug(t, pathA, "from A")

	t.Chdir(pathA)
	if out, err := execute(t, []string{"push"}); err != nil || !strings.Contains(out, "pushed to origin") {
		t.Fatalf("push: %v\n%s", err, out)
	}

	t.Chdir(pathB)
	out, err := execute(t, []string{"fetch"})
	if err != nil || !strings.Contains(out, "origin: issues 1 to receive") {
		t.Fatalf("fetch: %v\n%s", err, out)
	}
	if got := bugTitles(t, pathB); len(got) != 0 {
		t.Fatalf("the fetch merged %v", got)
	}
	out, err = execute(t, []string{"pull"})
	if err != nil || !strings.Contains(out, "pulled from origin") {
		t.Fatalf("pull: %v\n%s", err, out)
	}
	if got := bugTitles(t, pathB); !containsLabel(got, "from A") {
		t.Errorf("B after the pull: %v", got)
	}
}

func TestPullNamesAMissingRemote(t *testing.T) {
	pathA, _ := twoReposWithRemote(t)
	t.Chdir(pathA)
	out, err := execute(t, []string{"pull", "upstream"})
	if err == nil || !strings.Contains(err.Error(), `no remote "upstream"`) {
		t.Fatalf("err = %v\n%s", err, out)
	}
}

// A push origin refuses, as it holds what this clone never pulled, says
// to pull first: push is a command of its own now, not a step of sync.
func TestPushAfterOriginMoved(t *testing.T) {
	pathA, pathB := twoReposWithRemote(t)
	a := storetest.Open(t, pathA)
	id := storetest.NewIssue(t, a, "shared")
	t.Chdir(pathA)
	if out, err := execute(t, []string{"push"}); err != nil {
		t.Fatalf("A push: %v\n%s", err, out)
	}
	t.Chdir(pathB)
	if out, err := execute(t, []string{"pull"}); err != nil {
		t.Fatalf("B pull: %v\n%s", err, out)
	}

	me, _, _ := a.Me()
	storetest.CommentAs(t, a, id, me, 1, "from A")
	t.Chdir(pathA)
	if out, err := execute(t, []string{"push"}); err != nil {
		t.Fatalf("A push again: %v\n%s", err, out)
	}

	b := storetest.Open(t, pathB)
	them, _, _ := b.Me()
	storetest.CommentAs(t, b, id, them, 2, "from B")
	t.Chdir(pathB)
	out, err := execute(t, []string{"push"})
	if err == nil || !strings.Contains(err.Error(), "origin has changes this repository doesn't have: git board pull first") {
		t.Fatalf("err = %v\n%s", err, out)
	}
}

func TestPullWaitsForTheSwitch(t *testing.T) {
	path, _ := twoReposWithRemote(t)
	storetest.Git(t, path, "update-ref", "refs/bugs/"+strings.Repeat("a", 64), strings.TrimSpace(storetest.Git(t, path, "hash-object", "-w", "--stdin")))
	t.Chdir(path)
	out, err := execute(t, []string{"pull"})
	if err == nil || !strings.Contains(err.Error(), "run git-board switch") {
		t.Fatalf("err = %v\n%s", err, out)
	}
}
