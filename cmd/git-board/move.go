package main

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/core/fields"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

func newMoveCmd() *cobra.Command {
	var next, prev bool

	cmd := &cobra.Command{
		Use:   "move <id> <status> | move <id> --next|--prev",
		Short: "Move an issue's status: to a status, or to the next or the previous option",
		Long: `Move an issue's status. "move <id> <status>" is "set <id> status=<status>":
the status is checked against [fields.status] options. --next and --prev
step through those options instead. As set does, the last option closes
the issue and another one reopens it.`,
		Example: `  git-board move a1b2c3 in-progress
  git-board move a1b2c3 --next
  git-board move a1b2c3 --prev`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				if next || prev {
					return usageError{fmt.Errorf("a status, or --next or --prev: one of them")}
				}
				if strings.ContainsAny(args[1], "=") || args[1] == "" {
					return usageError{fmt.Errorf("move %s %q: a status option, as [fields.status] lists them", args[0], args[1])}
				}
				return runSet(cmd.OutOrStdout(), args[0], []string{"status=" + args[1]}, "", false)
			}
			if next == prev {
				return usageError{fmt.Errorf("a status, or --next or --prev")}
			}
			return runMove(cmd.OutOrStdout(), args[0], prev)
		},
	}
	cmd.Flags().BoolVar(&next, "next", false, "advance to the next status option")
	cmd.Flags().BoolVar(&prev, "prev", false, "go back to the previous status option")
	return cmd
}

func runMove(out io.Writer, idPrefix string, back bool) error {
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

	step := nextStatusOption
	if back {
		step = prevStatusOption
	}
	next, err := step(cfg, currentStatusValue(labels))
	if err != nil {
		return err
	}

	label, err := fields.Apply(b, cfg.IssueSchema(), idPrefix, "status", next, "", time.Now())
	if err != nil {
		return err
	}
	// as set does: onto the last option closes, back from it reopens
	state, err := statusInStep(b, cfg, idPrefix, []fields.Assignment{{Field: "status", Value: next}}, []string{label})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s: %s%s\n", idPrefix, label, state)
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

// prevStatusOption is nextStatusOption's mirror: the option before
// current; an error at the first one, or with none set.
func prevStatusOption(cfg *config.Config, current string) (string, error) {
	field, ok := cfg.Fields["status"]
	if !ok || len(field.Options) == 0 {
		return "", fmt.Errorf("no [fields.status] options configured in .git-board.toml")
	}
	i := slices.Index(field.Options, current)
	switch {
	case current == "":
		return "", fmt.Errorf("no status set: nothing to go back from")
	case i < 0:
		return "", fmt.Errorf("current status %q doesn't match any configured [fields.status] option", current)
	case i == 0:
		return "", fmt.Errorf("status %q is already the first option: nothing to go back to", current)
	}
	return field.Options[i-1], nil
}
