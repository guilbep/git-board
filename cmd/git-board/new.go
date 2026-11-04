package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/store"
)

func newNewCmd() *cobra.Command {
	var text textFlags
	var title, me string
	var force bool
	var js *jsonOut

	cmd := &cobra.Command{
		Use:   "new [title] [field=value...]",
		Short: "Create an issue",
		Long: `Create an issue, as the person you write as (git board me), with its
fields in the same change.

The title is the first argument, or --title. The description comes from
--body, --body-file (- for stdin) or --editor: with --editor the buffer's
first line is the title and the rest the description, as gh's -e. The
field=value arguments are set's (git board set --help): checked against
.git-board.toml unless --force, @me and @next resolved.

The first argument is read as the title unless it is a field=value: a
title like "a=b" goes with --title.

It prints the new issue's id, alone on its line; --json prints the issue
as ls --json prints each one. The issue goes to a forge with the next
sync, nowhere before.`,
		Example: `  git board new "Login fails on Safari"
  git board new "Login fails on Safari" prio=p1 assignee=+@me sprint=@current
  git board new "Export to CSV" -b "As ls --md, but CSV."
  git board new "Crash report" -F crash.md label=+bug
  git board new -e prio=p2
  id=$(git board new "Follow-up")       # the id, for a script`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("title") && len(args) > 0 {
				if _, err := fields.Parse(args[0]); err != nil {
					title, args = args[0], args[1:]
				}
			}
			if title == "" && !text.editor {
				return usageError{fmt.Errorf("a title: git board new <title>, or --editor")}
			}
			return runNew(cmd, title, args, &text, resolveMe(me), force, js)
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "the title")
	text.add(cmd, "description")
	// no --template: new --template is for an issue template, as gh's
	js = addJSON(cmd, "print the new issue as JSON", jsonFields(store.Issue{}), false)
	cmd.Flags().BoolVar(&force, "force", false, "write field values .git-board.toml doesn't allow")
	meFlag(cmd, &me)
	return cmd
}

func runNew(cmd *cobra.Command, title string, args []string, text *textFlags, me string, force bool, js *jsonOut) error {
	out := cmd.OutOrStdout()
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()
	if err := needMe(b); err != nil {
		return err
	}

	assignments, err := parseAssignments(b, cfg, args, me, force)
	if err != nil {
		return err
	}
	labels, _, err := foldAssignments(cfg, nil, assignments)
	if err != nil {
		return err
	}

	var body string
	if text.editor {
		buf, err := runEditor("new", title+"\n\n")
		if err != nil {
			return err
		}
		if title, body = splitTitle(buf); title == "" {
			return errAborted
		}
	} else if body, err = text.read(cmd.InOrStdin()); err != nil {
		return err
	}

	id, err := b.NewIssue(title, body, labels)
	if err != nil {
		if text.editor {
			return keptErr(err, "new", title+"\n\n"+body)
		}
		return err
	}
	if _, err := statusInStep(b, cfg, id, assignments, labels); err != nil {
		return err
	}
	return printIssue(out, b, id, js)
}

// printIssue prints an issue's id, or the issue as JSON, as ls --json
// prints each one.
func printIssue(out io.Writer, b *store.Backend, id string, js *jsonOut) error {
	if !js.on() {
		fmt.Fprintln(out, id)
		return nil
	}
	is, err := b.GetIssue(id)
	if err != nil {
		return err
	}
	return js.write(out, is)
}

// needMe fails a write before it starts, before an editor opens, when no
// one is chosen to write as.
func needMe(b *store.Backend) error {
	_, ok, err := b.Me()
	if err == nil && !ok {
		err = store.ErrNoMe
	}
	return err
}
