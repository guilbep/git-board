package main

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/gitbugio"
)

func newCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "close <id>",
		Short:   "Set status to the terminal option and close the issue",
		Example: `  git-board close a1b2c3`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClose(cmd.OutOrStdout(), args[0])
		},
	}
}

func runClose(out io.Writer, idPrefix string) error {
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

	b, err := gitbugio.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()

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
