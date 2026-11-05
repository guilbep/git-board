package cli

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/kernel"
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
	var title, me string
	var text textFlags
	var force, noPrompt bool
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "new [title] [field=value...]",
		Short: "Create an issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("title") && len(args) > 0 {
				if _, err := issues.ParseAssignment(args[0]); err != nil {
					title, args = args[0], args[1:]
				}
			}
			_ = noPrompt // M2: no prompt yet; a title is needed
			if title == "" && !text.editor {
				return usageError{fmt.Errorf("a title: git board new <title>, or --editor")}
			}
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			if _, err := b.People.Whom(); err != nil {
				return err
			}
			body := ""
			switch {
			case text.editor:
				buf, err := runEditor("new", title+"\n\n")
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
			}
			is, err := b.issues.New(cmd.Context(), issues.NewReq{Title: title, Body: body, Assign: args, Force: force, MeName: resolveMe(me)})
			if err != nil {
				if text.editor || text.given(cmd) {
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
			ctx := cmd.Context()
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			me, err := b.People.Whom()
			if err != nil {
				return err
			}
			th, err := b.issues.Show(ctx, args[0])
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
	js = addJSON(cmd, "print the comment's id as JSON", []string{"id", "ref", "issue"}, false)
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
			ctx := cmd.Context()
			b, err := open(cmd, true)
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
	var me string
	var force bool
	cmd := &cobra.Command{
		Use:   "set <id> field=value [field=value...]",
		Short: "Set one or more fields on an issue",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			if _, err := b.People.Whom(); err != nil {
				return err
			}
			lines, err := b.issues.Set(cmd.Context(), issues.SetReq{Name: args[0], Assign: args[1:], Force: force, MeName: resolveMe(me)})
			if err != nil {
				return err
			}
			for _, l := range lines {
				fmt.Fprintln(cmd.OutOrStdout(), l)
			}
			return nil
		},
	}
	meFlag(cmd, &me)
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
