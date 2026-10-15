package insights

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// ReportIssue is an issue as the sprint report lists it, as it is now.
type ReportIssue struct {
	ID, Title   string
	Number, URL string // on its first forge; empty when it isn't on one
	Assignees   []string
	Status      string // the status field's value
	Open        bool
}

// report sections, in the order a reader scans them
var sections = []string{"Done", "In progress", "In review", "Blocked or waiting", "Not started"}

// section sorts a status by its words, as the page picks status icons
// (web/src/valueicons.ts); a closed issue is done.
func section(status string, open bool) string {
	if !open {
		return "Done"
	}
	words := strings.FieldsFunc(strings.ToLower(status), func(r rune) bool { return r == '-' || r == '_' || r == ' ' || r == '/' })
	has := func(list ...string) bool {
		for _, w := range words {
			for _, l := range list {
				if w == l {
					return true
				}
			}
		}
		return false
	}
	switch {
	case has("blocked", "blocker", "stuck", "waiting", "wait", "hold", "pending", "comite", "comité", "committee", "triage", "paused"):
		return "Blocked or waiting"
	case has("done", "validated", "closed", "shipped", "merged", "complete", "completed", "resolved", "released"):
		return "Done"
	case has("review", "qa", "validation", "approval", "test", "tests", "testing"):
		return "In review"
	case has("progress", "doing", "wip", "active", "started", "ongoing"):
		return "In progress"
	}
	return "Not started"
}

// SprintReport is the sprint as Markdown to paste in Slack, an email or an
// issue: its dates, how much is done and how much time is left, the
// Insights findings, the issues by where they stand, and those added after
// the sprint started (from history, when their sprint label came).
func SprintReport(sp Sprint, issues []ReportIssue, history []Issue, findings []string, now time.Time) string {
	var b strings.Builder
	last := sp.End.AddDate(0, 0, -1) // End is the next sprint's start
	fmt.Fprintf(&b, "# %s (%s – %s)\n\n", sp.Title, sp.Start.Format("2 Jan"), last.Format("2 Jan 2006"))

	by := map[string][]ReportIssue{}
	for _, is := range issues {
		s := section(is.Status, is.Open)
		by[s] = append(by[s], is)
	}
	late := addedLate(sp, issues, history)
	done := len(by["Done"])
	pct := 0
	if len(issues) > 0 {
		pct = int(math.Round(100 * float64(done) / float64(len(issues))))
	}
	when := fmt.Sprintf("ended %s", last.Format("2 Jan"))
	if now.Before(sp.End) {
		days := int(math.Ceil(sp.End.Sub(now).Hours() / 24))
		when = fmt.Sprintf("%d day%s left", days, plural(days))
	}
	fmt.Fprintf(&b, "**%d of %d done (%d%%)** · %s · %d added after the start\n", done, len(issues), pct, when, len(late))
	if len(findings) > 0 {
		b.WriteString("\n")
		for _, f := range findings {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}
	for _, s := range sections {
		if len(by[s]) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s (%d)\n\n", s, len(by[s]))
		for _, is := range by[s] {
			fmt.Fprintf(&b, "- %s\n", line(is))
		}
	}
	if len(late) > 0 {
		fmt.Fprintf(&b, "\n## Added after the start (%d)\n\n", len(late))
		for _, l := range late {
			fmt.Fprintf(&b, "- %s, on %s\n", line(l.issue), l.at.Format("2 Jan"))
		}
	}
	return b.String()
}

// line is one issue: its forge number linked, else its git-bug id; its
// title; its assignees.
func line(is ReportIssue) string {
	ref := "`" + is.ID[:min(7, len(is.ID))] + "`"
	if is.Number != "" && is.URL != "" {
		ref = fmt.Sprintf("[#%s](%s)", is.Number, is.URL)
	}
	s := ref + " " + escapeMD(is.Title)
	if len(is.Assignees) > 0 {
		s += " (@" + strings.Join(is.Assignees, ", @") + ")"
	}
	return s
}

// escapeMD keeps a title literal: [BUG](x) isn't a link, *x* isn't bold.
var mdEscaper = strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`, `*`, `\*`, `_`, `\_`, "`", "\\`", `<`, `\<`, `>`, `\>`)

func escapeMD(s string) string { return mdEscaper.Replace(s) }

type lateIssue struct {
	issue ReportIssue
	at    time.Time
}

// addedLate are the sprint's issues whose sprint label came after the
// sprint's first day: what was added to it once it ran.
func addedLate(sp Sprint, issues []ReportIssue, history []Issue) []lateIssue {
	label := sp.Field + ":" + sp.Slug
	firstDay := truncDay(sp.Start).AddDate(0, 0, 1)
	added := map[string]time.Time{}
	for _, h := range history {
		for _, c := range h.Changes {
			for _, l := range c.Added {
				if l == label {
					added[h.ID] = c.At // the last time it came: after a removal, that one
				}
			}
		}
	}
	var res []lateIssue
	for _, is := range issues {
		if at, ok := added[is.ID]; ok && !at.Before(firstDay) {
			res = append(res, lateIssue{is, at})
		}
	}
	return res
}
