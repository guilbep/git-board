package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	gosync "sync"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/store"
	"github.com/guilbep/git-board/store/forgecopy"
	"github.com/guilbep/git-board/store/git"
	"github.com/guilbep/git-board/sync"
)

// ErrNotOnForge: issues whose writes the forges may not have yet.
var ErrNotOnForge = errors.New("issues with writes the forges may not have yet")

// DropCopy throws away the copy a board kept on the forges alone holds,
// and its sync journal, as v1's git board datastore drop: the next sync
// pulls the board again. People and releases, which no forge keeps, go to
// the new copy. It refuses (ErrNotOnForge) while an issue isn't on a
// forge yet, unless force, and answers how many such issues there were.
// The board is closed after it: open it again to read.
func (b *Board) DropCopy(ctx context.Context, force bool) (int, error) {
	f, ok := b.Kernel.Store().(*forgecopy.Store)
	if !ok {
		return 0, errors.New("the board isn't kept on the forges alone: there is no copy to throw away")
	}
	waiting, err := b.notOnForge(ctx)
	if err != nil {
		return 0, err
	}
	if waiting > 0 && !force {
		return waiting, fmt.Errorf("%d %w (sync first, or --force to lose them)", waiting, ErrNotOnForge)
	}
	if err := sync.Forget(f); err != nil {
		return waiting, err
	}
	folder := f.Folder()
	dir := forgecopy.Dir(folder)
	if err := b.Close(); err != nil {
		return waiting, err
	}
	old := dir + ".dropped"
	if err := os.RemoveAll(old); err != nil {
		return waiting, err
	}
	if err := os.Rename(dir, old); err != nil {
		return waiting, err
	}
	defer os.RemoveAll(old)
	from, err := git.OpenGitDir(old)
	if err != nil {
		return waiting, err
	}
	defer from.Close()
	to, err := forgecopy.Open(ctx, folder)
	if err != nil {
		return waiting, err
	}
	defer to.Close()
	_, err = store.Copy(ctx, from, to, "person", "release")
	return waiting, err
}

// notOnForge counts the issues no forge has, or with a write of this
// board's own since the last sync run.
func (b *Board) notOnForge(ctx context.Context) (int, error) {
	ctx = b.Kernel.Snapshot(ctx)
	records, err := sync.Records(ctx, b.Kernel, nil)
	if err != nil {
		return 0, err
	}
	var mu gosync.Mutex
	var last int64
	err = sync.Kind.Logs(ctx, b.Kernel, func(id kernel.ID, log []kernel.Entry, err error) {
		if err != nil {
			return
		}
		if st, err := sync.Kind.Fold(id, log); err == nil {
			mu.Lock()
			last = max(last, st.LastRun)
			mu.Unlock()
		}
	})
	if err != nil {
		return 0, err
	}
	n := 0
	err = issues.Kind.Logs(ctx, b.Kernel, func(id kernel.ID, log []kernel.Entry, err error) {
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if len(records[string(id)]) == 0 || slices.ContainsFunc(log, func(e kernel.Entry) bool { return e.Origin == nil && e.Time >= last }) {
			n++
		}
	})
	return n, err
}
