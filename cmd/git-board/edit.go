package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/store"
)

func newEditCmd() *cobra.Command {
	var text textFlags
	var title string

	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit an issue's title or description",
		Long: `Edit an issue's title (--title) or description (--body, --body-file,
- for stdin, or --editor), as the person you write as. --editor opens
the title on the first line and the description below it, as gh's -e.

The edit is refused, exit code 5, if someone changed the title or the
description since you read them: your text is kept in a file the error
names. Every earlier version stays in the issue's history (git board log).
Fields are set's (git board set), comments comment's.`,
		Example: `  git board edit a1b2c3d --title "Login fails on Safari 17"
  git board edit a1b2c3d -b "Steps: ..."
  git board edit a1b2c3d -F description.md
  git board edit a1b2c3d -e`,
		Args: cobra.ExactArgs(1),
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
			return runEdit(cmd, args[0], title, setTitle, &text)
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "the new title")
	text.add(cmd, "description")
	return cmd
}

func runEdit(cmd *cobra.Command, idPrefix, title string, setTitle bool, text *textFlags) error {
	out := cmd.OutOrStdout()
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
	desc := th.Events[0] // created: the description
	body, setBody := desc.Body, text.given(cmd)

	switch {
	case text.editor:
		buf, err := runEditor("edit", th.Title+"\n\n"+desc.Body+"\n")
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

	var changed []string
	if setTitle && title != th.Title {
		now, err := b.GetIssue(th.ID)
		if err != nil {
			return err
		}
		if now.Title != th.Title {
			return keptErr(store.ErrChangedMeanwhile, "edit", title+"\n\n"+body)
		}
		if err := b.SetTitle(th.ID, title); err != nil {
			return keptErr(err, "edit", title+"\n\n"+body)
		}
		changed = append(changed, "title")
	}
	if setBody && body != desc.Body {
		if err := b.EditComment(th.ID, desc.ID, body, desc.Body); err != nil {
			return keptErr(err, "edit", title+"\n\n"+body)
		}
		changed = append(changed, "description")
	}
	if len(changed) == 0 {
		fmt.Fprintf(out, "%s: unchanged\n", idPrefix)
		return nil
	}
	fmt.Fprintf(out, "%s: %s edited\n", idPrefix, strings.Join(changed, " and "))
	return nil
}
