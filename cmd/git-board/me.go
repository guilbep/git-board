package main

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/store"
)

func newMeCmd() *cobra.Command {
	var ls, create bool
	var js *jsonOut
	var name, email string

	cmd := &cobra.Command{
		Use:   "me [person-id]",
		Short: "Show or choose who you write as",
		Long: `Show or choose who you write as: the person every issue, comment and
change you make is signed with. A board has no one chosen until you do,
and every write fails until then.

With no argument, it prints who you write as. --new creates a person,
from your git user.name and user.email unless --name and --email say
otherwise, and writes as them. A person id (a prefix of it, as --ls shows
it) writes as someone already on the board: who a forge sync made of
your account, say. The choice is kept in this clone's git config
(board.me).

--json prints {"id", "name", "login", "avatar_url"} (login and avatar_url
left out when empty); with --ls, an array of them, each with "me": true
for you.`,
		Example: `  git board me                    # who you write as
  git board me --new              # create you from git user.name/user.email
  git board me --new --name "Ada Lovelace" --email ada@example.com
  git board me --ls               # everyone on the board
  git board me 3f2a9c1            # write as this person`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			switch {
			case create && (len(args) > 0 || ls):
				return usageError{fmt.Errorf("--new takes no person id, nor --ls")}
			case !create && (cmd.Flags().Changed("name") || cmd.Flags().Changed("email")):
				return usageError{fmt.Errorf("--name and --email go with --new")}
			case ls && len(args) > 0:
				return usageError{fmt.Errorf("--ls takes no person id")}
			}
			b, err := store.OpenCwd()
			if err != nil {
				return err
			}
			defer b.Close()
			switch {
			case create:
				return runMeNew(out, b, name, email, js)
			case ls:
				return runMeLs(out, b, js)
			case len(args) == 1:
				if err := b.Adopt(args[0]); err != nil {
					return err
				}
			}
			return runMe(out, b, js)
		},
	}
	js = addJSON(cmd, "render as JSON", jsonFields(meRow{}), true)
	cmd.Flags().BoolVar(&ls, "ls", false, "list everyone on the board")
	cmd.Flags().BoolVar(&create, "new", false, "create a person and write as them")
	cmd.Flags().StringVar(&name, "name", "", "with --new: the name (default: git config user.name)")
	cmd.Flags().StringVar(&email, "email", "", "with --new: the email (default: git config user.email)")
	return cmd
}

func runMe(out io.Writer, b *store.Backend, js *jsonOut) error {
	p, ok, err := b.Me()
	if err != nil {
		return err
	}
	if !ok {
		return store.ErrNoMe
	}
	if js.on() {
		return js.write(out, p)
	}
	fmt.Fprintln(out, personLine(p))
	return nil
}

// meRow is a person as me --ls --json prints them.
type meRow struct {
	store.Person
	Me bool `json:"me,omitempty"`
}

// personLine is a person as me prints them: name, short id, login.
func personLine(p store.Person) string {
	s := p.Name + " (" + shortID(p.ID)
	if p.Login != "" {
		s += ", @" + p.Login
	}
	return s + ")"
}

func runMeNew(out io.Writer, b *store.Backend, name, email string, js *jsonOut) error {
	if name == "" {
		name = gitConfig("user.name")
	}
	if email == "" {
		email = gitConfig("user.email")
	}
	if name == "" {
		return fmt.Errorf("no name: pass --name, or set git config user.name")
	}
	persons, err := b.Persons()
	if err != nil {
		return err
	}
	for _, p := range persons {
		if p.Name == name && strings.EqualFold(p.Email, email) {
			return fmt.Errorf("%s <%s> is on the board already, as %s: git board me %s", name, email, shortID(p.ID), shortID(p.ID))
		}
	}
	p, err := b.NewIdentity(name, email)
	if err != nil {
		return err
	}
	if js.on() {
		return js.write(out, p)
	}
	fmt.Fprintln(out, "you write as", personLine(p))
	return nil
}

func runMeLs(out io.Writer, b *store.Backend, js *jsonOut) error {
	persons, err := b.Identities()
	if err != nil {
		return err
	}
	me, _, err := b.Me()
	if err != nil {
		return err
	}
	if js.on() {
		rows := make([]meRow, 0, len(persons))
		for _, p := range persons {
			rows = append(rows, meRow{p, p.ID == me.ID})
		}
		return js.write(out, rows)
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "\tid\tname\tlogin")
	for _, p := range persons {
		mark := ""
		if p.ID == me.ID {
			mark = "*"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", mark, shortID(p.ID), p.Name, p.Login)
	}
	return tw.Flush()
}

// gitConfig is a git config value, "" when unset.
func gitConfig(key string) string {
	out, err := exec.Command("git", "config", "--get", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
