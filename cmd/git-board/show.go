package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/tui"
)

func newShowCmd() *cobra.Command {
	var md, asJSON, noComments bool
	var at string

	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show an issue: its fields, description and comments",
		Long: `Show an issue: its title, number, state, author, fields, forges,
description and comments, each comment with its ref (abc1234:1) for
"git board comment --edit".

<id> is a prefix of its id (7 characters or more), its project number
(CO2-3075) or a forge's (#2528, quoted: # starts a shell comment).

--md renders it as Markdown. --json prints the whole thread, documented
in docs/guide/cli.md: the issue's fields as ls --json prints them, plus
"author", "edited_at", "participants" and "events" (created, comment,
labels, status, title, each with its "id", "author" and "at"; a text's
earlier versions under "history").

--at shows it as it was then, folded from the operations made by then:
a date (as it stood at its end), a time (2026-10-01T14:30) or a while
ago (36h, 7d). Each operation counts at its own time, its writer's
clock.`,
		Example: `  git board show a1b2c3d
  git board show CO2-3075 --md
  git board show "#2528" --json
  git board show a1b2c3d --no-comments
  git board show a1b2c3d --at 2026-10-01`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if md && asJSON {
				return usageError{fmt.Errorf("--md or --json, not both")}
			}
			var when time.Time
			if at != "" {
				var err error
				if when, err = parseAt(at, time.Now()); err != nil {
					return usageError{err}
				}
			}
			return runShow(cmd.OutOrStdout(), args[0], md, asJSON, !noComments, when)
		},
	}
	cmd.Flags().StringVar(&at, "at", "", "as it was then: a date (its end), a time (2026-10-01T14:30) or a while ago (36h, 7d)")
	cmd.Flags().BoolVar(&md, "md", false, "render as Markdown")
	cmd.Flags().BoolVar(&asJSON, "json", false, "render as JSON")
	cmd.Flags().BoolVar(&noComments, "no-comments", false, "leave the comments out")
	return cmd
}

// runShow is show; at, when set, is the time to show the issue as it was.
func runShow(out io.Writer, idPrefix string, md, asJSON, comments bool, at time.Time) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()
	th, err := b.Thread(idPrefix)
	if !at.IsZero() {
		th, err = b.ThreadAt(idPrefix, at)
	}
	if err != nil {
		return err
	}
	if asJSON {
		if th.Participants == nil {
			th.Participants = []store.Person{}
		}
		if th.Events == nil {
			th.Events = []store.Event{}
		}
		return writeJSON(out, th)
	}
	if !at.IsZero() {
		fmt.Fprintf(out, "As it was on %s.\n\n", when(at))
	}
	if md {
		writeThreadMarkdown(out, th, cfg, comments)
	} else {
		writeThread(out, th, cfg, comments)
	}
	return nil
}

// when is a time as the commands print it, in local time.
func when(t time.Time) string { return t.Local().Format("2006-01-02 15:04") }

// headerLine is the line under the title: number, id, state, author,
// comments.
func headerLine(th store.Thread) string {
	parts := []string{}
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
	parts = append(parts, fmt.Sprintf("%d comment%s", n, map[bool]string{true: "", false: "s"}[n == 1]))
	return strings.Join(parts, " · ")
}

// fieldLines are an issue's fields, one "name: values" each, then its
// plain labels and those it inherits.
func fieldLines(is store.Issue, cfg *config.Config) [][2]string {
	known, other := tui.ParseLabels(is.Labels)
	var res [][2]string
	for _, f := range known {
		res = append(res, [2]string{tui.DisplayFieldName(f.Name), formatFieldValues(f.Name, f.Values, cfg)})
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

func commentHead(e store.Event) string {
	s := e.Author.Name + ", " + when(e.At)
	if e.Edited {
		s += " (edited)"
	}
	return s + " [" + commentRef(e.ID) + "]"
}

func writeThread(out io.Writer, th store.Thread, cfg *config.Config, comments bool) {
	fmt.Fprintln(out, th.ShownTitle())
	fmt.Fprintln(out, headerLine(th))
	if lines := fieldLines(th.Issue, cfg); len(lines) > 0 {
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

func writeThreadMarkdown(out io.Writer, th store.Thread, cfg *config.Config, comments bool) {
	title := th.ShownTitle()
	if th.Number != "" {
		title += " (" + th.Number + ")"
	}
	fmt.Fprintf(out, "# %s\n\n%s\n", title, headerLine(th))
	if lines := fieldLines(th.Issue, cfg); len(lines) > 0 {
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
