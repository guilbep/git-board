package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/forge"
)

// forge status and forge login: the forges' tokens (ADR 0010: from the
// environment, else git's credential helper; git-board keeps none), as
// v1's.

func newForgeStatus() *cobra.Command {
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
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			list, err := b.ForgeStatuses(cmd.Context(), check)
			if err != nil {
				return err
			}
			if js.on() {
				return js.write(cmd.OutOrStdout(), list)
			}
			printForgeStatus(cmd.OutOrStdout(), list)
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "ask each forge whose each token is (the network)")
	js = addJSON(cmd, "print the forges as JSON", jsonFields(app.ForgeStatus{}), true)
	return cmd
}

func printForgeStatus(out io.Writer, list []app.ForgeStatus) {
	if len(list) == 0 {
		fmt.Fprintln(out, "no forge: none in .git-board.toml, and no GitHub or GitLab remote (git board init sets one up)")
		return
	}
	for _, f := range list {
		sync := "sync on"
		if !f.Sync {
			sync = "sync off"
		}
		switch f.Way {
		case "push":
			sync += ", push only (a backup)"
		case "pull":
			sync += ", read only"
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
				line += "none found: git board forge login " + f.Name
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

func newForgeLogin() *cobra.Command {
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
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			f, who, err := b.LoginFor(cmd.Context(), args[0], login)
			if err != nil {
				return err
			}
			token, err := readSecret(cmd, fmt.Sprintf("token for %s on %s: ", who, f.Host))
			if err != nil {
				return err
			}
			if token == "" {
				return usageError{errors.New("no token read: type it at the prompt, or give it on stdin")}
			}
			if err := app.StoreToken(cmd.Context(), f, who, token); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "stored a token for %s on %s in git's credential helper; git board forge status --check asks %s whose it is\n", who, f.Host, f.Host)
			return nil
		},
	}
	cmd.Flags().StringVar(&login, "login", "", "the account the token is for (default: yours on that host, else the forge's posting account)")
	return cmd
}

// readSecret reads a secret: typed at a hidden prompt on a terminal, else
// the first line of stdin. Never from the command line, where ps and the
// shell's history would show it.
func readSecret(cmd *cobra.Command, prompt string) (string, error) {
	in := cmd.InOrStdin()
	if f, ok := in.(*os.File); ok && isTerminal(in) {
		fmt.Fprint(cmd.ErrOrStderr(), prompt)
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(cmd.ErrOrStderr())
		return strings.TrimSpace(string(b)), err
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// newForgeBackup is the easy path to a backup (use case 1): one command
// adds a forge whose table says way = "push", and stores its token.
func newForgeBackup() *cobra.Command {
	var kind, login, name string
	cmd := &cobra.Command{
		Use:   "backup <url>",
		Short: "Add a forge as a backup: the issues go there, and nothing comes back",
		Long: `Add a forge whose table says way = "push": git board sync and git board
forge push send the issues there, and nothing is pulled from it. An edit
made on the backup never reaches the board: the next run puts the board's
value back there, and says so.

<url> is the project's address, like https://gitlab.example.com/team/repo.
The kind comes from the host's name: a host that names gitlab is GitLab,
one that names github is GitHub; any other host needs --kind. The token
is for --login, by default your own account on that host. It is read at
a hidden prompt on a terminal, else from the first line of stdin, and
stored in git's credential helper, never in .git-board.toml (ADR 0010).`,
		Example: `  git board forge backup https://gitlab.example.com/team/repo
  gh auth token | git board forge backup https://github.com/owner/mirror --kind github`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			u, err := url.Parse(strings.TrimSpace(args[0]))
			if err != nil || u.Host == "" {
				return usageError{fmt.Errorf("%s isn't a project's address, like https://gitlab.example.com/team/repo", args[0])}
			}
			if kind == "" {
				switch {
				case strings.Contains(u.Host, "gitlab"):
					kind = "gitlab"
				case strings.Contains(u.Host, "github"):
					kind = "github"
				default:
					return usageError{fmt.Errorf("%s: whose forge is this? say it with --kind github or --kind gitlab", u.Host)}
				}
			}
			who := login
			if who == "" {
				who = "your account"
			}
			token, err := readSecret(cmd, fmt.Sprintf("token for %s on %s, empty to store none: ", who, u.Host))
			if err != nil {
				return err
			}
			added, err := b.AddForge(cmd.Context(), app.NewForge{Kind: kind, URL: args[0], Name: name, Login: login, Token: token, Way: forge.WayPush})
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "forge %s added: the issues are sent there, and nothing is pulled from it (way push)\n", added)
			if token != "" {
				fmt.Fprintf(out, "its token is in git's credential helper; git board forge status --check asks %s whose it is\n", u.Host)
			} else {
				fmt.Fprintf(out, "no token stored: git board forge login %s stores one\n", added)
			}
			fmt.Fprintln(out, "git board forge sync sends the issues to it from now on")
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "the forge's kind: github or gitlab (default: the host's name says)")
	cmd.Flags().StringVar(&login, "login", "", "the account the token is for (default: yours on that host)")
	cmd.Flags().StringVar(&name, "name", "backup", "the forge's name in .git-board.toml")
	return cmd
}
