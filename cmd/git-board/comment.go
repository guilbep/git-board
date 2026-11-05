package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/internal/store"
)

func newCommentCmd() *cobra.Command {
	var text textFlags
	var edit, del string
	var editLast, yes bool
	var js *jsonOut

	cmd := &cobra.Command{
		Use:   "comment <id>",
		Short: "Comment on an issue, or edit a comment",
		Long: `Comment on an issue, as the person you write as (git board me). The
text comes from --body, --body-file (- for stdin) or --editor; with none
of them, the editor opens, as git commit's does. An empty text writes
nothing.

--edit <ref> edits a comment instead, by the ref show prints
([abc1234:1]; a prefix of its hash is enough when it names one comment),
and --edit-last your last comment on the issue. The editor opens on the
comment's text. The edit is refused, exit code 5, if someone changed the
comment since you read it: your text is kept in a file the error names,
nothing is lost.

--delete <ref> deletes a comment: no view, search, export or sync shows
it again. Its text stays in the board's history, and on a forge it was
synced to (docs/adr/0019): revoke a leaked secret. On a terminal it asks
first; --yes doesn't ask, and is needed when no one is at a terminal.

--json prints {"id", "ref", "issue"}: the comment's whole id, its short
ref, and the issue's id.`,
		Example: `  git board comment a1b2c3d -b "Fixed in 4e5f6a7"
  git board comment a1b2c3d                    # in the editor
  git board comment a1b2c3d -F notes.md
  git log -1 --format=%B | git board comment a1b2c3d -F -
  git board comment a1b2c3d --edit-last
  git board comment a1b2c3d --edit 9c8b7a6:1 -b "Fixed in 4e5f6a7, released in v1.2"
  git board comment a1b2c3d --delete 9c8b7a6:1 --yes`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if edit != "" && editLast {
				return usageError{fmt.Errorf("--edit or --edit-last, not both")}
			}
			if del != "" {
				if edit != "" || editLast || text.given(cmd) {
					return usageError{fmt.Errorf("--delete takes no text, nor --edit")}
				}
				return runCommentDelete(cmd, args[0], del, yes, js)
			}
			if yes {
				return usageError{fmt.Errorf("--yes goes with --delete")}
			}
			return runComment(cmd, args[0], &text, edit, editLast, js)
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

// runCommentDelete deletes the comment ref names on the issue.
func runCommentDelete(cmd *cobra.Command, idPrefix, ref string, yes bool, js *jsonOut) error {
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()
	if err := needMe(b); err != nil {
		return err
	}
	th, err := b.Thread(idPrefix)
	if err != nil {
		return err
	}
	target, err := findComment(th, ref, "")
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
	if err := b.DeleteComment(th.ID, target.ID); err != nil {
		return err
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "note: "+keptInHistory)
	if js.on() {
		return js.write(cmd.OutOrStdout(), map[string]any{"id": target.ID, "ref": commentRef(target.ID), "issue": th.ID, "deleted": true})
	}
	fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", commentRef(target.ID))
	return nil
}

func runComment(cmd *cobra.Command, idPrefix string, text *textFlags, edit string, editLast bool, js *jsonOut) error {
	out := cmd.OutOrStdout()
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()
	me, ok, err := b.Me()
	if err == nil && !ok {
		err = store.ErrNoMe
	}
	if err != nil {
		return err
	}
	th, err := b.Thread(idPrefix)
	if err != nil {
		return err
	}

	var target *model.Event
	if edit != "" || editLast {
		if target, err = findComment(th, edit, me.ID); err != nil {
			return err
		}
	}
	initial := ""
	if target != nil {
		initial = target.Body + "\n"
	}
	body, err := commentText(cmd, text, initial)
	if err != nil {
		return err
	}

	id := ""
	if target == nil {
		if id, err = b.AddComment(th.ID, body); err != nil {
			return keptErr(err, "comment", body)
		}
	} else {
		id = target.ID
		if err := b.EditComment(th.ID, id, body, target.Body); err != nil {
			return keptErr(err, "comment", body)
		}
	}
	if js.on() {
		return js.write(out, map[string]string{"id": id, "ref": commentRef(id), "issue": th.ID})
	}
	fmt.Fprintln(out, commentRef(id))
	return nil
}

// commentText is the text the flags give, or the editor's when none does;
// empty, nothing to write.
func commentText(cmd *cobra.Command, text *textFlags, initial string) (string, error) {
	var body string
	var err error
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
	return body, err
}

// findComment is the comment ref names on th (show's abc1234:1, a prefix of
// its hash, or its whole id), or, ref empty, the last one meID wrote.
func findComment(th model.Thread, ref, meID string) (*model.Event, error) {
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
		return nil, fmt.Errorf("%w: comment %s, on %d comments", store.ErrAmbiguous, ref, len(found))
	case ref == "":
		return nil, fmt.Errorf("%w: you haven't commented on %s", store.ErrNoSuchComment, shortID(th.ID))
	}
	rh, _, _ := strings.Cut(ref, ":")
	if desc, _, _ := strings.Cut(th.Events[0].ID, ":"); rh != "" && strings.HasPrefix(desc, rh) {
		return nil, errors.New("that is the description: git board edit changes it")
	}
	return nil, fmt.Errorf("%w: %s (git board show %s lists them)", store.ErrNoSuchComment, ref, shortID(th.ID))
}

// commentRef is a text's id as show prints it and comment --edit takes
// it: the first 7 characters of its operation's hash, then its index
// (abc1234:1).
func commentRef(id string) string {
	hash, n, ok := strings.Cut(id, ":")
	if !ok {
		return shortID(id)
	}
	return hash[:min(7, len(hash))] + ":" + n
}
