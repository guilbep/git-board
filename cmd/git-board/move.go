package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/gitbugio"
)

func newMoveCmd() *cobra.Command {
	var next bool

	cmd := &cobra.Command{
		Use:     "move <id>",
		Short:   "Advance an issue's status to the next configured option",
		Example: `  git-board move a1b2c3 --next`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !next {
				return fmt.Errorf("specify --next (the only supported direction for now)")
			}
			return runMove(cmd.OutOrStdout(), args[0])
		},
	}
	cmd.Flags().BoolVar(&next, "next", false, "advance to the next status option")
	return cmd
}

func runMove(out io.Writer, idPrefix string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	b, err := gitbugio.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()

	labels, err := b.GetLabels(idPrefix)
	if err != nil {
		return err
	}

	next, err := nextStatusOption(cfg, currentStatusValue(labels))
	if err != nil {
		return err
	}

	label, err := fields.Apply(b, cfg, idPrefix, "status", next, "", time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s: %s\n", idPrefix, label)
	return nil
}

// currentStatusValue extracts the current status:* label's value, or ""
// if none is set.
func currentStatusValue(labels []string) string {
	for _, l := range labels {
		if v, ok := strings.CutPrefix(l, "status:"); ok {
			return v
		}
	}
	return ""
}

// nextStatusOption returns the configured status option after current in
// [fields.status].options order. An unset current status advances to the
// first option (starting the pipeline), matching the intuitive reading of
// "move to the next step" for an issue that hasn't been triaged yet. No
// wrap-around: advancing past the last option is an error, not a silent
// jump back to the first.
func nextStatusOption(cfg *config.Config, current string) (string, error) {
	field, ok := cfg.Fields["status"]
	if !ok || len(field.Options) == 0 {
		return "", fmt.Errorf("no [fields.status] options configured in .git-board.toml")
	}
	if current == "" {
		return field.Options[0], nil
	}
	for i, opt := range field.Options {
		if opt == current {
			if i+1 >= len(field.Options) {
				return "", fmt.Errorf("status %q is already the last option (%s) — nothing to advance to", current, opt)
			}
			return field.Options[i+1], nil
		}
	}
	return "", fmt.Errorf("current status %q doesn't match any configured [fields.status] option", current)
}
