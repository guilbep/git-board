package cli

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/people"
)

// newDiff is v1's diff: what changed on an issue since a time. The issue
// then is folded from the operations made before --since (ShowAt), now
// from all of them.
func newDiff() *cobra.Command {
	var since string
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "diff <id> --since <when>",
		Short: "What changed on an issue since a date",
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
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			ctx := cmd.Context()
			now, err := b.issues.Show(ctx, args[0])
			if err != nil {
				return err
			}
			then, err := b.issues.ShowAt(ctx, now.ID, from.Add(-time.Second))
			created := errors.Is(err, issues.ErrNotFound)
			if err != nil && !created {
				return err
			}
			if created {
				then = issues.Thread{Issue: issues.Issue{Status: "open"}}
				if len(now.Events) > 0 { // what it was created with is new
					then.Events = []issues.Event{{Kind: "created", ID: now.Events[0].ID}}
				}
			}
			d := diffThreads(then, now)
			d.Since, d.Created = from, created
			if js.on() {
				return js.write(cmd.OutOrStdout(), d)
			}
			writeDiff(cmd.OutOrStdout(), d)
			return nil
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
	ID     string     `json:"id"`
	Author people.Who `json:"author"`
	At     time.Time  `json:"at"`
	Was    string     `json:"was,omitempty"` // edited: the text then
	Body   string     `json:"body"`
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

// diffThreads is what changed from then to now.
func diffThreads(then, now issues.Thread) issueDiff {
	d := issueDiff{ID: now.ID, Added: []string{}, Removed: []string{}, Comments: []diffComment{}, EditedComments: []diffComment{},
		shown: now.ShownTitle(), number: now.Number}
	if then.Title != now.Title {
		d.Title = &change{Was: then.Title, Now: now.Title}
	}
	if then.Status != now.Status {
		d.Status = &change{Was: string(then.Status), Now: string(now.Status)}
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
	texts := map[string]issues.Event{}
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
	if d.Title == nil && d.Status == nil && len(d.Added)+len(d.Removed) == 0 && d.Description == nil &&
		len(d.Comments)+len(d.EditedComments) == 0 {
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

// lineDiff is b against a, a line each, "- " removed, "+ " added, "  "
// kept: the longest common subsequence of their lines.
func lineDiff(a, b string) string {
	x, y := textLines(a), textLines(b)
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

// textLines are a text's lines; none for an empty text.
func textLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
