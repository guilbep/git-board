package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/gitbugio"
)

func newSyncCmd() *cobra.Command {
	var remote string

	cmd := &cobra.Command{
		Use:   "sync [path...]",
		Short: "git bug pull/push on one or more git-bug repos",
		Long: `git bug pull/push on one or more git-bug repos.

Each path is a directory containing a git-bug-initialized repo (defaults
to the current directory if none given). This is deliberately explicit
paths, not [repos] from .git-board.toml: that table holds "owner/repo"
display names, not local clone locations, and there's no established
convention yet for mapping one to the other — see docs/implementation-plan.md.`,
		Example: `  git-board sync                          # cwd only
  git-board sync ../co2-calculator ../git-time
  git-board sync --remote upstream`,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := args
			if len(paths) == 0 {
				paths = []string{"."}
			}
			return runSync(cmd.OutOrStdout(), paths, remote)
		},
	}
	cmd.Flags().StringVar(&remote, "remote", "origin", "git remote to pull from / push to")
	return cmd
}

// runSync pulls then pushes each path in turn, in-process (no `git bug`
// subprocess — internal/gitbugio.Pull/Push wrap cache.RepoCache.Pull/Push
// directly). Keeps going after a failure on one path so one broken repo
// doesn't block the rest; every failure is still surfaced, as a combined
// error at the end.
func runSync(out io.Writer, paths []string, remote string) error {
	var failed []string
	for _, path := range paths {
		fmt.Fprintf(out, "%s: pulling from %s...\n", path, remote)
		if err := syncOne(out, path, remote); err != nil {
			fmt.Fprintf(out, "%s: %v\n", path, err)
			failed = append(failed, path)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("sync failed for %d of %d repo(s): %v", len(failed), len(paths), failed)
	}
	return nil
}

func syncOne(out io.Writer, path, remote string) error {
	b, err := gitbugio.Open(path)
	if err != nil {
		return fmt.Errorf("opening: %w", err)
	}
	defer b.Close()

	if err := b.Pull(remote); err != nil {
		// An empty remote (nothing pushed there yet — the normal state
		// of a brand-new shared repo before anyone's first sync) is not
		// a failure: there's nothing to pull, not an error pulling it.
		// Confirmed this is a real case, not a hypothetical: found via
		// a real two-repo test using a genuinely empty bare remote.
		if errors.Is(err, transport.ErrEmptyRemoteRepository) {
			fmt.Fprintf(out, "%s: remote has nothing to pull yet\n", path)
		} else {
			return fmt.Errorf("pull: %w", err)
		}
	}

	pushOut, err := b.Push(remote)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	if pushOut != "" {
		fmt.Fprintf(out, "%s: %s\n", path, pushOut)
	}
	fmt.Fprintf(out, "%s: synced\n", path)
	return nil
}
