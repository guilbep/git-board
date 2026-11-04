package main

import (
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/internal/store"
)

func newLogCmd() *cobra.Command {
	var field, since string
	var js *jsonOut

	cmd := &cobra.Command{
		Use:     "log <id>",
		Aliases: []string{"history"},
		Short:   "Every change made to an issue: who, when, what",
		Long: `Every change made to an issue, oldest first: its creation, each
comment, each edit of its description or of a comment, each title,
state and label change, with who made it and when. The board keeps every
operation, so nothing is missing: a forge shows label events at best,
and keeps no text's earlier versions.

--field <name> keeps the changes of one field (status, prio, assignee…),
and answers "who set prio:p0, when, and what was it before". --since
keeps the changes from a date (2026-10-01) or a while ago (36h, 7d).

--json prints an array of entries: {"at", "author", "kind", "id"}, kind
one of created, comment, edit, title, state, labels, and as the kind
has them "target" (edit: the text's id, the created entry's for the
description), "title" and "was" (title), "state" (open or closed),
"added" and "removed" (labels), "body" (created, comment, edit: the
text).`,
		Example: `  git board log a1b2c3d
  git board log a1b2c3d --field status         # its path through the workflow
  git board log CO2-3075 --since 7d
  git board log a1b2c3d --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var from time.Time
			if since != "" {
				var err error
				if from, err = parseSince(since, time.Now()); err != nil {
					return usageError{err}
				}
			}
			return runLog(cmd.OutOrStdout(), args[0], field, from, js)
		},
	}
	cmd.Flags().StringVar(&field, "field", "", "only the changes of this field")
	cmd.Flags().StringVar(&since, "since", "", "only the changes from this date (YYYY-MM-DD) or this long ago (36h, 7d)")
	js = addJSON(cmd, "render as JSON", jsonFields(logEntry{}), true)
	return cmd
}

// logEntry is one change, as log prints it.
type logEntry struct {
	At      time.Time    `json:"at"`
	Author  model.Person `json:"author"`
	Kind    string       `json:"kind"` // created, comment, edit, title, state, labels
	ID      string       `json:"id"`
	Target  string       `json:"target,omitempty"`
	Title   string       `json:"title,omitempty"`
	Was     string       `json:"was,omitempty"`
	State   string       `json:"state,omitempty"`
	Added   []string     `json:"added,omitempty"`
	Removed []string     `json:"removed,omitempty"`
	Body    string       `json:"body,omitempty"`
}

var daysAgo = regexp.MustCompile(`^(\d+)d$`)

// parseSince is a date, or a while before now: 36h, 7d.
func parseSince(s string, now time.Time) (time.Time, error) {
	if t, err := time.ParseInLocation(time.DateOnly, s, time.Local); err == nil {
		return t, nil
	}
	if m := daysAgo.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return now.AddDate(0, 0, -n), nil
	}
	if d, err := time.ParseDuration(s); err == nil && d >= 0 {
		return now.Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("--since %q: a date (2026-10-01) or a while ago (36h, 7d)", s)
}

// historyOf is a thread's changes, in the order the log applied them
// (order, Backend.OpOrder), else in time.
func historyOf(th model.Thread, order map[string]int) []logEntry {
	var res []logEntry
	for _, e := range th.Events {
		le := logEntry{At: e.At, Author: e.Author, Kind: e.Kind, ID: e.ID}
		switch e.Kind {
		case "created":
			le.Title, le.Body = firstTitle(th), firstBody(e)
		case "comment":
			le.Body = firstBody(e)
		case "title":
			le.Title, le.Was = e.Title, e.Was
		case "status":
			le.Kind, le.State = "state", e.Status
		case "labels":
			le.Added, le.Removed = e.Added, e.Removed
		}
		res = append(res, le)
		for _, v := range slices.Clip(e.History[min(1, len(e.History)):]) {
			res = append(res, logEntry{At: v.At, Author: v.Author, Kind: "edit", ID: v.ID, Target: e.ID, Body: v.Body})
		}
	}
	slices.SortStableFunc(res, func(x, y logEntry) int {
		i, okX := order[x.ID]
		j, okY := order[y.ID]
		if okX && okY {
			return i - j
		}
		return x.At.Compare(y.At)
	})
	return res
}

// firstBody is a text as it was written, before any edit.
func firstBody(e model.Event) string {
	if len(e.History) > 0 {
		return e.History[0].Body
	}
	return e.Body
}

// firstTitle is the title the issue was created with: the first title
// change's was, else the title.
func firstTitle(th model.Thread) string {
	for _, e := range th.Events {
		if e.Kind == "title" {
			return e.Was
		}
	}
	return th.Title
}

// keepField keeps the label changes of field, each with that field's
// labels only.
func keepField(entries []logEntry, field string) []logEntry {
	in := func(ls []string) []string {
		return slices.DeleteFunc(slices.Clone(ls), func(l string) bool { return !strings.HasPrefix(l, field+":") })
	}
	var res []logEntry
	for _, e := range entries {
		if e.Kind != "labels" {
			continue
		}
		e.Added, e.Removed = in(e.Added), in(e.Removed)
		if len(e.Added)+len(e.Removed) > 0 {
			res = append(res, e)
		}
	}
	return res
}

func runLog(out io.Writer, idPrefix, field string, since time.Time, js *jsonOut) error {
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()
	th, err := b.Thread(idPrefix)
	if err != nil {
		return err
	}
	order, err := b.OpOrder(th.ID)
	if err != nil {
		return err
	}
	entries := historyOf(th, order)
	if field != "" {
		entries = keepField(entries, field)
	}
	entries = slices.DeleteFunc(entries, func(e logEntry) bool { return e.At.Before(since) })
	if js.on() {
		if entries == nil {
			entries = []logEntry{}
		}
		return js.write(out, entries)
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	for _, e := range entries {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", when(e.At), e.Author.Name, describeEntry(e, th))
	}
	return tw.Flush()
}

// describeEntry is what a change did, in a line.
func describeEntry(e logEntry, th model.Thread) string {
	switch e.Kind {
	case "created":
		return fmt.Sprintf("created %q", e.Title)
	case "comment":
		return "commented [" + commentRef(e.ID) + "]: " + excerpt(e.Body)
	case "edit":
		if len(th.Events) > 0 && e.Target == th.Events[0].ID {
			return "edited the description: " + excerpt(e.Body)
		}
		return "edited comment [" + commentRef(e.Target) + "]: " + excerpt(e.Body)
	case "title":
		return fmt.Sprintf("retitled %q → %q", e.Was, e.Title)
	case "state":
		return map[string]string{"open": "reopened", "closed": "closed"}[e.State]
	case "labels":
		var parts []string
		for _, l := range e.Added {
			parts = append(parts, "+"+l)
		}
		for _, l := range e.Removed {
			parts = append(parts, "-"+l)
		}
		return strings.Join(parts, " ")
	}
	return e.Kind
}

// excerpt is a text's first line, cut at 60 characters.
func excerpt(s string) string {
	line, _, more := strings.Cut(strings.TrimSpace(s), "\n")
	if r := []rune(line); len(r) > 60 {
		line, more = string(r[:59]), true
	}
	if more {
		line += "…"
	}
	return line
}
