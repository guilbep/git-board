package filter_test

import (
	"slices"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/filter"
	"github.com/guilbep/git-board/internal/store"
)

func TestInvolvement(t *testing.T) {
	cfg := &config.Config{}
	in := filter.Involved{
		AuthorLogins:    []string{"Ann Example", "ann"},
		Labels:          []string{"assignee:guilbep", "reviewer:GUILBEP", "status:guilbep", "bug"},
		CommenterLogins: []string{"srossi", "guilbep"},
		Mentioned:       true,
	}
	// status isn't a person field: a value that happens to be a login
	if got, want := filter.Involvement(in, "guilbep", "", cfg), []string{"assignee", "reviewer", "commented", "mentioned"}; !slices.Equal(got, want) {
		t.Errorf("guilbep: %v, want %v", got, want)
	}
	if got := filter.Involvement(filter.Involved{AuthorLogins: in.AuthorLogins}, "ANN", "", cfg); !slices.Equal(got, []string{"author"}) {
		t.Errorf("ann: %v, want [author]", got)
	}
	nobody := in
	nobody.Mentioned = false // Mentioned is the asked login's: nothing mentions nobody
	if got := filter.Involvement(nobody, "nobody", "", cfg); got != nil {
		t.Errorf("nobody: %v, want none", got)
	}
	if got := filter.Involvement(in, "", "", cfg); got != nil {
		t.Errorf("no login: %v, want none", got)
	}
	if got := filter.Involvement(in, "guilbep", "", nil); len(got) == 0 {
		t.Error("a nil config still knows assignee by its name")
	}
}

func TestInvolvesTerm(t *testing.T) {
	cfg, now := &config.Config{}, time.Now()
	assigned := store.Issue{ID: "a1", Labels: []string{"assignee:guilbep"}}
	mentioned := store.Issue{ID: "b2"}
	other := store.Issue{ID: "c3", AuthorLogins: []string{"srossi"}}

	conds := filter.Parse("involves:@me")
	conds[0].Mentions = map[string]bool{"b2": true}
	for _, tc := range []struct {
		issue store.Issue
		want  bool
	}{{assigned, true}, {mentioned, true}, {other, false}} {
		if got := filter.Matches(tc.issue, conds, cfg, "guilbep", now); got != tc.want {
			t.Errorf("involves:@me on %s = %v, want %v", tc.issue.ID, got, tc.want)
		}
	}
	if !filter.Matches(other, filter.Parse("involves:srossi"), cfg, "guilbep", now) {
		t.Error("involves:srossi misses srossi's issue")
	}
	if filter.Matches(assigned, filter.Parse("-involves:@me"), cfg, "guilbep", now) {
		t.Error("-involves:@me keeps an issue you're assigned")
	}
	if !filter.RequiresMe(filter.Parse("involves:@me")) {
		t.Error("involves:@me needs to know who you are")
	}
}
func TestFollowsTerm(t *testing.T) {
	cfg, now := &config.Config{}, time.Now()
	assigned := store.Issue{ID: "a1", Labels: []string{"assignee:guilbep"}}
	unfollowed := store.Issue{ID: "b2", Labels: []string{"assignee:guilbep"}}
	followed := store.Issue{ID: "c3"}
	inView := store.Issue{ID: "d4"}
	nothing := store.Issue{ID: "e5"}

	conds := filter.Parse("follows:@me")
	conds[0].Followed = &filter.Followed{
		Follow:   map[string]bool{"c3": true},
		Unfollow: map[string]bool{"b2": true},
		InViews:  map[string]bool{"d4": true},
	}
	for _, tc := range []struct {
		issue store.Issue
		want  bool
	}{{assigned, true}, {unfollowed, false}, {followed, true}, {inView, true}, {nothing, false}} {
		if got := filter.Matches(tc.issue, conds, cfg, "guilbep", now); got != tc.want {
			t.Errorf("follows:@me on %s = %v, want %v", tc.issue.ID, got, tc.want)
		}
	}
	if filter.Matches(assigned, filter.Parse("follows:ann"), cfg, "guilbep", now) {
		t.Error("follows: is yours only: follows:ann matches nothing")
	}
	if !filter.RequiresMe(filter.Parse("follows:@me")) {
		t.Error("follows:@me needs to know who you are")
	}
}

// A role the issue takes from the project (internal/config's roles) isn't
// involvement: the project's lead-dev isn't in every issue's discussion.
// pm:@me still finds it, as a field.
func TestInvolvesLeavesOutProjectRoles(t *testing.T) {
	cfg, now := &config.Config{Fields: map[string]config.Field{"pm": {Type: config.TypePerson}}}, time.Now()
	inherited := store.Issue{ID: "a1", Inherited: []string{"pm:guilbep"}}
	if filter.Matches(inherited, filter.Parse("involves:@me"), cfg, "guilbep", now) {
		t.Error("involves:@me took a project role for involvement")
	}
	if !filter.Matches(inherited, filter.Parse("pm:@me"), cfg, "guilbep", now) {
		t.Error("pm:@me misses the project's pm")
	}
	own := store.Issue{ID: "b2", Labels: []string{"pm:guilbep"}}
	if !filter.Matches(own, filter.Parse("involves:@me"), cfg, "guilbep", now) {
		t.Error("involves:@me misses a pm set on the issue itself")
	}
}
