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
	var md, asJSON, noComments, web, forge bool

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

--web opens the issue in this repository's web UI: where its server
already runs, else it starts one here, as a bare "git board" does, until
Ctrl-C. --forge opens it on its forge, the first one it is on; an issue
on none is exit code 3. Both print the URL they open.`,
		Example: `  git board show a1b2c3d
  git board show CO2-3075 --md
  git board show "#2528" --json
  git board show a1b2c3d --no-comments
  git board show a1b2c3d --web
  git board show a1b2c3d --forge`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n := 0
			for _, on := range []bool{md, asJSON, web, forge} {
				if on {
					n++
				}
			}
			if n > 1 {
				return usageError{fmt.Errorf("one of --md, --json, --web and --forge")}
			}
			if web || forge {
				return runShowOpen(cmd, args[0], forge)
			}
			return runShow(cmd.OutOrStdout(), args[0], md, asJSON, !noComments)
		},
	}
	cmd.Flags().BoolVar(&md, "md", false, "render as Markdown")
	cmd.Flags().BoolVar(&asJSON, "json", false, "render as JSON")
	cmd.Flags().BoolVar(&noComments, "no-comments", false, "leave the comments out")
	cmd.Flags().BoolVarP(&web, "web", "w", false, "open it in this repository's web UI")
	cmd.Flags().BoolVar(&forge, "forge", false, "open it on its forge")
	return cmd
}

func runShow(out io.Writer, idPrefix string, md, asJSON, comments bool) error {
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
	if md {
		writeThreadMarkdown(out, th, cfg, comments)
	} else {
		writeThread(out, th, cfg, comments)
	}
	return nil
}

// runShowOpen opens the issue in the browser: on its forge, or in the
// web UI.
func runShowOpen(cmd *cobra.Command, idPrefix string, forge bool) error {
	out := cmd.OutOrStdout()
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	is, err := b.GetIssue(idPrefix)
	b.Close() // before a server opens the board
	if err != nil {
		return err
	}
	if !forge {
		return runBoardAt(cmd.Context(), out, true, "/#/issues/"+is.ID)
	}
	for _, f := range is.Forges {
		if f.URL != "" {
			fmt.Fprintln(out, f.URL)
			return openURL(f.URL)
		}
	}
	return fmt.Errorf("%w: %s is on no forge yet (git board sync sends it)", store.ErrNotFound, shortID(is.ID))
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
