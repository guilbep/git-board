package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

func TestMeShowsChoosesAndCreatesWhoYouWriteAs(t *testing.T) {
	dir := storetest.Repo(t, false)
	storetest.Git(t, dir, "config", "user.name", "Ada")
	storetest.Git(t, dir, "config", "user.email", "ada@example.com")
	t.Chdir(dir)

	if _, err := execute(t, []string{"me"}); exitCode(err) != exitNoMe {
		t.Fatalf("me on a fresh board = %v, want exit %d", err, exitNoMe)
	}
	out, err := execute(t, []string{"me", "--new"})
	if err != nil || !strings.Contains(out, "Ada") {
		t.Fatalf("me --new: %v\n%s", err, out)
	}
	if _, err := execute(t, []string{"me", "--new"}); err == nil {
		t.Error("me --new twice: want a refusal naming the person already there")
	}
	out, err = execute(t, []string{"me", "--new", "--name", "Bob", "--json"})
	if err != nil {
		t.Fatalf("me --new --name Bob: %v\n%s", err, out)
	}
	var bob model.Person
	if err := json.Unmarshal([]byte(out), &bob); err != nil || bob.Name != "Bob" {
		t.Fatalf("me --new --json = %s, %v", out, err)
	}

	out, err = execute(t, []string{"me", "--ls", "--json"})
	if err != nil {
		t.Fatalf("me --ls: %v", err)
	}
	var everyone []struct {
		model.Person
		Me bool `json:"me"`
	}
	if err := json.Unmarshal([]byte(out), &everyone); err != nil || len(everyone) != 2 {
		t.Fatalf("me --ls --json = %s, %v", out, err)
	}
	ada := everyone[0]
	if ada.Name != "Ada" || ada.Me || !everyone[1].Me {
		t.Fatalf("me --ls = %+v, want Ada, then Bob (me)", everyone)
	}

	if out, err := execute(t, []string{"me", ada.ID[:7]}); err != nil || !strings.HasPrefix(out, "Ada") {
		t.Fatalf("me <ada>: %v\n%s", err, out)
	}
	if out, err := execute(t, []string{"me"}); err != nil || !strings.HasPrefix(out, "Ada (") {
		t.Errorf("me = %q, %v; want Ada", out, err)
	}
	if _, err := execute(t, []string{"me", "nobody"}); exitCode(err) != exitNotFound {
		t.Errorf("me nobody = %v, want exit %d", err, exitNotFound)
	}
}

func TestExitCodes(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]int{
		errors.New("boom"):         exitError,
		usageError{errors.New("")}: exitUsage,
		store.ErrNoMe:              exitNoMe,
		store.ErrChangedMeanwhile:  exitConflict,
		store.ErrAmbiguous:         exitNotFound,
	} {
		if got := exitCode(err); got != want {
			t.Errorf("exitCode(%v) = %d, want %d", err, got, want)
		}
	}
	var buf strings.Builder
	if code := report(&buf, store.ErrNoMe); code != exitNoMe || !strings.Contains(buf.String(), "git board me --new") {
		t.Errorf("report(ErrNoMe) = %d, %q", code, buf.String())
	}
}
