package main

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/store"
)

// labelCount is a label in use and how many issues have it.
type labelCount struct {
	Label  string `json:"label"`
	Field  string `json:"field,omitempty"` // status for status:ready; "" for a plain label
	Value  string `json:"value"`
	Open   int    `json:"open"`
	Closed int    `json:"closed"`
}

func newLabelsCmd() *cobra.Command {
	var plain bool
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "labels",
		Short: "List the labels and field values in use, with counts",
		Long: `List every label on the board's issues, with how many open and closed
issues have it: the plain labels (bug) first, then each field's values
(status:ready), by field. A label counts once per issue. Deleted issues
don't count.

--plain lists the plain labels alone. --json prints [{"label", "field",
"value", "open", "closed"}].

git board labels rename <old> <new> renames a label on every issue that
has it, one change each.`,
		Example: `  git board labels
  git board labels --plain
  git board labels rename regresion regression
  git board labels rename prio:p0 prio:p1`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := store.OpenCwd()
			if err != nil {
				return err
			}
			defer b.Close()
			issues, err := b.ListIssues()
			if err != nil {
				return err
			}
			counts := countLabels(issues)
			if plain {
				counts = slices.DeleteFunc(counts, func(c labelCount) bool { return c.Field != "" })
			}
			if js.on() {
				return js.write(cmd.OutOrStdout(), counts)
			}
			return printLabels(cmd.OutOrStdout(), counts)
		},
	}
	js = addJSON(cmd, "print the labels as JSON", jsonFields(labelCount{}), true)
	cmd.Flags().BoolVar(&plain, "plain", false, "only the plain labels, no field values")
	cmd.AddCommand(newLabelsRenameCmd())
	return cmd
}

// countLabels counts each label over the issues: plain ones first, then by
// field and value.
func countLabels(issues []store.Issue) []labelCount {
	byLabel := map[string]*labelCount{}
	for _, is := range issues {
		for _, l := range is.Labels {
			c := byLabel[l]
			if c == nil {
				c = &labelCount{Label: l, Value: l}
				if f, v, ok := strings.Cut(l, ":"); ok {
					c.Field, c.Value = f, v
				}
				byLabel[l] = c
			}
			if is.Status == "closed" {
				c.Closed++
			} else {
				c.Open++
			}
		}
	}
	out := make([]labelCount, 0, len(byLabel))
	for _, c := range byLabel {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b labelCount) int {
		return cmp.Or(strings.Compare(a.Field, b.Field), strings.Compare(a.Value, b.Value)) // "": plain first
	})
	return out
}

func printLabels(out io.Writer, counts []labelCount) error {
	if len(counts) == 0 {
		fmt.Fprintln(out, "no labels in use")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "LABEL\tOPEN\tCLOSED")
	for _, c := range counts {
		fmt.Fprintf(w, "%s\t%d\t%d\n", c.Label, c.Open, c.Closed)
	}
	return w.Flush()
}

func newLabelsRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Rename a label on every issue that has it",
		Long: `Rename a label on every issue that has it, as the person you write as:
one change per issue, which removes old and adds new. An issue that has
both keeps new. A field's value is a label too: prio:p0 prio:p1.

It doesn't check new against .git-board.toml: git board lint does.`,
		Example: `  git board labels rename regresion regression`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			old, new := args[0], args[1]
			switch {
			case old == new:
				return usageError{errors.New("old and new are the same label")}
			case strings.TrimSpace(new) != new || new == "" || strings.ContainsAny(new, "\n\r"):
				return usageError{fmt.Errorf("%q: a label is a name with no space around it and no line break", new)}
			}
			b, err := store.OpenCwd()
			if err != nil {
				return err
			}
			defer b.Close()
			if err := needMe(b); err != nil {
				return err
			}
			issues, err := b.ListIssues()
			if err != nil {
				return err
			}
			n := 0
			for _, is := range issues {
				if !slices.Contains(is.Labels, old) {
					continue
				}
				if _, err := b.ApplyLabelChange(is.ID, []string{new}, []string{old}); err != nil {
					return fmt.Errorf("%s: %w (%d renamed before it)", shortID(is.ID), err, n)
				}
				n++
			}
			if n == 0 {
				return fmt.Errorf("%w: no issue has the label %q", store.ErrNotFound, old)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s → %s on %d issue%s\n", old, new, n, plural(n))
			return nil
		},
	}
}
