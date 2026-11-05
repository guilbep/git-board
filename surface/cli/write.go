package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/kernel"
)

// keptInHistory is what a delete says: a tombstone hides, it doesn't
// erase (ADR 0019).
const keptInHistory = "its text stays in the board's history, in every clone that has it: revoke a leaked secret"

func meFlag(cmd *cobra.Command, me *string) {
	cmd.Flags().StringVar(me, "me", "", "current user, for @me filters/values (env GIT_BOARD_ME)")
}

func resolveMe(flag string) string {
	if flag != "" {
		return flag
	}
	return os.Getenv("GIT_BOARD_ME")
}

func newNew() *cobra.Command {
	var title, template, me string
	var text textFlags
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
				if _, err := issues.ParseAssignment(args[0]); err != nil {
					title, args = args[0], args[1:]
				}
			}
			prompt := !noPrompt && os.Getenv("GIT_BOARD_NO_PROMPT") == "" && isTerminal(cmd.InOrStdin())
			if title == "" && !text.editor && !prompt {
				return usageError{fmt.Errorf("a title: git board new <title>, or --editor")}
			}
			asks := prompt && title == "" && !text.editor
			var tmpl *issueTemplate
			var templates []issueTemplate
			if template != "" || asks {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				templates = loadTemplates(filepath.Dir(app.ConfigPath(cwd)))
			}
			if template != "" {
				t, err := findTemplate(templates, template)
				if err != nil {
					return err
				}
				tmpl = &t
			}
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			if _, err := b.People.Whom(); err != nil {
				return err
			}
			body, haveBody := "", false
			if asks {
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
				if !text.given(cmd) {
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
			switch {
			case text.editor:
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
			case text.given(cmd):
				if body, err = text.read(cmd.InOrStdin()); err != nil {
					return err
				}
			case !haveBody && tmpl != nil:
				body = tmpl.Body
			}
			is, err := b.issues.New(cmd.Context(), issues.NewReq{Title: title, Body: body, Labels: initial, Assign: args, Force: force, MeName: resolveMe(me)})
			if err != nil {
				if text.editor || haveBody {
					return keptErr(err, "new", title+"\n\n"+body)
				}
				return err
			}
			if js.on() {
				return js.write(cmd.OutOrStdout(), is)
			}
			fmt.Fprintln(cmd.OutOrStdout(), is.ID)
			return nil
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "the title")
	text.add(cmd, "description")
	cmd.Flags().StringVarP(&template, "template", "T", "", "start from this issue template (.github/ISSUE_TEMPLATE, .gitlab/issue_templates)")
	cmd.Flags().BoolVar(&noPrompt, "no-prompt", false, "never ask, even on a terminal (env GIT_BOARD_NO_PROMPT)")
	js = addJSON(cmd, "print the new issue as JSON", jsonFields(issues.Issue{}), false)
	cmd.Flags().BoolVar(&force, "force", false, "write field values .git-board.toml doesn't allow")
	meFlag(cmd, &me)
	return cmd
}

// findComment is the comment ref names on th (show's abc1234:1, a prefix
// of its hash, its whole id), or, ref empty, the last one meID wrote.
func findComment(th issues.Thread, ref string, meID kernel.ID) (*issues.Event, error) {
	var found []int
	for i, e := range th.Events {
		if e.Kind != "comment" {
			continue
		}
		if ref == "" {
			if e.Author.ID == meID {
				found = []int{i}
			}
			continue
		}
		hash, n, _ := strings.Cut(e.ID, ":")
		rh, rn, hasN := strings.Cut(ref, ":")
		if e.ID == ref || rh != "" && strings.HasPrefix(hash, rh) && (!hasN || rn == n) {
			found = append(found, i)
		}
	}
	switch {
	case len(found) == 1:
		return &th.Events[found[0]], nil
	case len(found) > 1:
		return nil, fmt.Errorf("%w: comment %s, on %d comments", kernel.ErrAmbiguous, ref, len(found))
	case ref == "":
		return nil, fmt.Errorf("%w: you haven't commented on %s", issues.ErrNoSuchComment, shortID(th.ID))
	}
	rh, _, _ := strings.Cut(ref, ":")
	if desc, _, _ := strings.Cut(th.Events[0].ID, ":"); rh != "" && strings.HasPrefix(desc, rh) {
		return nil, errors.New("that is the description: git board edit changes it")
	}
	return nil, fmt.Errorf("%w: %s (git board show %s lists them)", issues.ErrNoSuchComment, ref, shortID(th.ID))
}

func newComment() *cobra.Command {
	var text textFlags
	var edit, del string
	var editLast, yes bool
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "comment <id>",
		Short: "Comment on an issue, or edit or delete a comment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if edit != "" && editLast {
				return usageError{fmt.Errorf("--edit or --edit-last, not both")}
			}
			if del != "" && (edit != "" || editLast || text.given(cmd)) {
				return usageError{fmt.Errorf("--delete takes no text, nor --edit")}
			}
			if yes && del == "" {
				return usageError{fmt.Errorf("--yes goes with --delete")}
			}
			b, err := open(cmd, true)
			ctx := cmd.Context()
			if err != nil {
				return err
			}
			defer b.Close()
			me, err := b.People.Whom()
			if err != nil {
				return err
			}
			// the thread when a comment of it is named; else the issue's id
			var th issues.Thread
			if del != "" || edit != "" || editLast {
				th, err = b.issues.Show(ctx, args[0])
			} else {
				var id kernel.ID
				id, err = b.issues.Resolve(ctx, args[0])
				th.ID = string(id)
			}
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if del != "" {
				target, err := findComment(th, del, "")
				if err != nil {
					return err
				}
				line, _, _ := strings.Cut(target.Body, "\n")
				if r := []rune(line); len(r) > 50 {
					line = string(r[:50]) + "…"
				}
				if err := confirm(cmd, yes, fmt.Sprintf("delete %s's comment %s %q?", target.Author.Name, commentRef(target.ID), line)); err != nil {
					return err
				}
				if err := b.issues.DeleteComment(ctx, th.ID, kernel.OpID(target.ID)); err != nil {
					return err
				}
				fmt.Fprintln(cmd.ErrOrStderr(), "note: "+keptInHistory)
				if js.on() {
					return js.write(out, map[string]any{"id": target.ID, "ref": commentRef(target.ID), "issue": th.ID, "deleted": true})
				}
				fmt.Fprintf(out, "deleted %s\n", commentRef(target.ID))
				return nil
			}
			var target *issues.Event
			if edit != "" || editLast {
				if target, err = findComment(th, edit, me); err != nil {
					return err
				}
			}
			initial := ""
			if target != nil {
				initial = target.Body + "\n"
			}
			var body string
			if text.editor || !text.given(cmd) {
				if body, err = runEditor("comment", initial); err == nil {
					body = trimText(body)
				}
			} else {
				body, err = text.read(cmd.InOrStdin())
			}
			if err == nil && body == "" {
				err = errAborted
			}
			if err != nil {
				return err
			}
			var id string
			if target == nil {
				cid, _, err := b.issues.Comment(ctx, th.ID, body)
				if err != nil {
					return keptErr(err, "comment", body)
				}
				id = string(cid)
			} else {
				id = target.ID
				if err := b.issues.EditText(ctx, th.ID, kernel.OpID(id), body, target.Body); err != nil {
					return keptErr(err, "comment", body)
				}
			}
			if js.on() {
				return js.write(out, map[string]string{"id": id, "ref": commentRef(id), "issue": th.ID})
			}
			fmt.Fprintln(out, commentRef(id))
			return nil
		},
	}
	text.add(cmd, "comment")
	cmd.Flags().StringVar(&edit, "edit", "", "edit the comment with this ref (as show prints it)")
	cmd.Flags().BoolVar(&editLast, "edit-last", false, "edit your last comment on the issue")
	cmd.Flags().StringVar(&del, "delete", "", "delete the comment with this ref (as show prints it)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "with --delete: don't ask")
	js = addJSON(cmd, "print the comment's id as JSON", []string{"id", "ref", "issue", "deleted"}, true)
	return cmd
}

func newEdit() *cobra.Command {
	var title string
	var text textFlags
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit an issue's title or description",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			setTitle := cmd.Flags().Changed("title")
			if !setTitle && !text.given(cmd) {
				return usageError{fmt.Errorf("nothing to change: --title, --body, --body-file or --editor")}
			}
			if setTitle && strings.TrimSpace(title) == "" {
				return usageError{fmt.Errorf("an empty title")}
			}
			if setTitle && text.editor {
				return usageError{fmt.Errorf("--editor edits the title too: --title or --editor, not both")}
			}
			b, err := open(cmd, true)
			ctx := cmd.Context()
			if err != nil {
				return err
			}
			defer b.Close()
			if _, err := b.People.Whom(); err != nil {
				return err
			}
			th, err := b.issues.Show(ctx, args[0])
			if err != nil {
				return err
			}
			body, setBody := th.Events[0].Body, text.given(cmd)
			switch {
			case text.editor:
				buf, err := runEditor("edit", th.Title+"\n\n"+body+"\n")
				if err != nil {
					return err
				}
				if title, body = splitTitle(buf); title == "" {
					return errAborted
				}
				setTitle = true
			case setBody:
				if body, err = text.read(cmd.InOrStdin()); err != nil {
					return err
				}
			}
			var t, d *string
			if setTitle {
				t = &title
			}
			if setBody {
				d = &body
			}
			changed, err := b.issues.Edit(ctx, th.ID, t, d, th)
			if err != nil {
				return keptErr(err, "edit", title+"\n\n"+body)
			}
			if len(changed) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: unchanged\n", args[0])
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s edited\n", args[0], strings.Join(changed, " and "))
			return nil
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "the new title")
	text.add(cmd, "description")
	return cmd
}

func newSet() *cobra.Command {
	var me, filter string
	var force, apply bool
	cmd := &cobra.Command{
		Use:   "set <id> field=value [field=value...] | set --filter <filter> field=value... [--apply]",
		Short: "Set one or more fields on an issue",
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("filter") {
				return cobra.MinimumNArgs(1)(cmd, args)
			}
			return cobra.MinimumNArgs(2)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			byFilter := cmd.Flags().Changed("filter")
			if apply && !byFilter {
				return usageError{fmt.Errorf("--apply goes with --filter: set <id> writes at once")}
			}
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			var lines []string
			if byFilter {
				if apply {
					if _, err := b.People.Whom(); err != nil {
						return err
					}
				}
				lines, err = b.issues.SetMatching(cmd.Context(), issues.SetMatchingReq{Filter: filter, Assign: args, Force: force, MeName: resolveMe(me), Apply: apply})
			} else {
				if _, err := b.People.Whom(); err != nil {
					return err
				}
				lines, err = b.issues.Set(cmd.Context(), issues.SetReq{Name: args[0], Assign: args[1:], Force: force, MeName: resolveMe(me)})
			}
			for _, l := range lines {
				fmt.Fprintln(cmd.OutOrStdout(), l)
			}
			return err
		},
	}
	meFlag(cmd, &me)
	cmd.Flags().StringVar(&filter, "filter", "", "set the fields on every issue this filter matches (ls's grammar)")
	cmd.Flags().BoolVar(&apply, "apply", false, "with --filter: write the changes, not only show them")
	cmd.Flags().BoolVar(&force, "force", false, "write values .git-board.toml doesn't allow (an unknown field, a value outside the options)")
	return cmd
}

func newMove() *cobra.Command {
	var next, prev bool
	cmd := &cobra.Command{
		Use:   "move <id> [status] | move <id> --next | --prev",
		Short: "Move an issue to a status, or the next or previous one",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				if next || prev {
					return usageError{fmt.Errorf("a status, or --next or --prev: one of them")}
				}
				if strings.ContainsAny(args[1], "=") || args[1] == "" {
					return usageError{fmt.Errorf("move %s %q: a status option, as [fields.status] lists them", args[0], args[1])}
				}
			} else if next == prev {
				return usageError{fmt.Errorf("a status, or --next or --prev")}
			}
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			if _, err := b.People.Whom(); err != nil {
				return err
			}
			ctx := cmd.Context()
			req := issues.SetReq{Name: args[0]}
			if len(args) == 2 {
				req.Assign = []string{"status=" + args[1]}
			} else {
				is, err := b.issues.Get(ctx, args[0])
				if err != nil {
					return err
				}
				to, err := b.issues.Schema.Step(statusOf(is.Labels), prev)
				if err != nil {
					return err
				}
				req.Assign, req.Force = []string{"status=" + to}, true
			}
			lines, err := b.issues.Set(ctx, req)
			if err != nil {
				return err
			}
			for _, l := range lines {
				fmt.Fprintln(cmd.OutOrStdout(), l)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&next, "next", false, "the next status option")
	cmd.Flags().BoolVar(&prev, "prev", false, "the previous status option")
	return cmd
}

func statusOf(labels []string) string {
	for _, l := range labels {
		if v, ok := strings.CutPrefix(l, "status:"); ok {
			return v
		}
	}
	return ""
}

// closedAs is the label close --reason writes.
const closedAs = "closed-as"

var closeReasons = []string{"completed", "not-planned", "duplicate"}

func newClose() *cobra.Command {
	var comment, reason, duplicateOf string
	cmd := &cobra.Command{
		Use:   "close <id>",
		Short: "Close an issue: its status the terminal option",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := strings.ToLower(strings.NewReplacer(" ", "-", "_", "-").Replace(strings.TrimSpace(reason)))
			if r == "" {
				r = "completed"
			}
			if !slices.Contains(closeReasons, r) {
				return usageError{fmt.Errorf("--reason %q: one of %s", reason, strings.Join(closeReasons, ", "))}
			}
			if duplicateOf != "" {
				if cmd.Flags().Changed("reason") && r != "duplicate" {
					return usageError{fmt.Errorf("--duplicate-of closes as a duplicate, not --reason %s", r)}
				}
				r = "duplicate"
			}
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			if _, err := b.People.Whom(); err != nil {
				return err
			}
			line, err := b.issues.Close(cmd.Context(), issues.CloseReq{Name: args[0], Comment: trimText(comment), Reason: r, DuplicateOf: duplicateOf})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), line)
			return nil
		},
	}
	cmd.Flags().StringVarP(&comment, "comment", "c", "", "comment before closing")
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "why: completed (the default), not-planned or duplicate")
	cmd.Flags().StringVar(&duplicateOf, "duplicate-of", "", "the issue this one duplicates: closes it as a duplicate")
	return cmd
}

func newReopen() *cobra.Command {
	var comment string
	cmd := &cobra.Command{
		Use:   "reopen <id>",
		Short: "Reopen a closed issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			if _, err := b.People.Whom(); err != nil {
				return err
			}
			line, err := b.issues.Reopen(cmd.Context(), args[0], trimText(comment))
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), line)
			return nil
		},
	}
	cmd.Flags().StringVarP(&comment, "comment", "c", "", "comment before reopening")
	return cmd
}

func newDelete() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an issue: hidden everywhere, kept in history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			if _, err := b.People.Whom(); err != nil {
				return err
			}
			is, err := b.issues.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, fmt.Sprintf("delete %s %q?", shortID(is.ID), is.Headline())); err != nil {
				return err
			}
			if _, err := b.issues.Delete(cmd.Context(), is.ID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s %s\n", shortID(is.ID), is.Headline())
			fmt.Fprintln(cmd.ErrOrStderr(), "note: "+keptInHistory)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "don't ask")
	return cmd
}

func newUndo() *cobra.Command {
	var force bool
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "undo [<id>]",
		Short: "Undo your last change to an issue",
		Long: `Undo the last change you made to an issue, by writing its inverse: a
label added is removed and one removed added back, a status, a title or
a text goes back to what it was, a new comment is deleted. With no id,
your last change to any issue, by its time.

A change is what one command wrote: git board edit -e changes the title
and the description at once, and undo puts both back. Undoing twice
redoes, but for a new comment: its undo deletes it, and a delete is
final (docs/adr/0019), so the second undo refuses; post the text again
with git board comment. Changes a sync pulled from a forge aren't yours
to undo here.

It refuses, exit code 2, what it can't undo, and says why: creating an
issue (git board delete deletes it), a delete (a comment's undo's too),
a change of metadata. It
refuses too when someone else wrote to the issue after you, or a sync
pulled into it: --force undoes yours anyway, over theirs.`,
		Example: `  git board undo a1b2c3d
  git board undo                     # your last change, on any issue
  git board undo a1b2c3d --force     # though someone wrote after you`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			u, err := b.issues.Undo(cmd.Context(), name, force)
			if errors.Is(err, issues.ErrCantUndo) || errors.Is(err, issues.ErrWroteAfter) {
				return usageError{err}
			}
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if js.on() {
				return js.write(out, u)
			}
			fmt.Fprintf(out, "undone on %s %s:\n", shortID(u.Issue), u.Title)
			for _, w := range u.What {
				fmt.Fprintln(out, "  "+w)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "undo though someone else wrote to the issue after you")
	js = addJSON(cmd, `print {"issue", "title", "undone", "what"}`, jsonFields(issues.Undone{}), true)
	return cmd
}
