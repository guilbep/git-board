package cli

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/people"
)

func newLabels() *cobra.Command {
	var plain bool
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "labels",
		Short: "List the labels and field values in use, with counts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := open(cmd, false)
			if err != nil {
				return err
			}
			defer b.Close()
			counts, err := b.issues.Labels(cmd.Context())
			if err != nil {
				return err
			}
			if plain {
				counts = slices.DeleteFunc(counts, func(c issues.LabelCount) bool { return c.Field != "" })
			}
			out := cmd.OutOrStdout()
			if js.on() {
				return js.write(out, counts)
			}
			if len(counts) == 0 {
				fmt.Fprintln(out, "no labels in use")
				return nil
			}
			rows := [][]string{{"LABEL", "OPEN", "CLOSED"}}
			for _, c := range counts {
				rows = append(rows, []string{c.Label, fmt.Sprint(c.Open), fmt.Sprint(c.Closed)})
			}
			writeColumns(out, rows)
			return nil
		},
	}
	js = addJSON(cmd, "print the labels as JSON", jsonFields(issues.LabelCount{}), true)
	cmd.Flags().BoolVar(&plain, "plain", false, "only the plain labels, no field values")
	return cmd
}

// meRow is a person as me --ls --json prints them.
type meRow struct {
	people.Who
	Me bool `json:"me,omitempty"`
}

func personLine(p people.Who) string {
	s := p.Name + " (" + shortID(string(p.ID))
	if p.Login != "" {
		s += ", @" + p.Login
	}
	return s + ")"
}

func newMe() *cobra.Command {
	var ls, create bool
	var name, email string
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "me [person-id]",
		Short: "Show or choose who you write as",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case create && (len(args) > 0 || ls):
				return usageError{fmt.Errorf("--new takes no person id, nor --ls")}
			case !create && (cmd.Flags().Changed("name") || cmd.Flags().Changed("email")):
				return usageError{fmt.Errorf("--name and --email go with --new")}
			case ls && len(args) > 0:
				return usageError{fmt.Errorf("--ls takes no person id")}
			}
			ctx := cmd.Context()
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			out := cmd.OutOrStdout()
			switch {
			case create:
				if name == "" {
					name = b.GitConfig(ctx, "user.name")
				}
				if email == "" {
					email = b.GitConfig(ctx, "user.email")
				}
				p, err := b.People.New(ctx, name, email)
				if err != nil {
					return err
				}
				if js.on() {
					return js.write(out, p)
				}
				fmt.Fprintln(out, "you write as", personLine(p))
				return nil
			case ls:
				all, err := b.People.List(ctx)
				if err != nil {
					return err
				}
				if js.on() {
					rows := make([]meRow, 0, len(all))
					for _, p := range all {
						rows = append(rows, meRow{p, p.ID == b.People.Me})
					}
					return js.write(out, rows)
				}
				rows := [][]string{{"", "id", "name", "login"}}
				for _, p := range all {
					mark := ""
					if p.ID == b.People.Me {
						mark = "*"
					}
					rows = append(rows, []string{mark, shortID(string(p.ID)), p.Name, p.Login})
				}
				writeColumns(out, rows)
				return nil
			case len(args) == 1:
				if err := b.People.Adopt(ctx, args[0]); err != nil {
					return err
				}
			}
			p, ok, err := b.People.WhoAmI(ctx)
			if err != nil {
				return err
			}
			if !ok {
				return people.ErrNoMe
			}
			if js.on() {
				return js.write(out, p)
			}
			fmt.Fprintln(out, personLine(p))
			return nil
		},
	}
	js = addJSON(cmd, "render as JSON", jsonFields(meRow{}), true)
	cmd.Flags().BoolVar(&ls, "ls", false, "list everyone on the board")
	cmd.Flags().BoolVar(&create, "new", false, "create a person and write as them")
	cmd.Flags().StringVar(&name, "name", "", "with --new: the name (default: git config user.name)")
	cmd.Flags().StringVar(&email, "email", "", "with --new: the email (default: git config user.email)")
	return cmd
}
