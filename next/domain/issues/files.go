package issues

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sync"

	"github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/kernel"
)

// What a Markdown folder's files are rendered from (mdfiles): each issue
// at its version, its thread, and what else a file shows.

// Head is a visible issue, as a list shows it, at its version.
type Head struct {
	Issue
	Version kernel.Version
}

// Heads are the visible issues at their versions, by id, and a sum of
// what else their files show: the people's names, the counter. A file
// whose issue is at the version it was rendered at, under the same sum,
// renders the same again.
func (s *Service) Heads(ctx context.Context) ([]Head, string, error) {
	ctx = s.Board.Snapshot(ctx)
	if err := s.Board.Fresh(ctx, Kind, people.Kind); err != nil {
		return nil, "", err
	}
	ls, err := Kind.List(ctx, s.Board, kernel.Query{})
	if err != nil {
		return nil, "", err
	}
	ps, err := people.Kind.List(ctx, s.Board, kernel.Query{})
	if err != nil {
		return nil, "", err
	}
	dir, err := people.Read(ctx, s.Board, s.Avatars)
	if err != nil {
		return nil, "", err
	}
	rd := s.reading(ctx, s.notice, true)
	h := sha256.New()
	for _, p := range ps {
		h.Write([]byte(string(p.ID) + " " + string(p.Version) + "\n"))
	}
	if rd.cnt != nil {
		h.Write(rd.cnt.Bytes())
	}
	h.Write([]byte(rd.sum))
	out := make([]Head, len(ls))
	for i, l := range ls {
		out[i] = Head{Issue: s.issue(l.ID, &l.Row, dir, rd), Version: l.Version}
	}
	slices.SortFunc(out, func(x, y Head) int { return compare(x.ID, y.ID) })
	return out, hex.EncodeToString(h.Sum(nil)[:16]), nil
}

func compare(x, y string) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

// Threads are the issues' threads, in the order of ids.
func (s *Service) Threads(ctx context.Context, ids []kernel.ID) ([]Thread, error) {
	ctx = s.Board.Snapshot(ctx)
	dir, err := people.Read(ctx, s.Board, s.Avatars)
	if err != nil {
		return nil, err
	}
	rd := s.reading(ctx, s.notice, true)
	out := make([]Thread, len(ids))
	at := make(map[kernel.ID]int, len(ids))
	for i, id := range ids {
		at[id] = i
	}
	one := func(i int, id kernel.ID, log []kernel.Entry) error {
		st, err := Kind.Fold(id, log)
		if err != nil {
			return err
		}
		d := index(&st)
		out[i] = s.thread(&st, &d.Row, log, dir, rd)
		return nil
	}
	if len(ids) < 64 {
		for i, id := range ids {
			log, _, err := Kind.Log(ctx, s.Board, id)
			if err != nil {
				return nil, err
			}
			if err := one(i, id, log); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	// many: every log read at once, on every CPU
	var mu sync.Mutex
	var first error
	err = Kind.Logs(ctx, s.Board, func(id kernel.ID, log []kernel.Entry, err error) {
		i, ok := at[id]
		if !ok {
			return
		}
		if err == nil {
			err = one(i, id, log)
		}
		if err != nil {
			mu.Lock()
			first = cmpErr(first, err)
			mu.Unlock()
		}
	})
	if err == nil {
		err = first
	}
	return out, err
}

func cmpErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

// Stored is a change of labels as the issue keeps them, raw its labels
// now (ChangeLabels's): a flat kind added as its plain label, an
// assignee once in any case.
func (s *Service) Stored(added, removed, raw []string) (a, r []string) {
	return s.stored(added, removed, raw)
}
