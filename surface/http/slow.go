package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/guilbep/git-board/obs"
)

// slowReq is a page's interaction that took longer than its budget: what
// the person did (web/src/interaction.ts), how long until the page
// painted its answer, and the trace its requests carried.
type slowReq struct {
	Name  string  `json:"name"`
	Ms    float64 `json:"ms"`
	Trace string  `json:"trace"`
	Span  string  `json:"span"`
}

// slow is POST /api/slow: a slow interaction, logged at warn and, when
// tracing is on, recorded as its trace's root, its requests under it.
func (s *Server) slow(ctx context.Context, r slowReq) (Reply, error) {
	switch {
	case r.Name == "" || len(r.Name) > 120:
		return Reply{}, invalid("name", errors.New("an interaction's name, 1 to 120 characters"))
	case r.Ms < 0 || r.Ms > 600_000:
		return Reply{}, invalid("ms", fmt.Errorf("%v ms: from 0 to 600000", r.Ms))
	}
	took := time.Duration(r.Ms * float64(time.Millisecond))
	s.log.LogAttrs(ctx, slog.LevelWarn, "slow interaction", slog.String("name", r.Name), slog.Duration("duration", took), slog.String("trace", r.Trace))
	obs.RecordRoot(r.Trace, r.Span, "interaction "+r.Name, time.Now().Add(-took), took)
	return Reply{Status: http.StatusNoContent}, nil
}
