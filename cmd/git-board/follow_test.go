package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/store/storetest"
)

// follow, unfollow and follow --auto are the web bell's three choices,
// and follows:@me sees them.
func TestFollowUnfollow(t *testing.T) {
	dir, b := seedRepo(t)
	ann := storetest.NewPerson(t, b, "Ann", "ann")
	theirs := storetest.IssueAs(t, b, ann, time.Now().Unix(), "Ann's issue", "")
	mine, err := b.NewIssue("Mine", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	following := func() []string {
		t.Helper()
		out, err := execute(t, []string{"ls", "follows:@me", "--json"})
		if err != nil {
			t.Fatalf("ls follows:@me: %v\n%s", err, out)
		}
		var issues []struct{ Title string }
		if err := json.Unmarshal([]byte(out), &issues); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		var res []string
		for _, is := range issues {
			res = append(res, is.Title)
		}
		slices.Sort(res)
		return res
	}
	if got := following(); !slices.Equal(got, []string{"Mine"}) {
		t.Fatalf("following at first = %v, want what you opened", got)
	}

	out, err := execute(t, []string{"follow", theirs[:8]})
	if err != nil || !strings.Contains(out, "following (followed)") {
		t.Errorf("follow: %v\n%s", err, out)
	}
	out, err = execute(t, []string{"unfollow", mine[:8]})
	if err != nil || !strings.HasSuffix(strings.TrimSpace(out), "not following (author), by choice") {
		t.Errorf("unfollow: %v\n%s", err, out)
	}
	if got := following(); !slices.Equal(got, []string{"Ann's issue"}) {
		t.Errorf("following = %v, want Ann's issue only", got)
	}

	out, err = execute(t, []string{"follow", mine[:8], "--auto", "--json"})
	var st followState
	if err != nil || json.Unmarshal([]byte(out), &st) != nil || !st.Following || st.Set != nil || !slices.Equal(st.Reasons, []string{"author"}) {
		t.Errorf("follow --auto --json: %v\n%s", err, out)
	}
	if out, err := execute(t, []string{"follow", mine[:8], "--auto", "--json", "following,set"}); err != nil || strings.TrimSpace(out) != "{\n  \"following\": true,\n  \"set\": null\n}" {
		t.Errorf("follow --auto --json following,set: %v\n%s", err, out)
	}
	if _, err := execute(t, []string{"follow", "0000000"}); exitCode(err) != exitNotFound {
		t.Errorf("follow 0000000 = %v, want exit %d", err, exitNotFound)
	}
}
