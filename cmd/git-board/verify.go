package main

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/proof"
	"github.com/guilbep/git-board/internal/release"
)

func newVerifyCmd() *cobra.Command {
	var base string
	cmd := &cobra.Command{
		Use:   "verify [commit]",
		Short: "Tell whether a commit's checks are proven, as Mark ready for review does",
		Long: `Reads the [cycle.checks.*] and the signers file of the base branch's
.git-board.toml, then each check's signed results in refs/notes/ci/*
(docs/adr/0001), and prints its verdict at the commit (default HEAD).
Exits 1 when the commit isn't proven: the server's gate, without a server.`,
		Example: `  git board verify
  git board verify fix/2518-retry --base origin/main`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			commit := "HEAD"
			if len(args) == 1 {
				commit = args[0]
			}
			return runVerify(cmd.Context(), cmd.OutOrStdout(), ".", base, commit)
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "the branch whose .git-board.toml and signers file rule (default: [branches] base, else origin's default branch, else main; origin/<name> when it exists)")
	return cmd
}

func runVerify(ctx context.Context, out io.Writer, dir, base, commit string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	sha, err := release.Git(ctx, dir, "rev-parse", "--verify", "--end-of-options", commit+"^{commit}")
	if err != nil {
		return err
	}
	if base == "" {
		cfg, err := config.Load(config.PathIn(dir))
		if err != nil {
			return err
		}
		base = baseRef(ctx, dir, cfg.Branches.Base)
	}
	g, err := proof.Gate(ctx, dir, base, sha)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s against the checks on %s\n", sha[:7], base)
	for _, c := range g.Checks {
		line := "not run"
		if c.Result != nil {
			line = fmt.Sprintf("%s, signed by %s on %s", c.Verdict, c.Signer, c.Result.At.Local().Format(time.DateTime))
		}
		fmt.Fprintf(out, "  %-12s %s\n", c.Name, line)
	}
	if !g.Proven {
		return fmt.Errorf("not proven: %s", g.Reason)
	}
	fmt.Fprintln(out, cmp.Or(g.Reason, "proven"))
	return nil
}

// baseRef is the server's base (branches.Suggest's Start), read with the
// git CLI: a linked worktree and a held git-bug lock don't stop it.
func baseRef(ctx context.Context, dir, name string) string {
	if name == "" {
		head, _ := release.Git(ctx, dir, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD")
		name, _ = strings.CutPrefix(head, "origin/")
	}
	if name == "" {
		name = "main"
		for _, c := range []string{"main", "master", "trunk"} {
			if has(ctx, dir, "refs/remotes/origin/"+c) || has(ctx, dir, "refs/heads/"+c) {
				name = c
				break
			}
		}
	}
	if has(ctx, dir, "refs/remotes/origin/"+name) {
		return "origin/" + name
	}
	return name
}

func has(ctx context.Context, dir, ref string) bool {
	_, err := release.Git(ctx, dir, "rev-parse", "--verify", "-q", ref)
	return err == nil
}
