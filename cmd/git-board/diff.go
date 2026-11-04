package main

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/store"
)

// The board as it was (#88): show --at, ls --at, diff --since. An
// operation counts at its own time, its writer's clock, which the log
// keeps; the order the log applies them in is its clock's, so a fold at
// a time is the operations made by then, in that order (issue.At).

var whileAgo = regexp.MustCompile(`^(\d+)([dw])$`)

// parseWhen is a time as --at and --since take it, and the span it names:
// a date that day, a time (2026-10-01T14:30, or with seconds or a zone,
// RFC 3339) that minute or second, a while ago (36h, 7d, 2w) that
// instant. The span is [from, until).
func parseWhen(s string, now time.Time) (from, until time.Time, err error) {
	if t, err := time.ParseInLocation(time.DateOnly, s, time.Local); err == nil {
		return t, t.AddDate(0, 0, 1), nil
	}
	for _, f := range []struct {
		layout string
		d      time.Duration
	}{{"2006-01-02T15:04", time.Minute}, {"2006-01-02 15:04", time.Minute}, {"2006-01-02T15:04:05", time.Second}, {"2006-01-02 15:04:05", time.Second}} {
		if t, err := time.ParseInLocation(f.layout, s, time.Local); err == nil {
			return t, t.Add(f.d), nil
		}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, t.Add(time.Second), nil
	}
	if m := whileAgo.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		if m[2] == "w" {
			n *= 7
		}
		t := now.AddDate(0, 0, -n)
		return t, t.Add(time.Second), nil
	}
	if d, err := time.ParseDuration(s); err == nil && d >= 0 {
		t := now.Add(-d)
		return t, t.Add(time.Second), nil
	}
	return time.Time{}, time.Time{}, fmt.Errorf("%q: a date (2026-10-01), a time (2026-10-01T14:30) or a while ago (36h, 7d)", s)
}

// parseAt is --at: the last second of the span named, so a date is the
// board at the end of that day.
func parseAt(s string, now time.Time) (time.Time, error) {
	_, until, err := parseWhen(s, now)
	return until.Add(-time.Second), err
}

func newDiffCmd() *cobra.Command {
	var since string
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "diff <id> --since <when>",
		Short: "What changed on an issue since a date",
		Long: `What changed on an issue since a date: its title, its state, the
labels added and removed, its description (a line diff), the comments
written and the comments edited. The issue then is folded from the
operations made before --since, now from all of them; each operation
counts at its own time, its writer's clock.

--since is a date (from its start), a time (2026-10-01T14:30) or a while
ago (36h, 7d). --json prints {"id", "since", "created", "title",
"status", "added", "removed", "description", "comments",
"edited_comments"}: created is true when the issue is newer than
--since; title, status and description are {"was", "now"} when they
changed, description with its "diff".`,
		Example: `  git board diff a1b2c3d --since 2026-10-01
  git board diff CO2-3075 --since 7d
  git board diff a1b2c3d --since 2026-10-01T09:00 --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if since == "" {
				return usageError{errors.New("--since: from when (2026-10-01, 7d)")}
			}
			from, _, err := parseWhen(since, time.Now())
			if err != nil {
				return usageError{fmt.Errorf("--since %w", err)}
			}
			return runDiff(cmd.OutOrStdout(), args[0], from, js)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "from when: a date (2026-10-01), a time (2026-10-01T14:30) or a while ago (36h, 7d)")
	js = addJSON(cmd, "render as JSON", jsonFields(issueDiff{}), true)
	return cmd
}

// change is a value that changed.
type change struct {
	Was  string `json:"was"`
	Now  string `json:"now"`
	Diff string `json:"diff,omitempty"` // a text's lines, - and +
}

// diffComment is a comment written or edited since.
type diffComment struct {
	ID     string       `json:"id"`
	Author store.Person `json:"author"`
	At     time.Time    `json:"at"`
	Was    string       `json:"was,omitempty"` // edited: the text then
	Body   string       `json:"body"`
}

// issueDiff is what diff prints.
type issueDiff struct {
	ID             string        `json:"id"`
	Since          time.Time     `json:"since"`
	Created        bool          `json:"created"`
	Title          *change       `json:"title,omitempty"`
	Status         *change       `json:"status,omitempty"`
	Added          []string      `json:"added"`
	Removed        []string      `json:"removed"`
	Description    *change       `json:"description,omitempty"`
	Comments       []diffComment `json:"comments"`
	EditedComments []diffComment `json:"edited_comments"`

	shown, number string
}

func runDiff(out io.Writer, idPrefix string, since time.Time, js *jsonOut) error {
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()
	now, err := b.Thread(idPrefix)
	if err != nil {
		return err
	}
	then, err := b.ThreadAt(now.ID, since.Add(-time.Second))
	created := errors.Is(err, store.ErrNotFound)
	if err != nil && !created {
		return err
	}
	if created {
		then = store.Thread{Issue: store.Issue{Status: "open"}}
		if len(now.Events) > 0 { // what it was created with is new
			then.Events = []store.Event{{Kind: "created", ID: now.Events[0].ID}}
		}
	}
	d := diffThreads(then, now)
	d.Since, d.Created = since, created
	if js.on() {
		return js.write(out, d)
	}
	writeDiff(out, d)
	return nil
}

// diffThreads is what changed from then to now.
func diffThreads(then, now store.Thread) issueDiff {
	d := issueDiff{ID: now.ID, Added: []string{}, Removed: []string{}, Comments: []diffComment{}, EditedComments: []diffComment{},
		shown: now.ShownTitle(), number: now.Number}
	if then.Title != now.Title {
		d.Title = &change{Was: then.Title, Now: now.Title}
	}
	if then.Status != now.Status {
		d.Status = &change{Was: then.Status, Now: now.Status}
	}
	for _, l := range now.Labels {
		if !slices.Contains(then.Labels, l) {
			d.Added = append(d.Added, l)
		}
	}
	for _, l := range then.Labels {
		if !slices.Contains(now.Labels, l) {
			d.Removed = append(d.Removed, l)
		}
	}
	texts := map[string]store.Event{}
	for _, e := range then.Events {
		if e.Kind == "created" || e.Kind == "comment" {
			texts[e.ID] = e
		}
	}
	for _, e := range now.Events {
		was, had := texts[e.ID]
		switch {
		case e.Kind == "created" && was.Body != e.Body:
			d.Description = &change{Was: was.Body, Now: e.Body, Diff: lineDiff(was.Body, e.Body)}
		case e.Kind != "comment":
		case !had:
			d.Comments = append(d.Comments, diffComment{ID: e.ID, Author: e.Author, At: e.At, Body: e.Body})
		case was.Body != e.Body:
			d.EditedComments = append(d.EditedComments, diffComment{ID: e.ID, Author: e.Author, At: e.At, Was: was.Body, Body: e.Body})
		}
	}
	return d
}

func writeDiff(out io.Writer, d issueDiff) {
	head := d.shown
	if d.number != "" {
		head += " (" + d.number + ")"
	}
	fmt.Fprintf(out, "%s, since %s\n", head, when(d.Since))
	if d.Created {
		fmt.Fprintln(out, "created since")
	}
	empty := d.Title == nil && d.Status == nil && len(d.Added)+len(d.Removed) == 0 && d.Description == nil &&
		len(d.Comments)+len(d.EditedComments) == 0
	if empty {
		fmt.Fprintln(out, "No change.")
		return
	}
	if d.Title != nil {
		fmt.Fprintf(out, "title: %q → %q\n", d.Title.Was, d.Title.Now)
	}
	if d.Status != nil {
		fmt.Fprintf(out, "state: %s → %s\n", d.Status.Was, d.Status.Now)
	}
	if len(d.Added)+len(d.Removed) > 0 {
		var parts []string
		for _, l := range d.Added {
			parts = append(parts, "+"+l)
		}
		for _, l := range d.Removed {
			parts = append(parts, "-"+l)
		}
		fmt.Fprintf(out, "labels: %s\n", strings.Join(parts, " "))
	}
	if d.Description != nil {
		fmt.Fprintf(out, "\ndescription:\n%s", d.Description.Diff)
	}
	for _, c := range d.Comments {
		fmt.Fprintf(out, "\n+++ %s, %s [%s]\n%s\n", c.Author.Name, when(c.At), commentRef(c.ID), c.Body)
	}
	for _, c := range d.EditedComments {
		fmt.Fprintf(out, "\n~~~ edited: %s, %s [%s]\n%s", c.Author.Name, when(c.At), commentRef(c.ID), lineDiff(c.Was, c.Body))
	}
}

// maxDiff bounds lineDiff's table: past it, the whole text was and is.
const maxDiff = 4_000_000

// lineDiff is b against a, a line each, "- " removed, "+ " added, "  " kept:
// the longest common subsequence of their lines.
func lineDiff(a, b string) string {
	x, y := lines(a), lines(b)
	var sb strings.Builder
	if len(x)*len(y) > maxDiff {
		for _, l := range x {
			sb.WriteString("- " + l + "\n")
		}
		for _, l := range y {
			sb.WriteString("+ " + l + "\n")
		}
		return sb.String()
	}
	// lcs[i][j]: the longest common subsequence of x[i:] and y[j:]
	lcs := make([][]int, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			sb.WriteString("  " + x[i] + "\n")
			i, j = i+1, j+1
		case i < len(x) && (j == len(y) || lcs[i+1][j] >= lcs[i][j+1]):
			sb.WriteString("- " + x[i] + "\n")
			i++
		default:
			sb.WriteString("+ " + y[j] + "\n")
			j++
		}
	}
	return sb.String()
}

// lines are a text's lines; none for an empty text.
func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
