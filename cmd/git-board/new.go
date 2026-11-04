package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/store"
)

// newInput is what new was given: the title, the text flags, the
// template, and whether it may ask what's missing.
type newInput struct {
	title    string
	text     textFlags
	template string // --template's name
	prompt   bool   // stdin a terminal, and no --no-prompt nor GIT_BOARD_NO_PROMPT
}

func newNewCmd() *cobra.Command {
	var in newInput
	var me string
	var force, noPrompt bool
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

--template <name> starts from one of the repository's issue templates:
GitHub's .github/ISSUE_TEMPLATE (*.md, and the issue forms, *.yml, as
the text GitHub makes of them) and GitLab's .gitlab/issue_templates. Its
text is the description unless --body or --body-file gives one; its
labels and assignees are set first, the field=value arguments over
them. Its title starts the title you are asked for, or the editor's
first line; a title you give wins. A name no template has is exit code
2, listing them.

With no title on a terminal, new asks: a template first when the
repository has some, then the title, then whether to write the
description in your editor. It never asks when stdin is not a terminal,
with --no-prompt, or when GIT_BOARD_NO_PROMPT is set: no title is exit
code 2 then.

It prints the new issue's id, alone on its line; --json prints the issue
as ls --json prints each one. The issue goes to a forge with the next
sync, nowhere before.`,
		Example: `  git board new "Login fails on Safari"
  git board new "Login fails on Safari" prio=p1 assignee=+@me sprint=@current
  git board new "Export to CSV" -b "As ls --md, but CSV."
  git board new "Crash report" -F crash.md label=+bug
  git board new -e prio=p2
  git board new "Login fails" --template "Bug report"
  git board new                             # on a terminal: asks
  id=$(git board new "Follow-up")       # the id, for a script`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("title") && len(args) > 0 {
				if _, err := fields.Parse(args[0]); err != nil {
					in.title, args = args[0], args[1:]
				}
			}
			in.prompt = !noPrompt && os.Getenv("GIT_BOARD_NO_PROMPT") == "" && isTerminal(cmd.InOrStdin())
			if in.title == "" && !in.text.editor && !in.prompt {
				return usageError{fmt.Errorf("a title: git board new <title>, or --editor")}
			}
			return runNew(cmd, &in, args, resolveMe(me), force, js)
		},
	}
	cmd.Flags().StringVarP(&in.title, "title", "t", "", "the title")
	in.text.add(cmd, "description")
	cmd.Flags().StringVarP(&in.template, "template", "T", "", "start from this issue template (.github/ISSUE_TEMPLATE, .gitlab/issue_templates)")
	cmd.Flags().BoolVar(&noPrompt, "no-prompt", false, "never ask, even on a terminal (env GIT_BOARD_NO_PROMPT)")
	js = addJSON(cmd, "print the new issue as JSON", jsonFields(model.Issue{}), false)
	cmd.Flags().BoolVar(&force, "force", false, "write field values .git-board.toml doesn't allow")
	meFlag(cmd, &me)
	return cmd
}

func runNew(cmd *cobra.Command, in *newInput, args []string, me string, force bool, js *jsonOut) error {
	out := cmd.OutOrStdout()
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	var tmpl *issueTemplate
	var templates []issueTemplate
	if in.template != "" || (in.prompt && in.title == "" && !in.text.editor) {
		templates = loadTemplates()
	}
	if in.template != "" {
		t, err := findTemplate(templates, in.template)
		if err != nil {
			return err
		}
		tmpl = &t
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

	// the title, and a description to start from
	title, body, haveBody := in.title, "", false
	if in.prompt && title == "" && !in.text.editor {
		p := prompter{bufio.NewReader(cmd.InOrStdin()), cmd.ErrOrStderr()}
		if tmpl == nil && len(templates) > 0 {
			if tmpl, err = chooseTemplate(p, templates); err != nil {
				return err
			}
		}
		prefix := ""
		if tmpl != nil {
			prefix = tmpl.Title
		}
		typed, err := p.ask("Title: " + prefix)
		if err != nil {
			return err
		}
		if title = strings.TrimSpace(prefix + typed); typed == "" {
			return usageError{fmt.Errorf("a title: nothing written")}
		}
		if !in.text.given(cmd) {
			start := ""
			if tmpl != nil {
				start = tmpl.Body
			}
			answer, err := p.ask("Description: e to write it in your editor, enter to " + map[bool]string{true: "keep the template's", false: "leave it empty"}[start != ""] + ": ")
			if err != nil {
				return err
			}
			body, haveBody = start, true
			if strings.EqualFold(answer, "e") {
				buf, err := runEditor("new", start+"\n")
				if err != nil {
					return err
				}
				body = trimText(buf)
			}
		}
	}

	var initial []string
	if tmpl != nil {
		initial = tmpl.labels()
	}
	labels, _, err := foldAssignments(cfg, initial, assignments)
	if err != nil {
		return err
	}

	switch {
	case in.text.editor:
		head, start := title, ""
		if tmpl != nil {
			start = tmpl.Body
			if head == "" {
				head = tmpl.Title
			}
		}
		buf, err := runEditor("new", head+"\n\n"+start)
		if err != nil {
			return err
		}
		if title, body = splitTitle(buf); title == "" {
			return errAborted
		}
	case in.text.given(cmd):
		if body, err = in.text.read(cmd.InOrStdin()); err != nil {
			return err
		}
	case !haveBody && tmpl != nil:
		body = tmpl.Body
	}

	id, err := b.NewIssue(title, body, labels)
	if err != nil {
		if in.text.editor || haveBody {
			return keptErr(err, "new", title+"\n\n"+body)
		}
		return err
	}
	if _, err := statusInStep(b, cfg, id, assignments, labels); err != nil {
		return err
	}
	return printIssue(out, b, id, js)
}

// chooseTemplate asks which template to start from; none is nil.
func chooseTemplate(p prompter, templates []issueTemplate) (*issueTemplate, error) {
	fmt.Fprintln(p.w, "Templates:")
	for i, t := range templates {
		fmt.Fprintf(p.w, "  %d  %s\n", i+1, t.Name)
	}
	fmt.Fprintln(p.w, "  0  none")
	for {
		answer, err := p.ask("Template [0]: ")
		if err != nil {
			return nil, err
		}
		if answer == "" || answer == "0" {
			return nil, nil
		}
		if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(templates) {
			return &templates[n-1], nil
		}
		if t, err := findTemplate(templates, answer); err == nil {
			return &t, nil
		}
		fmt.Fprintf(p.w, "%q: a number from 0 to %d, or a name\n", answer, len(templates))
	}
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
