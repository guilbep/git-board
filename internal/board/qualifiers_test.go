package board_test

import (
	"slices"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store/storetest"
)

func unix(s string) int64 {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t.Unix()
}

func timed(op oplog.Op, s string) oplog.Op {
	op.Time = unix(s)
	return op
}

// created:, updated:, closed:, comments: and in: answer the same through
// the index (store.Backend's SearchText) as without it (the issues' own
// texts).
func TestQualifiersWithAndWithoutTheIndex(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	me, _, err := b.Me()
	if err != nil {
		t.Fatal(err)
	}
	crash := storetest.IssueAs(t, b, me, unix("2026-09-01T10:00:00Z"), "Crash on save", "the editor dies")
	for range 6 {
		storetest.CommentAs(t, b, crash, me, unix("2026-10-02T10:00:00Z"), "me too")
	}
	storetest.Do(t, b, crash, timed(issue.SetStatus(me.ID, issue.Closed), "2026-10-05T10:00:00Z"))
	slow := storetest.IssueAs(t, b, me, unix("2026-10-01T09:00:00Z"), "Slow startup", "a crash in the logs")
	storetest.CommentAs(t, b, slow, me, unix("2026-10-10T23:59:59Z"), "nothing")
	docs := storetest.IssueAs(t, b, me, unix("2026-10-14T08:00:00Z"), "Docs", "")
	storetest.CommentAs(t, b, docs, me, unix("2026-10-14T08:30:00Z"), "crash here")

	today := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatal(err)
	}
	for expr, want := range map[string][]string{
		"created:2026-10-01":                  {slow},
		"created:>2026-10-01":                 {docs},
		"created:>=2026-10-01":                {slow, docs},
		"created:<2026-10-01":                 {crash},
		"created:<=2026-10-01":                {crash, slow},
		"created:2026-09-15..2026-10-01":      {slow},
		"created:2026-10-02..*":               {docs},
		"created:*..2026-09-01":               {crash},
		"-created:2026-10-01":                 {crash, docs},
		"created:2026-10-01T09:00":            {slow},
		"created:2026-10-01T09:01":            nil,
		"created:2026-10-01T11:00:00+02:00":   {slow},
		"created:>=@today-7d":                 {docs},
		"created:@today-1d":                   {docs},
		"created:>@today-2w":                  {docs},
		"updated:2026-10-10":                  {slow},
		"updated:<2026-10-06":                 {crash},
		"closed:2026-10-05":                   {crash},
		"closed:>2026-01-01":                  {crash},
		"-closed:>2026-01-01":                 {slow, docs},
		"comments:6":                          {crash},
		"comments:>1":                         {crash},
		"comments:<=1":                        {slow, docs},
		"comments:1..5":                       {slow, docs},
		"comments:0":                          nil,
		"crash":                               {crash, slow, docs},
		"crash in:title":                      {crash},
		"crash in:body":                       {slow},
		"crash in:comments":                   {docs},
		"crash in:title,body":                 {crash, slow},
		"crash in:title in:comments":          {crash, docs},
		"crash -in:title":                     {slow, docs},
		"crash -in:title,body,comments":       nil,
		"in:title":                            {crash, slow, docs}, // no text: nothing to narrow
		`"the logs" in:body comments:1`:       {slow},
		"crash in:body created:>=@today-30d":  {slow},
		"save in:title closed:2026-10-01..*":  {crash},
		"logs in:body updated:>=2026-10-10":   {slow},
		"me in:comments comments:>=6 is:open": nil,
	} {
		for _, path := range []struct {
			name string
			b    any
		}{{"index", b}, {"no index", struct{}{}}} {
			got, err := board.Match(path.b, &config.Config{}, expr, "", slices.Clone(issues), today)
			if err != nil {
				t.Fatalf("%s, %q: %v", path.name, expr, err)
			}
			var ids []string
			for _, is := range got {
				ids = append(ids, is.ID)
			}
			if !slices.Equal(ids, want) {
				t.Errorf("%s, %q: %v, want %v", path.name, expr, ids, want)
			}
		}
	}

	for _, expr := range []string{"created:2026-1-1", "created:>", "updated:@today-7x", "closed:soon", "comments:many", "comments:-1", "in:labels", "in:title,lables"} {
		if _, err := board.Match(b, &config.Config{}, expr, "", issues, today); !board.IsFilterError(err) {
			t.Errorf("%q: %v, want a filter error", expr, err)
		}
	}
}
