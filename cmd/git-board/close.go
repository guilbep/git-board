package main

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/store"
)

func newCloseCmd() *cobra.Command {
	var comment string
	cmd := &cobra.Command{
		Use:   "close <id>",
		Short: "Set status to the terminal option and close the issue",
		Long: `Set status to the last [fields.status] option (the terminal one,
whatever .git-board.toml calls it) and close the issue. --comment adds a
comment first, as gh issue close -c does. "git board reopen" undoes it.`,
		Example: `  git-board close a1b2c3
  git-board close a1b2c3 -c "Fixed in 4e5f6a7"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClose(cmd.OutOrStdout(), args[0], comment)
		},
	}
	cmd.Flags().StringVarP(&comment, "comment", "c", "", "comment before closing")
	return cmd
}

func runClose(out io.Writer, idPrefix, comment string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	field, ok := cfg.Fields["status"]
	if !ok || len(field.Options) == 0 {
		return fmt.Errorf("no [fields.status] options configured in .git-board.toml")
	}
	// The last configured option is the terminal state, whatever it's
	// actually called ("validated" in the PRD's example config) — not
	// hardcoded, since a project's own .git-board.toml owns that name.
	terminal := field.Options[len(field.Options)-1]

	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()

	if err := addComment(b, idPrefix, comment); err != nil {
		return err
	}
	label, err := fields.Apply(b, cfg, idPrefix, "status", terminal, "", time.Now())
	if err != nil {
		return err
	}
	if err := b.CloseBug(idPrefix); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s: %s, closed\n", idPrefix, label)
	return nil
}

// addComment adds a -c comment, when there is one.
func addComment(b *store.Backend, idPrefix, comment string) error {
	if comment = trimText(comment); comment == "" {
		return nil
	}
	_, err := b.AddComment(idPrefix, comment)
	return err
}

func newReopenCmd() *cobra.Command {
	var comment string
	cmd := &cobra.Command{
		Use:   "reopen <id>",
		Short: "Reopen a closed issue",
		Long: `Reopen an issue: the mirror of "git board close". It opens the issue
and, when its status is the terminal option (the last [fields.status]
option) or none, sets it to the first option. Another status stays.
--comment adds a comment first.`,
		Example: `  git board reopen a1b2c3d
  git board reopen a1b2c3d -c "Still fails on Safari"
  git board reopen a1b2c3d && git board set a1b2c3d status=ready`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReopen(cmd.OutOrStdout(), args[0], comment)
		},
	}
	cmd.Flags().StringVarP(&comment, "comment", "c", "", "comment before reopening")
	return cmd
}

func runReopen(out io.Writer, idPrefix, comment string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()

	labels, err := b.GetLabels(idPrefix)
	if err != nil {
		return err
	}
	if err := addComment(b, idPrefix, comment); err != nil {
		return err
	}
	said := ""
	if options := cfg.Fields["status"].Options; len(options) > 0 {
		status := currentStatusValue(labels)
		if status == "" || status == options[len(options)-1] {
			label, err := fields.Apply(b, cfg, idPrefix, "status", options[0], "", time.Now())
			if err != nil {
				return err
			}
			said = label + ", "
		}
	}
	if err := b.SetStatus(idPrefix, "open"); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s: %sopen\n", idPrefix, said)
	return nil
}
