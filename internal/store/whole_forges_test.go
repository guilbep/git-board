package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// wholeWith runs the whole sync with these forges, with consent, and
// counts the issues it created, or would create, on them; and the forges
// left out.
func wholeWith(t *testing.T, b *store.Backend, dry bool, fs ...store.NamedForge) (posted int, left []string, err error) {
	t.Helper()
	w := store.Whole{Remote: "origin", DryRun: dry, Forges: func(ctx context.Context) error {
		_, err := b.SyncForges(ctx, store.ForgeRun{Forges: fs, DryRun: dry, AllowCreate: true, Merge: "origin",
			See: func(e store.SyncEvent) {
				switch e.What {
				case "posted":
					posted++
				case "left":
					left = append(left, e.Forge)
				}
			}})
		return err
	}}
	return posted, left, b.SyncWhole(t.Context(), w)
}

// The whole check counts what its pull brings in: a colleague's issues,
// on origin, are counted as the run then creates them.
func TestWholeCheckCountsThePull(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	for _, title := range []string{"a", "b", "c"} {
		storetest.NewIssue(t, theirs, title)
	}
	if _, err := theirs.Push("origin"); err != nil {
		t.Fatal(err)
	}
	f := fake.New("github.com/o/r")
	gh := store.NamedForge{Name: "github", Forge: f}
	if n, _, err := wholeWith(t, mine, true, gh); err != nil || n != 3 {
		t.Fatalf("check: would create %d, want 3 (%v)", n, err)
	}
	if n, _, err := wholeWith(t, mine, false, gh); err != nil || n != 3 || len(f.All()) != 3 {
		t.Fatalf("run: created %d, forge holds %d, want 3 (%v)", n, len(f.All()), err)
	}
}

type unreadable struct{ *fake.Forge }

func (unreadable) Visibility(context.Context) (forge.Visibility, error) {
	return "", errors.New("gitlab.example.com is down")
}

type issuesOff struct{ *fake.Forge }

func (issuesOff) Issues(context.Context, int64) ([]forge.Issue, error) {
	return nil, errors.New("410 Issues are disabled for this repo: " + forge.ErrNotFound.Error())
}

// A forge that can't be read, its visibility or its listing, is left out
// of the run and told: the other forges sync, and the push runs.
func TestWholeLeavesOutAnUnreadableForge(t *testing.T) {
	t.Parallel()
	for name, bad := range map[string]forge.Forge{
		"down":       unreadable{fake.New("gitlab.example.com/7")},
		"issues off": issuesOff{fake.New("github.com/me/fork")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mine, theirs := storetest.NewPair(t)
			storetest.NewIssue(t, mine, "local")
			gh := fake.New("github.com/o/r")
			_, left, err := wholeWith(t, mine, false, store.NamedForge{Name: "bad", Forge: bad}, store.NamedForge{Name: "github", Forge: gh})
			if err != nil || strings.Join(left, ",") != "bad" || len(gh.All()) != 1 {
				t.Fatalf("err %v, left %v, github holds %d", err, left, len(gh.All()))
			}
			if err := theirs.Pull("origin"); err != nil {
				t.Fatal(err)
			}
			if is, _ := theirs.ListIssues(); len(is) != 1 {
				t.Fatalf("origin got %d issues, want 1", len(is))
			}
		})
	}
}
