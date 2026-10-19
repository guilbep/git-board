package gitbugio_test

import (
	"slices"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

func TestMentions(t *testing.T) {
	t.Parallel()
	for text, want := range map[string]bool{
		"thanks @guilbep":             true,
		"thanks @GuilBep.":            true,
		"(@guilbep)":                  true,
		"mail guilbep.pierre@epfl.ch": false, // an email
		"@guilbep-bot did it":         false, // another login
		"see `@guilbep`":              false, // code
		"a/b/@guilbep":                false, // a path
		"no one":                      false,
	} {
		if got := gitbugio.Mentions(text, "guilbep"); got != want {
			t.Errorf("Mentions(%q) = %v, want %v", text, got, want)
		}
	}
	if gitbugio.Mentions("@x", "") {
		t.Error("an empty login is mentioned by nothing")
	}
}

// The index finds the login anywhere (an email, @guilbep-bot); the
// pattern keeps the real mentions.
func TestMentioningConfirmsWhatTheIndexFinds(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	inBody, _ := b.NewIssue("Train legs", "ping @guilbep", nil)
	inComment, _ := b.NewIssue("Buildings", "nothing", nil)
	if _, err := b.AddComment(inComment, "what do you think @guilbep?"); err != nil {
		t.Fatal(err)
	}
	email, _ := b.NewIssue("Mail", "write to guilbep.pierre@epfl.ch or @guilbep-bot", nil)

	got, err := b.Mentioning("guilbep")
	if err != nil {
		t.Fatal(err)
	}
	if !got[inBody] || !got[inComment] || got[email] || len(got) != 2 {
		t.Errorf("Mentioning = %v, want %s and %s only", got, inBody, inComment)
	}
}

func TestCommenterLoginsLeaveOutTheAuthor(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	id, _ := b.NewIssue("Train legs", "", nil)
	if _, err := b.AddComment(id, "mine"); err != nil {
		t.Fatal(err)
	}
	ann := gitbugiotest.NewPerson(t, b, "Ann", "ann")
	gitbugiotest.CommentAs(t, b, id, ann, time.Now().Unix(), "hers")

	issue, err := b.GetIssue(id)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(issue.CommenterLogins, []string{"ann"}) {
		t.Errorf("CommenterLogins = %v, want [ann]", issue.CommenterLogins)
	}
}
