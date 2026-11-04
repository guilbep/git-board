package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// forgeAuth adds status and login to the forge command (ADR 0010: tokens
// from the environment, else git's credential helper; git-board keeps
// none).
func forgeAuth(root *cobra.Command) {
	for _, c := range root.Commands() {
		if c.Name() == "forge" {
			c.AddCommand(newForgeStatusCmd(), newForgeLoginCmd())
		}
	}
}

// tokenStatus is one account's token on a forge: found or not, where, and,
// checked, whose the forge says it is. Never the token.
type tokenStatus struct {
	Login  string `json:"login"`            // the account it is looked up for; "" for any
	Role   string `json:"role"`             // posting (the forge's table's login) or you
	Found  bool   `json:"found"`            // a token was found
	Source string `json:"source,omitempty"` // an environment variable, or git's credential helper
	Owner  string `json:"owner,omitempty"`  // --check: the login the forge says the token is
	Error  string `json:"error,omitempty"`  // --check: what the forge answered instead
}

type forgeStatus struct {
	Name       string        `json:"name"`
	Kind       string        `json:"kind"`
	Host       string        `json:"host"`
	Project    string        `json:"project"`
	Sync       bool          `json:"sync"`
	Discovered bool          `json:"discovered,omitempty"` // found in the git remotes, not in .git-board.toml
	Tokens     []tokenStatus `json:"tokens"`
}

func newForgeStatusCmd() *cobra.Command {
	var check bool
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Which forges the board syncs with, and whether a token is found for each",
		Long: `List the forges: those of .git-board.toml, else those found in the git
remotes. For each, whether a token is found for its posting account (the
table's login) and for your own account on that host, and where: an
environment variable (GITHUB_TOKEN for github.com, GITLAB_TOKEN for
GITLAB_HOST), else git's credential helper. No token is ever printed.

Nothing goes to the network, unless --check: it asks each forge whose
each token is, and says so.`,
		Example: `  git board forge status
  git board forge status --check
  git board forge status --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			b, err := store.OpenCwd()
			if err != nil {
				return err
			}
			defer b.Close()
			forges, err := b.Forges(cfg, func(string) {})
			if err != nil {
				return err
			}
			list := forgeStatuses(cmd.Context(), b, forges, check)
			if js.on() {
				return js.write(cmd.OutOrStdout(), list)
			}
			printForgeStatus(cmd.OutOrStdout(), list)
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "ask each forge whose each token is (the network)")
	js = addJSON(cmd, "print the forges as JSON", jsonFields(forgeStatus{}), true)
	return cmd
}

// myLogin is your login on host: your person's account there; "" for none.
func myLogin(b *store.Backend, host string) string {
	me, ok, err := b.Me()
	if err != nil || !ok {
		return ""
	}
	p, err := people.Load(b.Datastore(), me.ID)
	if err != nil {
		return ""
	}
	for _, a := range p.Accounts {
		if a.Server == host {
			return a.Login
		}
	}
	return ""
}

// tokenSource is where a token for f comes from when the environment has
// one for its host: that variable's name; else git's credential helper.
func tokenSource(f config.Forge) string {
	switch {
	case f.Kind == "github" && f.Host == "github.com" && os.Getenv("GITHUB_TOKEN") != "":
		return "GITHUB_TOKEN"
	case f.Kind == "github" && f.Host == os.Getenv("GITHUB_ENTERPRISE_HOST") && os.Getenv("GITHUB_ENTERPRISE_TOKEN") != "":
		return "GITHUB_ENTERPRISE_TOKEN"
	case f.Kind == "gitlab" && f.Host == cmp.Or(os.Getenv("GITLAB_HOST"), "gitlab.com") && os.Getenv("GITLAB_TOKEN") != "":
		return "GITLAB_TOKEN"
	}
	return "git's credential helper"
}

func forgeStatuses(ctx context.Context, b *store.Backend, forges []config.Forge, check bool) []forgeStatus {
	out := []forgeStatus{}
	for _, f := range forges {
		st := forgeStatus{Name: f.Name, Kind: f.Kind, Host: f.Host, Project: cmp.Or(f.Path, f.Project), Sync: f.Sync, Discovered: f.Discovered}
		logins := [][2]string{{f.Login, "posting"}}
		if mine := myLogin(b, f.Host); mine != "" && mine != f.Login {
			logins = append(logins, [2]string{mine, "you"})
		}
		for _, l := range logins {
			ts := tokenStatus{Login: l[0], Role: l[1]}
			if _, err := b.Token(ctx, f, l[0]); err == nil {
				ts.Found, ts.Source = true, tokenSource(f)
				if check {
					g := f
					g.Login = l[0]
					if who, err := forgeMe(ctx, g); err != nil {
						ts.Error = err.Error()
					} else {
						ts.Owner = who
					}
				}
			}
			st.Tokens = append(st.Tokens, ts)
		}
		out = append(out, st)
	}
	return out
}

// forgeMe asks the forge whose f's token is: its login. Tests replace it.
var forgeMe = func(ctx context.Context, f config.Forge) (string, error) {
	opened, err := store.OpenForges(ctx, []config.Forge{f})
	if err != nil {
		return "", err
	}
	a, err := opened[0].Me(ctx)
	return a.Login, err
}

func printForgeStatus(out io.Writer, list []forgeStatus) {
	if len(list) == 0 {
		fmt.Fprintln(out, "no forge: none in .git-board.toml, and no GitHub or GitLab remote (git board init sets one up)")
		return
	}
	for _, f := range list {
		sync := "sync on"
		if !f.Sync {
			sync = "sync off"
		}
		from := ""
		if f.Discovered {
			from = ", found in the git remotes"
		}
		fmt.Fprintf(out, "%s: %s %s/%s (%s%s)\n", f.Name, f.Kind, f.Host, f.Project, sync, from)
		for _, t := range f.Tokens {
			who := t.Login
			if who == "" {
				who = "any login"
			}
			line := fmt.Sprintf("  %s token for %s: ", t.Role, who)
			switch {
			case !t.Found:
				line += fmt.Sprintf("none found: git board forge login %s", f.Name)
				if t.Login != "" {
					line += " --login " + t.Login
				}
			case t.Error != "":
				line += "found (" + t.Source + "), but the forge refused it: " + t.Error
			case t.Owner != "" && !strings.EqualFold(t.Owner, t.Login) && t.Login != "":
				line += "found (" + t.Source + "), but it is " + t.Owner + "'s"
			case t.Owner != "":
				line += "found (" + t.Source + "), it is " + t.Owner + "'s"
			default:
				line += "found (" + t.Source + ")"
			}
			fmt.Fprintln(out, line)
		}
	}
}

func newForgeLoginCmd() *cobra.Command {
	var login string
	cmd := &cobra.Command{
		Use:   "login <forge>",
		Short: "Store a forge token in git's credential helper",
		Long: `Store a token for a forge's host in git's credential helper, with git
credential approve: git-board keeps no token of its own (ADR 0010).

<forge> is a forge's name, as git board forge status lists it. The token
is for --login, by default your own account on that host, else the
forge's posting account. It is read at a hidden prompt on a terminal,
else from the first line of stdin, never from the command line, where
the shell's history and ps would show it.

git board forge status --check then asks the forge whose it is.`,
		Example: `  git board forge login github
  gh auth token | git board forge login github --login guilbep
  git board forge login epfl --login guilbep < token.txt`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			b, err := store.OpenCwd()
			if err != nil {
				return err
			}
			defer b.Close()
			forges, err := b.Forges(cfg, func(string) {})
			if err != nil {
				return err
			}
			i := slices.IndexFunc(forges, func(f config.Forge) bool { return f.Name == args[0] })
			if i < 0 {
				var names []string
				for _, f := range forges {
					names = append(names, f.Name)
				}
				return fmt.Errorf("%w: forge %q (the forges: %s)", store.ErrNotFound, args[0], cmp.Or(strings.Join(names, ", "), "none"))
			}
			f := forges[i]
			who := cmp.Or(login, myLogin(b, f.Host), f.Login)
			if who == "" {
				return usageError{fmt.Errorf("whose token? --login <your login on %s>", f.Host)}
			}
			token, err := readSecret(cmd, fmt.Sprintf("token for %s on %s: ", who, f.Host))
			if err != nil {
				return err
			}
			if token == "" {
				return usageError{errors.New("no token read: type it at the prompt, or give it on stdin")}
			}
			if err := b.StoreToken(cmd.Context(), f, who, token); err != nil {
				return fmt.Errorf("storing the token with git credential approve: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "stored a token for %s on %s in git's credential helper; git board forge status --check asks %s whose it is\n", who, f.Host, f.Host)
			return nil
		},
	}
	cmd.Flags().StringVar(&login, "login", "", "the account the token is for (default: yours on that host, else the forge's posting account)")
	return cmd
}
