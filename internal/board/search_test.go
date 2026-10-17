package board_test

import (
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// Search as on GitHub: free text in titles, descriptions and comments;
// qualifiers on top.
func TestSearchLooksInCommentsAndBodies(t *testing.T) {
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
	b := gitbugiotest.New(t)
	assigned, _ := b.NewIssue("Train legs", "", []string{"assignee:guilbep"})
	mentioned, _ := b.NewIssue("Buildings", "ping @guilbep", nil)
	commented, _ := b.NewIssue("Food", "", nil)
	other := gitbugiotest.NewPerson(t, b, "Guilbert (GitHub)", "guilbep")
	gitbugiotest.CommentAs(t, b, commented, other, time.Now().Unix(), "on it")
	if _, err := b.NewIssue("Mail", "guilbep.pierre@epfl.ch", nil); err != nil {
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
