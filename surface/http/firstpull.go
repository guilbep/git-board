package httpapi

import (
	"context"

	"github.com/guilbep/git-board/app"
)

// FirstPull brings a fresh clone origin's board, once, as the server
// starts (app.PullIfEmpty, docs/prd/clone-shows-board.md). What came, or
// why nothing did, is kept for /api/board's first_pull: the page that
// opens next says it, and a page already open hears it now (a "me"
// event reloads the board; each issue pulled is an issue event).
func (s *Server) FirstPull(ctx context.Context) {
	h := s.boards.get()
	defer s.boards.release(h)
	note := h.b.PullIfEmpty(ctx)
	switch {
	case note == nil:
		return // nothing to say
	case note.Error != "":
		s.log.Warn(note.Error)
	default:
		s.log.Info("a fresh clone: pulled from "+note.Remote, "issues", note.Issues)
	}
	s.firstMu.Lock()
	s.first = note
	s.firstMu.Unlock()
	s.publish(ctx, Event{Kind: "me"})
}

// firstPulled is what the first open pulled; nil for nothing.
func (s *Server) firstPulled() *app.FirstPull {
	s.firstMu.Lock()
	defer s.firstMu.Unlock()
	return s.first
}
