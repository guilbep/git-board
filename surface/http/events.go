package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/kernel"
)

// Change events (docs/rewrite/api.md, Change events): one Server-Sent
// Events stream. A write through the API is told at once, by the
// kernel's observer; a change made outside (a command, a fetch, another
// process's sync) by the watcher, which compares the datastore's heads
// every second.

// Event is one change.
type Event struct {
	Kind    string  `json:"kind"`
	ID      string  `json:"id,omitempty"`
	Version string  `json:"version,omitempty"`
	By      string  `json:"by,omitempty"`
	Issue   string  `json:"issue,omitempty"`   // progress: the issue a step is about
	Step    *Step   `json:"step,omitempty"`    // progress
	Tallies []Tally `json:"tallies,omitempty"` // progress: the run's tallies so far
	Error   string  `json:"error,omitempty"`   // config-error: why the file edited by hand does not open
}

// subBuffer is how many events a page may fall behind before it is
// dropped: its stream closes, it reconnects and reloads.
const subBuffer = 64

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

func (b *broker) publish(e Event) {
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

// publish sends e to every page, and logs it at debug.
func (s *Server) publish(ctx context.Context, e Event) {
	s.log.LogAttrs(ctx, slog.LevelDebug, "event", slog.String("kind", e.Kind), slog.String("id", e.ID))
	s.events.publish(e)
}

// ping is how often an idle stream says it lives: proxies close one
// that says nothing.
var ping = 25 * time.Second

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeError(w, r, s.log, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // a proxy in front must not hold the stream
	ch, cancel := s.events.subscribe()
	defer cancel()
	fmt.Fprint(w, "retry: 3000\n\n")
	fl.Flush()
	t := time.NewTicker(ping)
	defer t.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.done:
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			fl.Flush()
		case <-t.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// seen is the version each entity was last told at: the observer and
// the watcher tell each change once. The watcher waits a tick before it
// tells a version it found: a write of the API's lands in the datastore
// before its observer runs, and the observer tells it with its page.
type seen struct {
	mu      sync.Mutex
	at      map[string]string // kind/id -> the version told
	pending map[string]string // found by the watcher, told next tick
}

// observe tells the pages of each change a request wrote, as it lands:
// of the kinds the watcher reads. The others (a release record, a forge's
// sync state) the server tells itself, by their tag or run, and the
// watcher would tell them gone a tick later.
func (s *Server) observe(ctx context.Context, cs []kernel.Change) {
	for _, c := range cs {
		if !slices.Contains(watchedKinds, c.Kind) {
			continue
		}
		k := c.Kind + "/" + string(c.ID)
		s.seen.mu.Lock()
		tell := s.seen.at[k] != string(c.Version)
		s.seen.at[k] = string(c.Version)
		delete(s.seen.pending, k)
		s.seen.mu.Unlock()
		if tell {
			s.publish(ctx, Event{Kind: c.Kind, ID: string(c.ID), Version: string(c.Version), By: clientOf(ctx)})
		}
	}
}

// watchedKinds are the kinds whose changes the pages hear.
var watchedKinds = []string{"issue", "person"}

// watch compares the datastore's heads and .git-board.toml every
// interval, until ctx ends, and tells what moved outside the API. A
// changed file opens the board again: its config, its schema.
func (s *Server) watch(ctx context.Context, interval time.Duration) {
	s.heads(ctx, false)
	s.own.mu.Lock()
	s.own.sum = s.fileSum()
	s.own.mu.Unlock()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if s.configMoved() {
			if err := s.boards.reopen(ctx); err != nil {
				// the board keeps the last file that opened; the pages say why
				s.log.Warn(".git-board.toml changed, and the board can't be opened again", "error", err)
				s.publish(ctx, Event{Kind: "config-error", Error: err.Error()})
			} else {
				s.publish(ctx, Event{Kind: "config"})
			}
		}
		s.heads(ctx, true)
	}
}

// heads tells what the last tick found and no request told, then reads
// the datastore's heads: what moved is told next tick, what is gone now.
// Without tell, it takes them as told: the start.
func (s *Server) heads(ctx context.Context, tell bool) {
	var told []Event
	s.seen.mu.Lock()
	for k, v := range s.seen.pending {
		if s.seen.at[k] != v {
			s.seen.at[k] = v
			kind, id, _ := cut(k, "/")
			told = append(told, Event{Kind: kind, ID: id, Version: v})
		}
	}
	clear(s.seen.pending)
	s.seen.mu.Unlock()
	for _, e := range told {
		s.publish(ctx, e)
	}
	h := s.boards.get()
	defer s.boards.release(h)
	hs, err := h.b.Kernel.Store().Heads(ctx, watchedKinds...)
	if err != nil {
		s.log.Debug("reading the heads", "error", err)
		return
	}
	now := make(map[string]bool, len(hs))
	var gone []string
	s.seen.mu.Lock()
	for _, x := range hs {
		k := x.Kind + "/" + x.ID
		now[k] = true
		switch {
		case !tell:
			s.seen.at[k] = x.Version
		case s.seen.at[k] != x.Version:
			s.seen.pending[k] = x.Version
		}
	}
	for k := range s.seen.at {
		if !now[k] {
			gone = append(gone, k)
			delete(s.seen.at, k)
		}
	}
	s.seen.mu.Unlock()
	for _, k := range gone {
		kind, id, _ := cut(k, "/")
		s.publish(ctx, Event{Kind: kind, ID: id})
	}
}

// ownConfig is the server's own writes of .git-board.toml: the watcher
// takes the file they leave as known, so a write through the API opens
// the board and is told once, by its page.
type ownConfig struct {
	mu  sync.Mutex // held while the server writes the file
	sum string     // the file's version as last known
}

// configMoved says whether .git-board.toml moved since the server last
// knew it; while the server writes it, not yet.
func (s *Server) configMoved() bool {
	if !s.own.mu.TryLock() {
		return false
	}
	defer s.own.mu.Unlock()
	f := s.fileSum()
	moved := f != s.own.sum
	s.own.sum = f
	return moved
}

// writeConfig runs write, the server's write of .git-board.toml, then
// opens the board again on it and tells the pages, once, by the page
// that wrote. The board opens again even when write failed, in case it
// wrote before it did.
func (s *Server) writeConfig(ctx context.Context, write func() error) error {
	s.own.mu.Lock()
	before := s.fileSum()
	err := write()
	after := s.fileSum()
	s.own.sum = after
	s.own.mu.Unlock()
	if after == before && err != nil {
		return err
	}
	if rerr := s.boards.reopen(ctx); rerr != nil {
		return cmpErr(err, rerr)
	}
	s.publish(ctx, Event{Kind: "config", By: clientOf(ctx)})
	return err
}

func cmpErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

// fileSum is .git-board.toml's version: a hash of its text, "" for none.
func (s *Server) fileSum() string {
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return ""
	}
	return app.TextVersion(data)
}
