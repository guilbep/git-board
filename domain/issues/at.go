package issues

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/domain/people"
	"github.com/guilbep/git-board/kernel"
)

// ErrNotYet is an issue whose create was made after the time asked.
var ErrNotYet = errors.New("issue: not created yet")

// At is a log as it stood at a time, by each operation's own time (its
// writer's clock): the operations made at or before at, in the log's
// order. An edit or a set-meta of a text made later goes with it, as it
// has nothing to change yet. ErrNotYet when the create was made later.
func At(log []kernel.Entry, at int64) ([]kernel.Entry, error) {
	if len(log) == 0 || log[0].Time > at {
		return nil, ErrNotYet
	}
	kept := map[kernel.OpID]bool{}
	var out []kernel.Entry
	for _, e := range log {
		if e.Time > at {
			continue
		}
		if e.Kind == "edit-text" || e.Kind == "set-meta" {
			if d, err := decode(e.Data); err == nil && !kept[d.Target] {
				continue
			}
		}
		kept[e.ID] = true
		out = append(out, e)
	}
	return out, nil
}

// Notice is what a read said aside: an issue it left out, and why, as
// v1 logs it on stderr.
type Notice func(format string, args ...any)

// ListAt is List on the board as it stood at a time, with today then:
// each issue folded from the operations made by then, free text looked
// for in its texts as they were. Who you are and who is mentioned are
// as now. Issues left out, deleted or broken, are told to notice.
func (s *Service) ListAt(ctx context.Context, r ListReq, at time.Time) ([]Issue, error) {
	notice := s.notice
	ctx = s.Board.Snapshot(ctx)
	dir, err := people.Read(ctx, s.Board, s.Avatars)
	if err != nil {
		return nil, err
	}
	type read struct {
		id  kernel.ID
		log []kernel.Entry
		err error
	}
	var mu sync.Mutex
	var reads []read
	err = Kind.Logs(ctx, s.Board, func(id kernel.ID, log []kernel.Entry, err error) {
		mu.Lock()
		reads = append(reads, read{id, log, err})
		mu.Unlock()
	})
	if err != nil {
		return nil, fmt.Errorf("listing the issues: %w", err)
	}
	slices.SortFunc(reads, func(a, b read) int { return strings.Compare(string(a.id), string(b.id)) })
	rdg := s.reading(ctx, notice, false)
	var list []Issue
	for _, rd := range reads {
		err := rd.err
		if err == nil {
			var st State
			if st, err = Kind.Fold(rd.id, rd.log); err == nil && st.Deleted {
				err = fmt.Errorf("issue %s: %w", rd.id, ErrDeleted)
			}
		}
		if err != nil { // left out, as a list leaves a broken one out
			notice("issue %s: %v", rd.id, err)
			continue
		}
		then, err := At(rd.log, at.Unix())
		if errors.Is(err, ErrNotYet) {
			continue
		}
		st, err := Kind.Fold(rd.id, then)
		if err != nil {
			notice("issue %s on %s: %v", rd.id, at.Format(time.DateOnly), err)
			continue
		}
		list = append(list, s.texted(&st, dir, rdg))
	}
	slices.SortFunc(list, func(x, y Issue) int { return cmp.Or(x.CreatedAt.Compare(y.CreatedAt), strings.Compare(x.ID, y.ID)) })
	r.Today = at
	conds, me, err := s.prepare(ctx, dir, r, false)
	if err != nil {
		return nil, err
	}
	return s.keep(list, conds, me, at), nil
}

// texted is a state as a list shows it, with its texts, for a free text
// looked for without the view.
func (s *Service) texted(st *State, dir people.Directory, rd reading) Issue {
	d := index(st)
	is := s.issue(st.ID, &d.Row, dir, rd)
	is.Body = st.Body
	for _, c := range st.Comments {
		is.CommentBodies = append(is.CommentBodies, c.Body)
	}
	return is
}

// ShowAt is Show as the issue stood at a time: only the operations made
// by then. Not found when it wasn't created yet.
func (s *Service) ShowAt(ctx context.Context, name string, at time.Time) (Thread, error) {
	ctx = s.Board.Snapshot(ctx)
	if err := s.Board.Fresh(ctx, Kind, people.Kind); err != nil {
		return Thread{}, err
	}
	id, err := s.Resolve(ctx, name)
	if err != nil {
		return Thread{}, err
	}
	log, _, err := Kind.Log(ctx, s.Board, id)
	if err != nil {
		return Thread{}, err
	}
	if st, err := Kind.Fold(id, log); err != nil {
		return Thread{}, err
	} else if st.Deleted {
		return Thread{}, fmt.Errorf("issue %s: %w", id, ErrDeleted)
	}
	then, err := At(log, at.Unix())
	if errors.Is(err, ErrNotYet) {
		return Thread{}, fmt.Errorf("%w: issue %.7s was not created yet on %s", ErrNotFound, id, at.Local().Format("2006-01-02 15:04"))
	}
	st, err := Kind.Fold(id, then)
	if err != nil {
		return Thread{}, err
	}
	dir, err := people.Read(ctx, s.Board, s.Avatars)
	if err != nil {
		return Thread{}, err
	}
	d := index(&st)
	return s.thread(&st, &d.Row, then, dir, s.reading(ctx, s.notice, false)), nil
}
