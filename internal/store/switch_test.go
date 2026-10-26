package store_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

func forkID(c byte) string { return strings.Repeat(string(c), 64) }

// switchSetup is a repository the fork kept: a numbered issue on GitHub,
// mirrored on GitLab as a copy with the fork's marker only; a numbered
// one on GitHub alone, with a comment the fork never posted; an issue on
// no forge, numbered; one on a forge the settings don't name; a release
// record. Its forges are fakes.
func switchSetup(t *testing.T) (*store.Backend, store.SwitchRun, []*fake.Forge) {
	t.Helper()
	b := storetest.New(t)
	gh, gl := fake.New("github.com/o/r"), fake.New("gitlab.example.com/7")
	alice, bob, board := forge.Account{ID: "7", Login: "alice"}, forge.Account{ID: "8", Login: "bob"}, forge.Account{ID: "1", Login: "board"}
	gh.Add(forge.Issue{Author: alice, Title: "One", Body: "one", Comments: []forge.Comment{{Body: "c1", Author: bob}}})
	gh.Add(forge.Issue{Author: alice, Title: "Two", Body: "two", Comments: []forge.Comment{{Body: "c2", Author: bob}}})
	gl.Add(forge.Issue{Author: board, Title: "One", Body: "<!-- git-bug:" + forkID('d') + " -->\none",
		Comments: []forge.Comment{{Body: "<!-- git-bug:" + forkID('e') + " -->\nc1", Author: board}}})
	fork := &store.Fork{
		Bugs: []store.ForkBug{
			{ID: forkID('a'), Title: "One", Metadata: map[string]string{
				"github-url":                         "https://github.com/o/r/issues/1",
				"sync:gitlab.example.com/7:number":   "1",
				"sync:gitlab.example.com/7:url":      "https://gitlab.example.com/grp/r/-/work_items/1",
				"sync:github.com/o/r:number":         "1",
				"sync:github.com/O/R:labels":         "",
				"gitlab-base-url":                    "",
				"origin":                             "github",
				"sync:gitlab.example.com/7:assignee": "",
			}},
			{ID: forkID('b'), Title: "Two", Metadata: map[string]string{"github-url": "https://github.com/o/r/issues/2",
				"sync:gitlab.example.com/7:number": "5"}}, // a GitLab issue the forge doesn't list
			{ID: forkID('c'), Title: "Local"},
			{ID: forkID('f'), Title: "Elsewhere", Metadata: map[string]string{"github-url": "https://github.com/x/y/issues/9"}},
		},
		Users: []store.ForkUser{{ID: forkID('9'), Name: "Alice Example", Login: "alice"}},
		Texts: func(id string) ([]string, error) {
			return map[string][]string{forkID('a'): {"one", "c1"}, forkID('b'): {"two", "c2", "never posted\nsecond line"}}[id], nil
		},
	}
	for _, id := range []string{forkID('a'), forkID('b'), forkID('c'), forkID('f')} {
		c, err := store.StoreCommit(b, id)
		if err != nil {
			t.Fatal(err)
		}
		if err := b.Store().UpdateRef(t.Context(), "refs/bugs/"+id, c, gitstore.ZeroHash); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.WriteCounter(b, []byte("prefix GB\nstart 5\n5 "+forkID('a')+"\n6 "+forkID('b')+"\n7 "+forkID('c')+"\n")); err != nil {
		t.Fatal(err)
	}
	author := `{"author":{"id":"` + forkID('9') + `"},"ops":`
	if err := store.WriteForkRelease(b, forkID('r'),
		author+`[{"type":1,"timestamp":100,"nonce":"x","commit":"abc123","previous":"v0.1.0","artifacts":null,"chart":{"version":"","digest":""},"assets":[{"name":"a.tgz","sha256":"00"}]}]}`,
		author+`[{"type":2,"timestamp":200,"nonce":"y","notes":"### feat\n\n- GB-5 one"}]}`); err != nil {
		t.Fatal(err)
	}
	r := store.SwitchRun{Fork: fork}
	for _, f := range []*fake.Forge{gh, gl} {
		name, _, _ := strings.Cut(f.Key(), ".")
		r.Forges = append(r.Forges, store.NamedForge{Name: name, Forge: f})
	}
	return b, r, []*fake.Forge{gh, gl}
}

func noWrites(t *testing.T, fs []*fake.Forge) {
	t.Helper()
	for _, f := range fs {
		if w := f.Writes(); len(w) > 0 {
			t.Fatalf("%s got writes: %+v", f.Key(), w)
		}
	}
}

// The dry run reports and writes nothing here; --apply refuses its
// blockers, and with --force writes the issues, the numbers and the
// release, once. No forge is ever written to.
func TestSwitch(t *testing.T) {
	t.Parallel()
	b, run, fs := switchSetup(t)
	before, _ := b.Store().Refs("refs/")

	rep, err := b.Switch(t.Context(), run)
	if err != nil {
		t.Fatal(err)
	}
	noWrites(t, fs)
	if after, _ := b.Store().Refs("refs/"); len(after) != len(before) {
		t.Fatalf("the dry run wrote refs: %d, then %d", len(before), len(after))
	}
	if rep.Forges["github"] != 2 || rep.Forges["gitlab"] != 1 || rep.Joined != 1 || rep.OnSeveral != 1 || rep.Pulled.Created != 2 {
		t.Fatalf("report %+v", rep)
	}
	if rep.Numbers != 2 || !slices.Equal(rep.NoIssue, []string{"GB-7 (ccccccc)"}) {
		t.Fatalf("numbers %d, without an issue %v", rep.Numbers, rep.NoIssue)
	}
	if len(rep.Lost) != 2 || !strings.Contains(rep.Lost[0], "Local: on no forge") || !strings.Contains(rep.Lost[1], "on no forge of [forges]") {
		t.Fatalf("lost %q", rep.Lost)
	}
	if !slices.Equal(rep.LostComments, []string{"bbbbbbb: never posted"}) {
		t.Fatalf("lost comments %q", rep.LostComments)
	}
	if rep.Releases != 1 || rep.Carried != 1 || len(rep.ReleaseNotes) != 0 {
		t.Fatalf("releases %d, carried %d, %v", rep.Releases, rep.Carried, rep.ReleaseNotes)
	}
	// Two isn't on GitLab: the next sync would create it there, and its comment
	want := []string{`gitlab: issue for github#2 "Two": git-bug links it to gitlab#5, which the forge didn't list`,
		`gitlab: a comment on the issue it creates for github#2 "Two"`}
	if !slices.Equal(rep.WouldCreate, want) || len(rep.Writes) != 0 || len(rep.Blockers) != 2 {
		t.Fatalf("would create %q, writes %q, blockers %q", rep.WouldCreate, rep.Writes, rep.Blockers)
	}

	run.Apply = true
	if rep, err = b.Switch(t.Context(), run); err != nil || rep.Applied {
		t.Fatalf("applied over its blockers: %v, %v", rep.Applied, err)
	}
	// --allow-create lifts the creations' blocker alone: the lost still block
	run.AllowCreate = true
	if rep, err = b.Switch(t.Context(), run); err != nil || rep.Applied || len(rep.Blockers) != 1 || !strings.Contains(rep.Blockers[0], "aren't on the forges") {
		t.Fatalf("allow-create: applied %v, blockers %q, %v", rep.Applied, rep.Blockers, err)
	}
	run.AllowCreate, run.Force = false, true
	for range 2 { // twice: the second adds nothing
		if rep, err = b.Switch(t.Context(), run); err != nil || !rep.Applied {
			t.Fatalf("not applied: %v", err)
		}
		issues, err := b.ListIssues()
		if err != nil || len(issues) != 2 {
			t.Fatalf("%d issues, %v", len(issues), err)
		}
		c, ok, err := b.Counter()
		if err != nil || !ok {
			t.Fatal(ok, err)
		}
		for _, is := range issues {
			want := map[string]int{"One": 5, "Two": 6}[is.Title]
			if c.ByID[is.ID] != want || c.ByID[forkID('c')] != 7 {
				t.Fatalf("%s numbered %d, want %d; counter %v", is.Title, c.ByID[is.ID], want, c.ByID)
			}
		}
		rs, err := b.Releases()
		if err != nil || len(rs) != 1 || rs[0].Commit != "abc123" || rs[0].Notes != "### feat\n\n- GB-5 one" || rs[0].Author != "alice" ||
			len(rs[0].Assets) != 1 {
			t.Fatalf("releases %+v, %v", rs, err)
		}
		if refs, _ := b.Store().Refs("refs/bugs/"); len(refs) != 4 {
			t.Fatalf("refs/bugs: %v", refs)
		}
	}
	noWrites(t, fs)
}

// A forge sync refuses a repository still on the fork, until its switch.
func TestForgeSyncWaitsForTheSwitch(t *testing.T) {
	t.Parallel()
	b, run, _ := switchSetup(t)
	if _, err := b.GetForges(t.Context(), store.ForgeRun{Forges: run.Forges}); !errors.Is(err, store.ErrNotSwitched) {
		t.Fatalf("err = %v, want ErrNotSwitched", err)
	}
	run.Apply, run.Force = true, true
	if _, err := b.Switch(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	if _, err := b.GetForges(t.Context(), store.ForgeRun{Forges: run.Forges}); err != nil {
		t.Fatal(err)
	}
}

// With no forge, the switch has nothing to pull: an error naming both
// fixes, which --force doesn't lift.
func TestSwitchWithoutForges(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	rep, err := b.Switch(t.Context(), store.SwitchRun{Apply: true, Force: true})
	if !errors.Is(err, store.ErrNoForge) || rep.Applied || !strings.Contains(err.Error(), "git remote on GitHub or GitLab") ||
		!strings.Contains(err.Error(), "[forges.<name>] table with sync = true") {
		t.Fatalf("%+v, %v", rep, err)
	}
}

// --apply keys origin's numbers again, not this clone's: here, numbers an
// earlier --apply left unpushed. Until the push, a fetch keeps them.
func TestSwitchKeysOriginsNumbers(t *testing.T) {
	t.Parallel()
	b, run, _ := switchSetup(t)
	dir := b.Store().CommonDir()
	storetest.Git(t, dir, "remote", "add", "origin", storetest.Repo(t, true))
	storetest.Git(t, dir, "push", "-q", "origin", "refs/board/seq")
	if err := store.WriteCounter(b, []byte("prefix GB\nstart 5\n5 "+forkID('a')+"\n6 "+forkID('b')+"\n7 "+forkID('c')+"\n8 "+forkID('z')+"\n")); err != nil {
		t.Fatal(err)
	}
	run.Apply, run.Force = true, true
	if rep, err := b.Switch(t.Context(), run); err != nil || !rep.Applied || rep.Numbers != 2 {
		t.Fatalf("%+v, %v", rep, err)
	}
	c, _, err := b.Counter()
	if err != nil || len(c.ByID) != 3 || c.ByID[forkID('z')] != 0 || c.ByID[forkID('c')] != 7 {
		t.Fatalf("counter %v, %v", c.ByID, err)
	}
	if err := b.FetchNumbers("origin"); !errors.Is(err, store.ErrNumbersAhead) {
		t.Fatalf("err = %v, want ErrNumbersAhead", err)
	}
	storetest.Git(t, dir, "push", "-q", "origin", "refs/board/seq")
	if err := b.FetchNumbers("origin"); err != nil {
		t.Fatal(err)
	}
}

// Every comment no forge has is listed, a review comment among them,
// though a comment only the forge has brings the counts level.
func TestSwitchFindsEveryLostComment(t *testing.T) {
	t.Parallel()
	b, run, fs := switchSetup(t)
	fs[0].Edit("1", func(i *forge.Issue) {
		i.Comments = append(i.Comments, forge.Comment{Body: "later, on GitHub", Author: forge.Account{ID: "8", Login: "bob"}})
	})
	run.Fork.Texts = func(id string) ([]string, error) {
		if id != forkID('a') {
			return nil, nil
		}
		return []string{"one", "c1", "a line note"}, nil
	}
	rep, err := b.Switch(t.Context(), run)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rep.LostComments, []string{"aaaaaaa: a line note"}) {
		t.Errorf("lost comments %q", rep.LostComments)
	}
}

// A GitLab URL names its project by path, the settings by id: it goes to
// the forge whose project's path it is, unless the fork links the issue
// there by key; a forge whose path isn't known leaves it unplaced.
func TestPlacesByPath(t *testing.T) {
	t.Parallel()
	gl := []store.NamedForge{{Name: "gitlab", Forge: fake.New("gitlab.example.com/7")}}
	paths := map[string]string{"gitlab.example.com/7": "grp/r"}
	for _, c := range []struct {
		meta     map[string]string
		paths    map[string]string
		want     []string
		unplaced bool
	}{
		{map[string]string{"gitlab-url": "https://gitlab.example.com/grp/r/-/issues/5"}, paths, []string{"gitlab.example.com/7#5"}, false},
		{map[string]string{"gitlab-url": "https://gitlab.example.com/other/proj/-/issues/5"}, paths, nil, false},
		{map[string]string{"gitlab-url": "https://gitlab.example.com/grp/r/-/issues/5", "sync:gitlab.example.com/7:number": "3"}, paths,
			[]string{"gitlab.example.com/7#3"}, false},
		{map[string]string{"gitlab-url": "https://gitlab.example.com/grp/r/-/issues/5"}, nil, nil, true},
	} {
		got, linked, unplaced := store.Places(store.ForkBug{Metadata: c.meta}, gl, c.paths)
		if !slices.Equal(got, c.want) || !linked || (unplaced != "") != c.unplaced {
			t.Errorf("%v: %v, %v, %q", c.meta, got, linked, unplaced)
		}
	}
}

// The switch writes its mark, refs/board/switched, which a forge sync
// waits for: a sync entity, as a trial left one, isn't it. The mark goes
// out with the entities, and switches the other clones.
func TestSwitchMarkTravels(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	for _, b := range []*store.Backend{mine, theirs} {
		c, err := store.StoreCommit(b, "bug")
		if err != nil {
			t.Fatal(err)
		}
		if err := b.Store().UpdateRef(t.Context(), "refs/bugs/"+forkID('a'), c, gitstore.ZeroHash); err != nil {
			t.Fatal(err)
		}
	}
	trial, _ := store.StoreCommit(mine, "trial")
	if err := mine.Store().UpdateRef(t.Context(), "refs/board/sync/"+forkID('t'), trial, gitstore.ZeroHash); err != nil {
		t.Fatal(err)
	}
	if err := mine.Switched(); !errors.Is(err, store.ErrNotSwitched) {
		t.Fatalf("a trial's sync entity: %v, want ErrNotSwitched", err)
	}
	run := store.SwitchRun{Forges: []store.NamedForge{{Name: "gh", Forge: fake.New("github.com/o/r")}},
		Fork: &store.Fork{Texts: func(string) ([]string, error) { return nil, nil }}, Apply: true, Force: true}
	if rep, err := mine.Switch(t.Context(), run); err != nil || !rep.Applied {
		t.Fatalf("%+v, %v", rep, err)
	}
	if err := mine.Switched(); err != nil {
		t.Fatal(err)
	}
	pushed(t, mine)
	if err := theirs.Switched(); !errors.Is(err, store.ErrNotSwitched) {
		t.Fatalf("before a pull: %v", err)
	}
	if err := theirs.Pull("origin"); err != nil {
		t.Fatal(err)
	}
	if err := theirs.Switched(); err != nil {
		t.Fatalf("after a pull: %v", err)
	}
}
