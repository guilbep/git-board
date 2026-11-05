package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/store/git"
)

// A fresh clone's board (docs/prd/clone-shows-board.md), as v1's
// store.PullIfEmpty: git clone fetches branches and tags, never
// refs/board/*, so a clone opens on an empty board. Its first open pulls
// origin's, once.

// BoardRemote is the remote a fresh clone's board comes from.
const BoardRemote = "origin"

// ProbeTimeout bounds the first open's look at the remote: a remote that
// doesn't answer (offline, an SSH host to confirm) is said, never waited
// on.
var ProbeTimeout = 20 * time.Second

// FirstPull is what the first open of a fresh clone did: how many issues
// it pulled from which remote, or why it couldn't.
type FirstPull struct {
	Remote string `json:"remote"`
	Issues int    `json:"issues"`
	Error  string `json:"error,omitempty"`
}

// PullIfEmpty pulls origin's board into a clone that has none of its
// own: no issue here, and origin never fetched. Nothing local can be
// lost, there is nothing local. It answers nil when it did nothing: a
// board with issues, a remote fetched before (an empty board stays empty,
// no fetch on every open), no origin, a board not kept in git, or an
// origin with no board yet.
func (b *Board) PullIfEmpty(ctx context.Context) *FirstPull {
	g, ok := b.Kernel.Store().(*git.Store)
	if !ok {
		return nil // the board isn't in git: origin holds none of it
	}
	remotes, err := gitRun(ctx, b.Dir, "remote")
	if err != nil || !slices.Contains(strings.Fields(remotes), BoardRemote) {
		return nil
	}
	if heads, err := g.Heads(ctx, "issue"); err != nil || len(heads) > 0 {
		return nil
	}
	// fetched or pushed once: the board has been here, it stays as it is
	if tracked, err := gitRun(ctx, b.Dir, "for-each-ref", "--count=1", "--format=%(refname)",
		"refs/board-remotes/"+BoardRemote+"/issue/", "refs/board-remotes/"+BoardRemote+"/person/"); err != nil || strings.TrimSpace(tracked) != "" {
		return nil
	}
	note := &FirstPull{Remote: BoardRemote}
	fail := func(err error) *FirstPull {
		note.Error = fmt.Sprintf("%s can't be read, your issues may be there: %v", BoardRemote, err)
		return note
	}
	// a look first, bounded and with no prompt
	has, err := remoteHasBoard(ctx, b.Dir, BoardRemote)
	if err != nil {
		return fail(err)
	}
	if !has {
		return nil // origin has no board yet
	}
	if _, err := b.Pull(ctx, BoardRemote); err != nil {
		return fail(err)
	}
	pulled, err := b.Issues.List(ctx, issues.ListReq{}) // every issue the board shows
	if err != nil {
		return fail(err)
	}
	note.Issues = len(pulled)
	return note
}

// Line is the note as the CLI says it; "" for none.
func (n *FirstPull) Line() string {
	switch {
	case n == nil:
		return ""
	case n.Error != "":
		return "git-board: " + n.Error
	case n.Issues == 1:
		return "git-board: a fresh clone: pulled 1 issue from " + n.Remote
	}
	return fmt.Sprintf("git-board: a fresh clone: pulled %d issues from %s", n.Issues, n.Remote)
}

// remoteHasBoard asks the remote whether it holds a board (any
// refs/board/issue/*), with no prompt and within ProbeTimeout.
func remoteHasBoard(ctx context.Context, dir, remote string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "ls-remote", remote, "refs/board/issue/*")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND=ssh -o BatchMode=yes", "GCM_INTERACTIVE=never", "GIT_ASKPASS=")
	cmd.WaitDelay = time.Second
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return false, fmt.Errorf("no answer within %s", ProbeTimeout)
		}
		return false, fmt.Errorf("%w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return out.Len() > 0, nil
}
