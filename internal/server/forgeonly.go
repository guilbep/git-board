package server

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/guilbep/git-board/internal/store"
)

// A forge-only board (ADR 0002, section 4) sends each write to the forges
// at once: the write lands in the copy, then its issue's sync runs, as the
// Sync now of one issue does. A send that fails is tried again later; until
// it lands, the issue shows what waits to go, as any write that waits does.

// pushRetry is how long a failed send waits before its next try.
var pushRetry = 30 * time.Second

// pusher sends forge-only writes, one issue at a time, in the order they
// came.
type pusher struct {
	mu      sync.Mutex
	pending map[string]bool
	order   []string
	wake    chan struct{}
	retry   time.Duration // pushRetry, as it was when pushing started
}

// startPushing has every write here sent to the forges at once, until ctx
// ends.
func (s *apiServer) startPushing(ctx context.Context) {
	p := &pusher{pending: map[string]bool{}, wake: make(chan struct{}, 1), retry: pushRetry}
	s.b.OnWrite = func(id string) {
		p.mu.Lock()
		if !p.pending[id] {
			p.pending[id] = true
			p.order = append(p.order, id)
		}
		p.mu.Unlock()
		select {
		case p.wake <- struct{}{}:
		default:
		}
	}
	go s.push(ctx, p)
}

// push sends what waits, each time a write wakes it, and again after
// pushRetry while some failed.
func (s *apiServer) push(ctx context.Context, p *pusher) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.wake:
		case <-time.After(p.retry):
		}
		for {
			p.mu.Lock()
			if len(p.order) == 0 {
				p.mu.Unlock()
				break
			}
			id := p.order[0]
			p.order = p.order[1:]
			delete(p.pending, id)
			p.mu.Unlock()
			if err := s.pushOne(ctx, id); err != nil {
				log.Printf("forge only: sending %s: %v; trying again in %s", id, err, p.retry)
				p.mu.Lock()
				if !p.pending[id] {
					p.pending[id] = true
					p.order = append(p.order, id)
				}
				p.mu.Unlock()
				break
			}
		}
	}
}

// pushOne runs one issue's sync with every sync bridge, under the write
// lock, and tells the pages.
func (s *apiServer) pushOne(ctx context.Context, id string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	run := s.runIssueSync
	if run == nil {
		run = s.issueRun
	}
	err := run(ctx, s.syncForges(), false, id, func(store.SyncEvent) {})
	s.seeRefs()
	s.events.publish(event{Kind: "issues", ID: id})
	return err
}
