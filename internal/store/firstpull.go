package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// probeTimeout bounds the first open's look at the remote: a remote that
// doesn't answer (offline, an SSH host to confirm) is said, never waited on.
var probeTimeout = 20 * time.Second

// BoardRemote is the remote a fresh clone's board comes from.
const BoardRemote = "origin"

// PullIfEmpty pulls remote's board into a clone that has none of its own:
// no issue here, and the remote never fetched (git clone fetches branches
// and tags, never refs/board/*). Nothing local can be lost, there is
// nothing local. It answers how many issues the board holds after, 0 when
// it did nothing: a board with issues, a remote fetched before (an empty
// board stays empty, no fetch on every open), or no such remote
// (docs/prd/clone-shows-board.md).
func (b *Backend) PullIfEmpty(remote string) (int, error) {
	if !b.inGit() {
		return 0, nil // the board isn't in git: origin holds none of it
	}
	has, err := b.hasRemote(remote)
	if err != nil || !has {
		return 0, err
	}
	issues, err := b.ListIssues()
	if err != nil || len(issues) > 0 {
		return 0, err
	}
	remotes, err := b.Remotes()
	if err != nil {
		return 0, err
	}
	for _, r := range remotes {
		if r.Name == remote && r.Tracked {
			return 0, nil
		}
	}
	// a look first, bounded and with no prompt: the pull holds the
	// server's writes while it runs
	ok, err := b.remoteHasBoard(remote)
	if err != nil {
		return 0, fmt.Errorf("%s can't be read, your issues may be there: %w", remote, err)
	}
	if !ok {
		return 0, nil // the remote has no board yet
	}
	if err := b.Pull(remote); err != nil {
		return 0, fmt.Errorf("%s can't be read, your issues may be there: %w", remote, err)
	}
	issues, err = b.ListIssues()
	return len(issues), err
}

// remoteHasBoard asks the remote whether it holds a board (any
// refs/board/issue/*), with no prompt and within probeTimeout.
func (b *Backend) remoteHasBoard(remote string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--git-dir", b.dir, "ls-remote", remote, "refs/board/issue/*")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND=ssh -o BatchMode=yes", "GCM_INTERACTIVE=never")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return false, fmt.Errorf("no answer within %s", probeTimeout)
		}
		return false, fmt.Errorf("%w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return out.Len() > 0, nil
}
