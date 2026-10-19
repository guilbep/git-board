package board_test

import (
	"slices"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
	"github.com/guilbep/git-board/internal/inbox"
)

// Search as on GitHub: free text in titles, descriptions and comments;
// qualifiers on top.
func TestSearchLooksInCommentsAndBodies(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	inComment, _ := b.NewIssue("Train distances", "nothing here", []string{"assignee:guilbep"})
	if _, err := b.AddComment(inComment, "the zebra table is wrong"); err != nil {
		t.Fatal(err)
	}
	inBody, _ := b.NewIssue("Buildings", "a zebra in the description", nil)
	if _, err := b.NewIssue("Unrelated", "nothing", nil); err != nil {
		t.Fatal(err)
	}
	me, _, _ := b.Me()

	for expr, want := range map[string][]string{
		"zebra":                  {inComment, inBody},
		"zebra assignee:guilbep": {inComment},
		"zebra no:assignee":      {inBody},
		`"zebra table"`:          {inComment},
		"author:" + me.Name:      {inComment, inBody, ""},
	} {
		got, err := board.LoadMatching(b, &config.Config{}, expr, "")
		if err != nil {
			t.Fatalf("%q: %v", expr, err)
		}
		n := 0
		for _, w := range want {
			if w != "" {
				n++
			}
		}
		if expr == "author:"+me.Name {
			n = 3
		}
		if len(got) != n {
			t.Errorf("%q: %d issues, want %d", expr, len(got), n)
			continue
		}
		for _, w := range want {
			found := w == ""
			for _, i := range got {
				found = found || i.ID == w
			}
			if !found {
				t.Errorf("%q: %s missing", expr, w)
			}
		}
	}
}

// involves:, on a real repository: a person field, a mention found
// through the index, a comment by an identity carrying the login.
func TestInvolvesOnARepository(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	assigned, _ := b.NewIssue("Train legs", "", []string{"assignee:guilbep"})
	mentioned, _ := b.NewIssue("Buildings", "ping @guilbep", nil)
	commented, _ := b.NewIssue("Food", "", nil)
	other := gitbugiotest.NewPerson(t, b, "Guilbert (GitHub)", "guilbep")
	gitbugiotest.CommentAs(t, b, commented, other, time.Now().Unix(), "on it")
	// someone else's: an issue you opened involves you (author)
	stranger := gitbugiotest.NewPerson(t, b, "Ann", "ann")
	if _, _, err := b.TestCache().Bugs().NewRaw(stranger, time.Now().Unix(), "Mail", "guilbep.pierre@epfl.ch", nil, nil); err != nil {
		t.Fatal(err)
	}

	got, err := board.LoadMatching(b, &config.Config{}, "involves:@me", "guilbep")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, i := range got {
		ids[i.ID] = true
	}
	if !ids[assigned] || !ids[mentioned] || !ids[commented] || len(ids) != 3 {
		t.Errorf("involves:@me = %v, want %s, %s, %s", ids, assigned, mentioned, commented)
	}
}

// follows:@me reads your inbox file; a followed view counts without its
// is: (a close still shows) and without its own follows: (no loop).
func TestFollowsOnARepository(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	assigned, _ := b.NewIssue("Train legs", "", []string{"assignee:guilbep"})
	unfollowed, _ := b.NewIssue("Buildings", "", []string{"assignee:guilbep"})
	followed, _ := b.NewIssue("Food", "", nil)
	closedInView, _ := b.NewIssue("Cache factors", "", []string{"area:backend"})
	gitbugiotest.CloseIssue(t, b, closedInView)
	ann := gitbugiotest.NewPerson(t, b, "Ann", "ann") // someone else's: an issue you opened involves you
	if _, _, err := b.TestCache().Bugs().NewRaw(ann, time.Now().Unix(), "Unrelated", "", nil, nil); err != nil {
		t.Fatal(err)
	}
	me, _, _ := b.Me()
	if err := inbox.Save(b.Storage(), me.ID, inbox.State{Follow: []string{followed}, Unfollow: []string{unfollowed}, Views: []string{"backend", "gone"}}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Views: map[string]config.View{"backend": {Filter: "is:open area:backend follows:@me"}}}

	got, err := board.LoadMatching(b, cfg, "follows:@me", "guilbep")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, i := range got {
		ids[i.ID] = true
	}
	if !ids[assigned] || !ids[followed] || !ids[closedInView] || ids[unfollowed] || len(ids) != 3 {
		t.Errorf("follows:@me = %v, want %s, %s, %s", ids, assigned, followed, closedInView)
	}

	all, _ := b.ListIssues()
	inViews, missing, err := board.FollowedViews(b, cfg, []string{"backend", "gone"}, "guilbep", all, time.Now())
	if err != nil || !slices.Equal(inViews[closedInView], []string{"backend"}) || !slices.Equal(missing, []string{"gone"}) {
		t.Errorf("FollowedViews = %v %v %v", inViews, missing, err)
	}
}

// You are your identity too, not only your login: an issue you opened or
// commented on as an identity without a login (--me gives the login)
// involves you, as the inbox says.
func TestInvolvesYouByIdentity(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t) // writes as an identity without a login
	opened, _ := b.NewIssue("Mine", "", nil)
	ann := gitbugiotest.NewPerson(t, b, "Ann", "ann")
	hers, _, err := b.TestCache().Bugs().NewRaw(ann, time.Now().Unix(), "Hers", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	commented := hers.Id().String()
	if _, err := b.AddComment(commented, "mine"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.TestCache().Bugs().NewRaw(ann, time.Now().Unix(), "Not mine", "", nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, expr := range []string{"involves:@me", "follows:@me"} {
		got, err := board.LoadMatching(b, &config.Config{}, expr, "guilbep")
		if err != nil {
			t.Fatal(err)
		}
		ids := map[string]bool{}
		for _, i := range got {
			ids[i.ID] = true
		}
		if !ids[opened] || !ids[commented] || len(ids) != 2 {
			t.Errorf("%s = %v, want %s and %s", expr, ids, opened, commented)
		}
	}
}
