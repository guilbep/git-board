package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/people"
)

func newShow() *cobra.Command {
	var md, noComments bool
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show an issue: its fields, description and comments",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if md && js.on() {
				return usageError{fmt.Errorf("one of --md, --json, --web and --forge")}
			}
			ctx := cmd.Context()
			b, err := open(ctx, true)
			if err != nil {
				return err
			}
			defer b.Close()
			th, err := b.issues.Show(ctx, args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			switch {
			case js.on():
				if th.Participants == nil {
					th.Participants = []people.Who{}
				}
				if th.Events == nil {
					th.Events = []issues.Event{}
				}
				return js.write(out, th)
			case md:
				writeThreadMarkdown(out, th, b.issues.Schema, !noComments)
			default:
				writeThread(out, th, b.issues.Schema, !noComments)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&md, "md", false, "render as Markdown")
	js = addJSON(cmd, "render as JSON", jsonFields(issues.Thread{}), true)
	cmd.Flags().BoolVar(&noComments, "no-comments", false, "leave the comments out")
	return cmd
}

// when is a time as the commands print it, in local time.
func when(t time.Time) string { return t.Local().Format("2006-01-02 15:04") }

// headerLine is the line under the title: number, id, state, author,
// comments.
func headerLine(th issues.Thread) string {
	var parts []string
	if th.Number != "" {
		parts = append(parts, th.Number)
	}
	parts = append(parts, shortID(th.ID), th.Status, "opened by "+th.Author.Name+" on "+when(th.CreatedAt))
	n := 0
	for _, e := range th.Events {
		if e.Kind == "comment" {
			n++
		}
	}
	s := "s"
	if n == 1 {
		s = ""
	}
	return strings.Join(append(parts, fmt.Sprintf("%d comment%s", n, s)), " · ")
}

// fieldLines are an issue's fields, "name: values" each, then its plain
// labels, those it inherits, and its forges.
func fieldLines(is issues.Issue, s *issues.Schema) [][2]string {
	known, other := issues.ParseLabels(is.Labels)
	var res [][2]string
	for _, f := range known {
		res = append(res, [2]string{issues.DisplayName(f.Name), s.Format(f.Name, f.Values)})
	}
	if len(other) > 0 {
		res = append(res, [2]string{"labels", strings.Join(other, ", ")})
	}
	if len(is.Inherited) > 0 {
		res = append(res, [2]string{"inherited", strings.Join(is.Inherited, ", ")})
	}
	for _, f := range is.Forges {
		v := f.URL
		if f.Number != "" {
			v = "#" + f.Number + " " + f.URL
		}
		res = append(res, [2]string{f.Name, v})
	}
	return res
}

// commentRef is a comment's id as people type it: abc1234:1.
func commentRef(id string) string {
	hash, n, ok := strings.Cut(id, ":")
	if !ok {
		return shortID(id)
	}
	return hash[:min(7, len(hash))] + ":" + n
}

func commentHead(e issues.Event) string {
	s := e.Author.Name + ", " + when(e.At)
	if e.Edited {
		s += " (edited)"
	}
	return s + " [" + commentRef(e.ID) + "]"
}

func writeThread(out io.Writer, th issues.Thread, s *issues.Schema, comments bool) {
	fmt.Fprintln(out, th.ShownTitle())
	fmt.Fprintln(out, headerLine(th))
	if lines := fieldLines(th.Issue, s); len(lines) > 0 {
		fmt.Fprintln(out)
		width := 0
		for _, l := range lines {
			width = max(width, len(l[0]))
		}
		for _, l := range lines {
			fmt.Fprintf(out, "%-*s  %s\n", width+1, l[0]+":", l[1])
		}
	}
	for _, e := range th.Events {
		switch {
		case e.Kind == "created":
			fmt.Fprintln(out)
			if e.Body == "" {
				fmt.Fprintln(out, "No description.")
			} else {
				fmt.Fprintln(out, e.Body)
			}
		case e.Kind == "comment" && comments:
			fmt.Fprintf(out, "\n--- %s\n%s\n", commentHead(e), e.Body)
		}
	}
}

func writeThreadMarkdown(out io.Writer, th issues.Thread, s *issues.Schema, comments bool) {
	title := th.ShownTitle()
	if th.Number != "" {
		title += " (" + th.Number + ")"
	}
	fmt.Fprintf(out, "# %s\n\n%s\n", title, headerLine(th))
	if lines := fieldLines(th.Issue, s); len(lines) > 0 {
		fmt.Fprint(out, "\n| field | value |\n|---|---|\n")
		for _, l := range lines {
			fmt.Fprintf(out, "| %s | %s |\n", l[0], strings.ReplaceAll(l[1], "|", `\|`))
		}
	}
	for _, e := range th.Events {
		switch {
		case e.Kind == "created" && e.Body != "":
			fmt.Fprintf(out, "\n%s\n", e.Body)
		case e.Kind == "comment" && comments:
			fmt.Fprintf(out, "\n## %s\n\n%s\n", commentHead(e), e.Body)
		}
	}
}

func newLog() *cobra.Command {
	var field, since string
	var js *jsonOut
	cmd := &cobra.Command{
		Use:     "log <id>",
		Aliases: []string{"history"},
		Short:   "Every change made to an issue: who, when, what",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := issues.LogReq{Name: args[0], Field: field}
			if since != "" {
				var err error
				if req.Since, err = parseSince(since, time.Now()); err != nil {
					return usageError{err}
				}
			}
			ctx := cmd.Context()
			b, err := open(ctx, true)
			if err != nil {
				return err
			}
			defer b.Close()
			entries, th, err := b.issues.Log(ctx, req)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if js.on() {
				return js.write(out, entries)
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			for _, e := range entries {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", when(e.At), e.Author.Name, describe(e, th))
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&field, "field", "", "only the changes of this field")
	cmd.Flags().StringVar(&since, "since", "", "only the changes from this date (YYYY-MM-DD) or this long ago (36h, 7d)")
	js = addJSON(cmd, "render as JSON", jsonFields(issues.LogEntry{}), true)
	return cmd
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

// describe is what a change did, in a line.
func describe(e issues.LogEntry, th issues.Thread) string {
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

// writeJSON prints v as indented JSON.
func writeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
