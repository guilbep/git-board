package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/store/storetest"
)

// status is what needs you: assigned, then the inbox split into what you
// opened and the rest; --mark-read empties the inbox, not the assigned.
func TestStatusIsWhatNeedsYou(t *testing.T) {
	dir, b := seedRepo(t)
	ann := storetest.NewPerson(t, b, "Ann", "ann")
	hour := time.Now().Add(-time.Hour).Unix()
	storetest.IssueAs(t, b, ann, hour, "Yours to do", "", "assignee:guilbep")
	mentioning := storetest.IssueAs(t, b, ann, time.Now().Add(-48*time.Hour).Unix(), "Ann's question", "")
	storetest.CommentAs(t, b, mentioning, ann, hour, "What do you think, @guilbep?")
	storetest.IssueAs(t, b, ann, hour, "Not about you", "")
	mine, err := b.NewIssue("My report", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	storetest.CommentAs(t, b, mine, ann, time.Now().Unix(), "Reproduced.")
	t.Chdir(dir)

	out, err := execute(t, []string{"status"})
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}
	assigned, rest, _ := strings.Cut(out, "New in what you opened")
	opened, follows, _ := strings.Cut(rest, "New in what you follow")
	if !strings.Contains(assigned, "Yours to do") || strings.Contains(assigned, "My report") {
		t.Errorf("assigned part:\n%s", assigned)
	}
	if !strings.Contains(opened, "My report") || !strings.Contains(opened, "1 comment, by Ann") {
		t.Errorf("opened part:\n%s", opened)
	}
	if !strings.Contains(follows, "Ann's question") || !strings.Contains(follows, "mentioned you") || !strings.Contains(follows, "Yours to do") {
		t.Errorf("follows part:\n%s", follows)
	}
	if strings.Contains(out, "Not about you") {
		t.Errorf("an issue you're not in:\n%s", out)
	}

	out, err = execute(t, []string{"status", "--json", "--mark-read"})
	var rep statusReport
	if err != nil || json.Unmarshal([]byte(out), &rep) != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	if len(rep.Assigned) != 1 || len(rep.Opened) != 1 || len(rep.Follows) != 2 || rep.Me.Login != "guilbep" {
		t.Errorf("status --json = %d assigned, %d opened, %d follows, me %+v", len(rep.Assigned), len(rep.Opened), len(rep.Follows), rep.Me)
	}

	out, err = execute(t, []string{"status", "--json"})
	rep = statusReport{}
	if err != nil || json.Unmarshal([]byte(out), &rep) != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	if len(rep.Assigned) != 1 || len(rep.Opened)+len(rep.Follows) != 0 {
		t.Errorf("after --mark-read: %s", out)
	}
	if !strings.Contains(out, `"opened": []`) {
		t.Errorf("an empty part is [], not null:\n%s", out)
	}
}
