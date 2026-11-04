package main

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/store"
)

// The reasons an issue closes for, as GitHub's state_reason: completed
// (the default) leaves no label; the others are closed-as:<reason>
// (docs/adr/0007: every field is a label).
const closedAs = "closed-as"

var closeReasons = []string{"completed", "not-planned", "duplicate"}

// closeReason reads --reason as gh does too: "not planned", not_planned.
func closeReason(s string) (string, error) {
	r := strings.ToLower(strings.NewReplacer(" ", "-", "_", "-").Replace(strings.TrimSpace(s)))
	if r == "" {
		return "completed", nil
	}
	if !slices.Contains(closeReasons, r) {
		return "", usageError{fmt.Errorf("--reason %q: one of %s", s, strings.Join(closeReasons, ", "))}
	}
	return r, nil
}

func newCloseCmd() *cobra.Command {
	var comment, reason, duplicateOf string
	cmd := &cobra.Command{
		Use:   "close <id>",
		Short: "Set status to the terminal option and close the issue",
		Long: `Set status to the last [fields.status] option (the terminal one,
whatever .git-board.toml calls it) and close the issue. --comment adds a
comment first, as gh issue close -c does. "git board reopen" undoes it.

--reason says why, as GitHub's: completed (the default), not-planned or
duplicate. A reason but completed is the label closed-as:<reason>, so
ls "closed-as:not-planned" finds them; completed leaves none.

--duplicate-of <id> closes it as a duplicate of that issue: it sets
closed-as:duplicate and the field duplicate-of to the other issue (its
forge number, #12, when both are on that forge, else its id), and
comments "Duplicate of #12", which GitHub reads too.`,
		Example: `  git-board close a1b2c3
  git-board close a1b2c3 -c "Fixed in 4e5f6a7"
  git-board close a1b2c3 --reason not-planned -c "Out of scope"
  git-board close a1b2c3 --duplicate-of "#2518"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := closeReason(reason)
			if err != nil {
				return err
			}
			if duplicateOf != "" {
				if cmd.Flags().Changed("reason") && r != "duplicate" {
					return usageError{fmt.Errorf("--duplicate-of closes as a duplicate, not --reason %s", r)}
				}
				r = "duplicate"
			}
			return runClose(cmd.OutOrStdout(), args[0], comment, r, duplicateOf)
		},
	}
	cmd.Flags().StringVarP(&comment, "comment", "c", "", "comment before closing")
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "why: completed, not-planned or duplicate")
	cmd.Flags().StringVar(&duplicateOf, "duplicate-of", "", "close as a duplicate of this issue")
	return cmd
}

func runClose(out io.Writer, idPrefix, comment, reason, duplicateOf string) error {
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

	// what the reason writes, worked out before anything is
	is, err := b.GetIssue(idPrefix)
	if err != nil {
		return err
	}
	var added, removed []string
	for _, l := range is.Labels {
		if strings.HasPrefix(l, closedAs+":") {
			removed = append(removed, l)
		}
	}
	if reason != "completed" {
		added = append(added, closedAs+":"+reason)
	}
	dupNote := ""
	if duplicateOf != "" {
		other, err := b.GetIssue(duplicateOf)
		if err != nil {
			return fmt.Errorf("--duplicate-of %s: %w", duplicateOf, err)
		}
		if other.ID == is.ID {
			return usageError{fmt.Errorf("--duplicate-of %s: that is the issue itself", duplicateOf)}
		}
		ref := issueRefFrom(is, other)
		added = append(added, "duplicate-of:"+ref)
		dupNote = "Duplicate of " + ref
	}
	added = slices.DeleteFunc(added, func(l string) bool { return slices.Contains(is.Labels, l) })
	removed = slices.DeleteFunc(removed, func(l string) bool { return slices.Contains(added, l) || l == closedAs+":"+reason })

	if err := addComment(b, is.ID, comment); err != nil {
		return err
	}
	if err := addComment(b, is.ID, dupNote); err != nil {
		return err
	}
	label, err := fields.Apply(b, cfg, is.ID, "status", terminal, "", time.Now())
	if err != nil {
		return err
	}
	if len(added)+len(removed) > 0 {
		if _, err := b.ApplyLabelChange(is.ID, added, removed); err != nil {
			return err
		}
	}
	if err := b.CloseBug(is.ID); err != nil {
		return err
	}
	said := label
	for _, l := range added {
		said += ", " + l
	}
	fmt.Fprintf(out, "%s: %s, closed\n", idPrefix, said)
	return nil
}

// issueRefFrom is how is names other in an issue field: other's number
// on a forge both are on (#12), else its id's first 8 characters.
func issueRefFrom(is, other store.Issue) string {
	for _, f := range other.Forges {
		if f.Number != "" && slices.ContainsFunc(is.Forges, func(g store.Forge) bool { return g.Name == f.Name }) {
			return "#" + f.Number
		}
	}
	return shortID(other.ID)
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
The closed-as label close --reason wrote goes; a duplicate-of stays.
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
	// why it closed is over: closed-as goes (a duplicate-of stays, a link)
	var reasons []string
	for _, l := range labels {
		if strings.HasPrefix(l, closedAs+":") {
			reasons = append(reasons, l)
		}
	}
	if len(reasons) > 0 {
		if _, err := b.ApplyLabelChange(idPrefix, nil, reasons); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "%s: %sopen\n", idPrefix, said)
	return nil
}
