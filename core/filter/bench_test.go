package filter

import (
	"fmt"
	"testing"

	"github.com/guilbep/git-board/core/model"
)

// benchIssues are n issues as a big board lists them: a status, a prio,
// a sprint, people, a few comments.
func benchIssues(n int) []model.Issue {
	statuses := []string{"backlog", "ready", "in-progress", "in-review", "qa"}
	issues := make([]model.Issue, n)
	for i := range issues {
		issues[i] = model.Issue{
			ID: fmt.Sprintf("%040x", i), Title: fmt.Sprintf("Fix the invoice export %d", i), Status: "open",
			Labels: []string{"status:" + statuses[i%len(statuses)], fmt.Sprintf("prio:p%d", i%4),
				"sprint:delivery", fmt.Sprintf("assignee:dev%d", i%7)},
			Body:          "The export crashes when the invoice has no lines.",
			CommentBodies: []string{"Seen on staging too", "@dev3 can you look?"},
			AuthorLogins:  []string{fmt.Sprintf("dev%d", i%5)},
			Comments:      2,
		}
		if i%3 == 0 {
			issues[i].Status = "closed"
		}
	}
	return issues
}

// Matches over a board of 5,000 issues: field terms, and free text
// without the index (an issue as it was, ls --at).
func BenchmarkMatches(b *testing.B) {
	cfg := co2
	issues := benchIssues(5000)
	for name, f := range map[string]string{
		"fields": "is:open status:ready,in-progress assignee:@me sprint:@current -label:wip",
		"text":   "is:open crash title:*invoice*",
	} {
		conds := Parse(f)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				n := 0
				for _, is := range issues {
					if Matches(is, conds, cfg, "dev3", today) {
						n++
					}
				}
				if n == 0 {
					b.Fatal("no match")
				}
			}
		})
	}
}
