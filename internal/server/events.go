package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Live updates: every open page holds GET /api/events (Server-Sent
// Events) and reloads what an event names. A write through the API
// publishes one when it succeeds (withEvents), a forge sync when it ends,
// and a change of the issues' refs made outside the API (watchRefs).

// event is what changed: issues (ID: which one, when the request named
// it), config (.git-board.toml, a view), sync, settings, inbox (yours:
// what you read or follow), releases (a job's step, a record, an
// overlay), progress (a forge run's step, Step; ID: the issue, when the
// run is one issue's own), cycle (a step 4 run recorded a result; ID: the
// issue). By is the page that made the change
// (X-Git-Board-Client), which ignores it.
type event struct {
	Kind string    `json:"kind"`
	ID   string    `json:"id,omitempty"`
	By   string    `json:"by,omitempty"`
	Step *syncStep `json:"step,omitempty"`
}

// subBuffer is how many events a page may fall behind before it is
// dropped: its stream closes, it reconnects and reloads.
const subBuffer = 16

type broker struct {
	mu   sync.Mutex
	subs map[chan []byte]bool
}

func (b *broker) subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, subBuffer)
	b.mu.Lock()
	if b.subs == nil {
		b.subs = map[chan []byte]bool{}
	}
	b.subs[ch] = true
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.subs[ch] {
			delete(b.subs, ch)
			close(ch)
		}
	}
}

func (b *broker) publish(e event) {
	data, _ := json.Marshal(e)
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- data:
		default: // too slow: dropped, it reconnects
			delete(b.subs, ch)
			close(ch)
		}
	}
}

func (s *apiServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // a proxy in front must not hold the stream
	ch, cancel := s.events.subscribe()
	defer cancel()
	fmt.Fprint(w, "retry: 3000\n\n")
	fl.Flush()
	ping := time.NewTicker(25 * time.Second) // keeps proxies from closing an idle stream
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			fl.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// notWrites are the POSTs that change nothing here (the labels' change
// GitHub's labels, not the board).
var notWrites = map[string]bool{"/api/markdown": true, "/api/config/check": true, "/api/uploads": true, "/api/worktrees/preview": true,
	"/api/labels/send": true, "/api/labels/adopt": true, "/api/labels/delete": true}

// withEvents publishes an event for every write that succeeds.
func (s *apiServer) withEvents(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions || notWrites[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if rec.status < 400 {
			s.seeRefs() // ours: not an outside change
			s.events.publish(eventFor(r))
		}
	})
}

func eventFor(r *http.Request) event {
	p := r.URL.Path
	e := event{By: r.Header.Get("X-Git-Board-Client")}
	switch {
	case strings.HasPrefix(p, "/api/issues"):
		e.Kind = "issues"
		if rest, ok := strings.CutPrefix(p, "/api/issues/"); ok {
			e.ID, _, _ = strings.Cut(rest, "/")
		}
	case p == "/api/config" || strings.HasPrefix(p, "/api/views/"):
		e.Kind = "config"
	case p == "/api/sync/apply" || p == "/api/sync/resolve": // the Project's values written to labels
		e.Kind = "issues"
	case strings.HasPrefix(p, "/api/remotes/") && strings.HasSuffix(p, "/pull"): // merged in
		e.Kind = "issues"
	case strings.HasPrefix(p, "/api/numbers/"): // the issues' numbers
		e.Kind = "issues"
	case strings.HasPrefix(p, "/api/sync") || strings.HasPrefix(p, "/api/forges") || strings.HasPrefix(p, "/api/remotes/"):
		e.Kind = "sync"
	case strings.HasPrefix(p, "/api/inbox"): // read, followed: your inbox's count, in every tab
		e.Kind = "inbox"
	case strings.HasPrefix(p, "/api/releases") || strings.HasPrefix(p, "/api/environments/"):
		e.Kind = "releases"
	default: // bridges, who you write as
		e.Kind = "settings"
	}
	return e
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// watchRefs publishes an issues event when the issues', people's or
// release records' refs change without going through the API (a sync job,
// a git fetch into the repository, a CLI cut), and a releases event when
// the records' or the tags' do, checking every interval.
func (s *apiServer) watchRefs(interval time.Duration, stop <-chan struct{}) {
	s.seeRefs()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			fp, rfp := s.b.RefsFingerprint(), s.b.ReleasesFingerprint()
			s.refsMu.Lock()
			changed, releasesChanged := fp != s.refs, rfp != s.releaseRefs
			s.refs, s.releaseRefs = fp, rfp
			s.refsMu.Unlock()
			if changed {
				s.events.publish(event{Kind: "issues"})
			}
			if releasesChanged { // a record or a tag: an issue edit doesn't reload the Releases page
				s.events.publish(event{Kind: "releases"})
			}
		}
	}
}

// seeRefs takes the refs as they are now as seen: a change the API made
// itself is announced by withEvents, not again by watchRefs.
func (s *apiServer) seeRefs() {
	fp, rfp := s.b.RefsFingerprint(), s.b.ReleasesFingerprint()
	s.refsMu.Lock()
	s.refs, s.releaseRefs = fp, rfp
	s.refsMu.Unlock()
}
