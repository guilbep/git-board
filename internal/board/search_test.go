package board_test

import (
	"testing"

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
