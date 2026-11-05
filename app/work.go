package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/guilbep/git-board/domain/cycle"
	"github.com/guilbep/git-board/domain/cycle/work"
	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/tools/branches"
	"github.com/guilbep/git-board/tools/code"
)

// The Work page's rows, every open issue's step. An issue's row is a
// function of its thread, the viewer, the code's refs and the config
// (a Board's own): each is kept by issue with what it was computed
// from, its version, the threads' mark (issues.ThreadsMark), the viewer
// and the refs, and computed again when one of them moved. A thread is
// kept slimmed to what the cycle reads, by its version and the mark. A
// source the evidence reads one day besides these (the forge's PRs)
// joins the row's key.

// WorkRow is a row of the Work page.
type WorkRow = work.Row

// workKept is what the Work page computed, by issue.
type workKept struct {
	mu   sync.Mutex
	mark string // the threads'
	by   map[kernel.ID]workIssue
}

type workIssue struct {
	v   kernel.Version
	th  issues.Thread // slim
	key string        // the row's: the viewer and the refs; "" for none
	row work.Row
}

// workRows is each listed issue's row, in the list's order.
func (b *Board) workRows(ctx context.Context, list []issues.Issue, m issues.InboxMe) ([]work.Row, error) {
	mark, err := b.Issues.ThreadsMark(ctx)
	if err != nil {
		return nil, err
	}
	// the branches and the base's history, read once for the page
	c := b.Code()
	c.Kept = code.NewKept()
	anc := branches.NewAncestry(c)
	key := rowKey(ctx, c, m, b.PRs.Gen())

	b.work.mu.Lock()
	had := b.work.by
	if b.work.mark != mark {
		had = nil
	}
	b.work.mu.Unlock()

	kept := make([]workIssue, len(list))
	var miss []issues.Issue // their threads unread
	var at []int
	for i, is := range list {
		k, ok := had[kernel.ID(is.ID)]
		if !ok || k.v != is.Version {
			miss, at = append(miss, is), append(at, i)
			k = workIssue{v: is.Version}
		}
		kept[i] = k
	}
	if len(miss) > 0 {
		errs := make([]error, len(miss))
		err := b.Issues.ThreadsOf(ctx, miss, func(i int, th issues.Thread, err error) {
			errs[i] = err
			kept[at[i]].th = slim(th)
		})
		if err = errors.Join(append(errs, err)...); err != nil {
			return nil, err
		}
	}

	// the rows not kept, a few at once: an issue with branches reads
	// them through git
	errs := make([]error, len(list))
	next := make(chan int)
	var wg sync.WaitGroup
	for range max(2, runtime.GOMAXPROCS(0)) {
		wg.Go(func() {
			for i := range next {
				k := &kept[i]
				in, err := b.EvidenceIn(ctx, anc, k.th, m.Login, b.cachedPRs(), nil)
				if errs[i] = err; err == nil {
					k.row, k.key = work.RowOf(k.th, cycle.Compute(in), m), key
				}
			}
		})
	}
	for i := range kept {
		if key == "" || kept[i].key != key {
			next <- i
		}
	}
	close(next)
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	rows := make([]work.Row, len(list))
	by := make(map[kernel.ID]workIssue, len(list))
	for i, is := range list {
		rows[i] = kept[i].row
		by[kernel.ID(is.ID)] = kept[i]
	}
	b.work.mu.Lock()
	b.work.mark, b.work.by = mark, by
	b.work.mu.Unlock()
	return rows, nil
}

// rowKey is what a row reads besides its thread and the config: the
// viewer, and the code's refs (the branches, local and remote, and the
// remote's HEAD), which name every commit and file the evidence reads.
// "" when git can't list them: nothing is kept then.
func rowKey(ctx context.Context, c code.Repo, m issues.InboxMe, prs uint64) string {
	all, err := c.Branches(ctx)
	if err != nil {
		return ""
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%d\n", m.ID, m.Login, c.RemoteHead(ctx, "origin"), prs)
	for _, br := range all {
		fmt.Fprintf(h, "%s %s %s\n", br.Remote, br.Name, br.Hash)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// slim is a thread as the cycle and the Work page's row read it
// (cycle.Gather, work.RowOf): the issue, its author, and each event's
// kind, id, author, time and metadata; the bodies of the description
// (the criteria) and of the comments a question can be (ending in ?, or
// asking), no other text.
func slim(th issues.Thread) issues.Thread {
	out := issues.Thread{Issue: th.Issue, Author: th.Author, Events: make([]issues.Event, len(th.Events))}
	for i, e := range th.Events {
		s := issues.Event{Kind: e.Kind, ID: e.ID, Author: e.Author, At: e.At, Meta: e.Meta}
		if i == 0 || e.Meta["ask"] != "" || strings.HasSuffix(strings.TrimSpace(e.Body), "?") {
			s.Body = e.Body
		}
		out.Events[i] = s
	}
	return out
}
