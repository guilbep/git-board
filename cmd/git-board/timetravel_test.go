package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/internal/store/storetest"
)

func local(s string) int64 {
	t, err := time.ParseInLocation("2006-01-02T15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t.Unix()
}

func atTime(op oplog.Op, s string) oplog.Op {
	op.Time = local(s)
	return op
}

// timeTravelRepo is a board with an issue that changed on 2026-10-05,
// and one created on 2026-10-04; the test chdirs into it.
func timeTravelRepo(t *testing.T) (login, later string) {
	t.Helper()
	dir, b := seedRepo(t)
	me, _, err := b.Me()
	if err != nil {
		t.Fatal(err)
	}
	login = storetest.IssueAs(t, b, me, local("2026-10-01T10:00"), "Login fails", "line one\nline two", "status:ready", "bug")
	first := storetest.CommentAs(t, b, login, me, local("2026-10-03T10:00"), "first look")
	later = storetest.IssueAs(t, b, me, local("2026-10-04T12:00"), "Later issue", "")
	is, err := issue.Load(b.Datastore(), login)
	if err != nil {
		t.Fatal(err)
	}
	storetest.Do(t, b, login,
		atTime(issue.SetTitle(me.ID, "Login fails on Safari"), "2026-10-05T10:00"),
		atTime(issue.EditText(me.ID, is.BodyOp, "line one\nline 2\nline three"), "2026-10-05T10:00"),
		atTime(issue.ChangeLabels(me.ID, []string{"prio:p1"}, []string{"bug"}), "2026-10-05T10:00"),
		atTime(issue.NewComment(me.ID, "fixed?"), "2026-10-05T10:00"),
		atTime(issue.EditText(me.ID, first, "first look, edited"), "2026-10-05T10:00"),
		atTime(issue.SetStatus(me.ID, issue.Closed), "2026-10-05T10:00"),
	)
	if err := os.WriteFile(filepath.Join(dir, configPath), []byte(testConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return login, later
}

func TestShowAndLsAt(t *testing.T) {
	login, later := timeTravelRepo(t)

	out, err := execute(t, []string{"show", login, "--at", "2026-10-02"})
	if err != nil {
		t.Fatalf("show --at: %v\n%s", err, out)
	}
	for _, want := range []string{"As it was on 2026-10-02 23:59", "Login fails\n", "open", "0 comments", "line two", "bug"} {
		if !strings.Contains(out, want) {
			t.Errorf("show --at lacks %q:\n%s", want, out)
		}
	}
	for _, not := range []string{"Safari", "first look", "prio"} {
		if strings.Contains(out, not) {
			t.Errorf("show --at shows %q, made later:\n%s", not, out)
		}
	}
	out, err = execute(t, []string{"show", login, "--at", "2026-10-04T00:00", "--json"})
	var th struct {
		Status string `json:"status"`
		Events []struct {
			Kind, Body string
		} `json:"events"`
	}
	if err != nil || json.Unmarshal([]byte(out), &th) != nil {
		t.Fatalf("show --at --json: %v\n%s", err, out)
	}
	if th.Status != "open" || len(th.Events) != 3 || th.Events[2].Body != "first look" {
		t.Errorf("show --at --json = %+v", th)
	}
	if _, err := execute(t, []string{"show", later, "--at", "2026-10-02"}); exitCode(err) != exitNotFound {
		t.Errorf("show of an issue not created yet = %v, want exit %d", err, exitNotFound)
	}
	if _, err := execute(t, []string{"show", login, "--at", "yesterday"}); exitCode(err) != exitUsage {
		t.Errorf("show --at yesterday = %v, want exit %d", err, exitUsage)
	}

	for _, tc := range []struct {
		filter, at string
		want       []string
	}{
		{"is:open", "2026-10-02", []string{login}},
		{"is:open", "2026-10-04", []string{login, later}},
		{"is:closed", "2026-10-06", []string{login}},
		{"label:bug", "2026-10-04", []string{login}},
		{"label:bug", "2026-10-06", nil},
		{"comments:1 created:<@today", "2026-10-04", []string{login}}, // @today: the day gone back to
		{"two in:body", "2026-10-02", []string{login}},
		{"two in:body", "2026-10-06", nil},
		{"look", "2026-10-04", []string{login}},
		{"", "2026-09-30", nil},
	} {
		out, err := execute(t, []string{"ls", tc.filter, "--at", tc.at, "--json"})
		if err != nil {
			t.Fatalf("ls %q --at %s: %v\n%s", tc.filter, tc.at, err, out)
		}
		var got []struct{ ID string }
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("ls --json: %v\n%s", err, out)
		}
		var ids []string
		for _, g := range got {
			ids = append(ids, g.ID)
		}
		if strings.Join(ids, " ") != strings.Join(tc.want, " ") {
			t.Errorf("ls %q --at %s = %v, want %v", tc.filter, tc.at, ids, tc.want)
		}
	}
}

func TestDiffSince(t *testing.T) {
	login, later := timeTravelRepo(t)

	out, err := execute(t, []string{"diff", login, "--since", "2026-10-04"})
	if err != nil {
		t.Fatalf("diff: %v\n%s", err, out)
	}
	for _, want := range []string{
		"Login fails on Safari, since 2026-10-04 00:00",
		`title: "Login fails" → "Login fails on Safari"`, "state: open → closed", "labels: +prio:p1 -bug",
		"  line one\n- line two\n+ line 2\n+ line three\n", "+++ guilbep", "fixed?",
		"~~~ edited: guilbep", "- first look\n+ first look, edited\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("diff lacks %q:\n%s", want, out)
		}
	}

	out, err = execute(t, []string{"diff", login, "--since", "2026-10-04", "--json"})
	var d issueDiff
	if err != nil || json.Unmarshal([]byte(out), &d) != nil {
		t.Fatalf("diff --json: %v\n%s", err, out)
	}
	if d.ID != login || d.Created || d.Title == nil || d.Title.Was != "Login fails" || d.Status == nil || d.Status.Now != "closed" ||
		strings.Join(d.Added, ",") != "prio:p1" || strings.Join(d.Removed, ",") != "bug" || d.Description == nil ||
		d.Description.Was != "line one\nline two" || len(d.Comments) != 1 || d.Comments[0].Body != "fixed?" ||
		len(d.EditedComments) != 1 || d.EditedComments[0].Was != "first look" {
		t.Errorf("diff --json = %s", out)
	}

	if out, _ := execute(t, []string{"diff", login, "--since", "2026-10-06"}); !strings.Contains(out, "No change.") {
		t.Errorf("diff since after the last change:\n%s", out)
	}
	out, err = execute(t, []string{"diff", later, "--since", "2026-10-01", "--json"})
	if err != nil || json.Unmarshal([]byte(out), &d) != nil || !d.Created || d.Title == nil || d.Title.Now != "Later issue" {
		t.Errorf("diff of an issue created since = %s, %v", out, err)
	}
	for _, args := range [][]string{{"diff", login}, {"diff", login, "--since", "soon"}} {
		if _, err := execute(t, args); exitCode(err) != exitUsage {
			t.Errorf("%v = %v, want exit %d", args, err, exitUsage)
		}
	}
}

func TestParseWhen(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	for s, want := range map[string][2]time.Time{
		"2026-10-01":           {day, day.AddDate(0, 0, 1)},
		"2026-10-01T14:30":     {day.Add(14*time.Hour + 30*time.Minute), day.Add(14*time.Hour + 31*time.Minute)},
		"2026-10-01 14:30:05":  {day.Add(14*time.Hour + 30*time.Minute + 5*time.Second), day.Add(14*time.Hour + 30*time.Minute + 6*time.Second)},
		"2026-10-01T12:00:00Z": {time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 12, 0, 1, 0, time.UTC)},
		"7d":                   {now.AddDate(0, 0, -7), now.AddDate(0, 0, -7).Add(time.Second)},
		"2w":                   {now.AddDate(0, 0, -14), now.AddDate(0, 0, -14).Add(time.Second)},
		"36h":                  {now.Add(-36 * time.Hour), now.Add(-36 * time.Hour).Add(time.Second)},
	} {
		from, until, err := parseWhen(s, now)
		if err != nil || !from.Equal(want[0]) || !until.Equal(want[1]) {
			t.Errorf("parseWhen(%q) = %v, %v, %v; want %v", s, from, until, err, want)
		}
	}
	if at, _ := parseAt("2026-10-01", now); !at.Equal(day.AddDate(0, 0, 1).Add(-time.Second)) {
		t.Errorf("parseAt(2026-10-01) = %v, want the day's last second", at)
	}
	for _, s := range []string{"", "yesterday", "-1h", "2026-10", "7x"} {
		if _, _, err := parseWhen(s, now); err == nil {
			t.Errorf("parseWhen(%q): no error", s)
		}
	}
}

// lineDiff keeps, removes and adds lines as the longest common run says.
func TestLineDiff(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ a, b, want string }{
		{"", "", ""},
		{"", "new", "+ new\n"},
		{"old\n", "", "- old\n"},
		{"a\nb\nc", "a\nc\nd", "  a\n- b\n  c\n+ d\n"},
		{"x", "y", "- x\n+ y\n"},
	} {
		if got := lineDiff(tc.a, tc.b); got != tc.want {
			t.Errorf("lineDiff(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}
