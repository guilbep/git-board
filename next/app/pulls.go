package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/next/forge"
)

// The cycle's pull requests (v1's internal/server pullCache): each
// branch's PR on origin's forge, as last read through the forge layer
// (forge.Puller), never stored. The server keeps one cache for its life;
// the cycle reads it (cycle.Pulls), and offline the last answer stands.

// PullsEvery is how long a branch's PR, as read, stands before an
// issue's cycle asks the forge again.
const PullsEvery = 5 * time.Minute

// pullsWithin bounds what one issue's read waits for the forge.
const pullsWithin = 10 * time.Second

// ErrNoPuller is a board whose origin is on no forge git-board reads
// pull requests from.
var ErrNoPuller = &Error{Class: Usage, Err: errors.New("origin isn't on GitHub nor on a GitLab: no pull requests to read or write")}

type cachedPull struct {
	pr        *forge.PR
	at, tried time.Time // at: the last answer (zero: never); tried: the last ask
}

// PullCache is each branch's PR on origin's forge. Its zero value is
// unusable: NewPullCache.
type PullCache struct {
	// Open opens the forge the PRs are on; nil is origin's (OriginPuller).
	// Tests give a fake.
	Open func(ctx context.Context, b *Board) (forge.Puller, error)

	mu     sync.Mutex
	m      map[string]cachedPull
	forge  forge.Puller // opened once
	failed time.Time    // when opening it last failed: no retry before PullsEvery
	now    func() time.Time
	gen    uint64 // counts the PRs put: rows computed from the cache key on it
}

// Gen changes whenever a PR the cache holds does: what was computed from
// an older Gen may be out of date.
func (c *PullCache) Gen() uint64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

// NewPullCache is an empty cache.
func NewPullCache() *PullCache {
	return &PullCache{m: map[string]cachedPull{}, now: time.Now}
}

// PR is branch's PR as last read, and when; nil for none or never read.
func (c *PullCache) PR(branch string) (*forge.PR, time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.m[branch]
	return e.pr, e.at
}

// Stale has every branch asked again at its next read: a sync or a fetch
// moved the branches, the forge may have moved the PRs.
func (c *PullCache) Stale() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.m {
		e.tried = time.Time{}
		c.m[k] = e
	}
	c.failed = time.Time{}
}

// put records an answer; err keeps the last PR.
func (c *PullCache) put(branch string, pr *forge.PR, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.m[branch]
	e.tried = c.now()
	if err == nil {
		e.pr, e.at = pr, e.tried
		c.gen++
	}
	c.m[branch] = e
}

func (c *PullCache) stale(branch string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now().Sub(c.m[branch].tried) >= PullsEvery
}

// puller is the forge, opened once; nil with no error when it can't be
// opened now (no forge, no token): the cycle computes without PRs.
func (c *PullCache) puller(ctx context.Context, b *Board) (forge.Puller, error) {
	c.mu.Lock()
	f, failed := c.forge, c.failed
	c.mu.Unlock()
	if f != nil {
		return f, nil
	}
	if !failed.IsZero() && c.now().Sub(failed) < PullsEvery {
		return nil, ErrNoPuller
	}
	open := c.Open
	if open == nil {
		open = OriginPuller
	}
	f, err := open(ctx, b)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.failed = c.now()
		return nil, err
	}
	c.forge = f
	return f, nil
}

// Refresh asks the forge for the PR of each of names whose answer is
// older than PullsEvery (force: every one), within 10 seconds, and says
// whether an answer changed. Without a forge or a token it asks nothing.
// ponytail: one branch after the other, as v1; an issue has one to three.
func (c *PullCache) Refresh(ctx context.Context, b *Board, names []string, force bool) bool {
	var ask []string
	for _, n := range names {
		if n != "" && (force || c.stale(n)) {
			ask = append(ask, n)
		}
	}
	if len(ask) == 0 {
		return false
	}
	f, err := c.puller(ctx, b)
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, pullsWithin)
	defer cancel()
	changed := false
	for _, n := range ask {
		was, _ := c.PR(n)
		pr, err := f.FindPR(ctx, n)
		c.put(n, pr, err)
		changed = changed || err == nil && !samePR(was, pr)
	}
	return changed
}

// Put is a PR the app wrote (opened, edited, merged): the cache takes it
// at once, the next read finds it again.
func (c *PullCache) Put(branch string, pr *forge.PR) { c.put(branch, pr, nil) }

func samePR(a, b *forge.PR) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Number == b.Number && a.State == b.State && a.Draft == b.Draft && a.Head == b.Head && a.Decision == b.Decision &&
		a.MergeState == b.MergeState && a.Title == b.Title && a.Body == b.Body && len(a.Checks) == len(b.Checks)
}

// OriginPuller is origin's forge, opened through its adapter with the
// writer's token: GitHub or a GitLab, as origin's URL names it.
func OriginPuller(ctx context.Context, b *Board) (forge.Puller, error) {
	out, _ := gitRun(ctx, b.Dir, "remote", "get-url", NumbersRemote)
	t, _ := forge.FromRemote(NoCredentials(strings.TrimSpace(out)))
	if t.Kind == "" {
		return nil, ErrNoPuller
	}
	t.Name, t.Sync = t.Kind, true
	project, err := resolveProject(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("opening %s %s: %w", t.Host, t.Project, err)
	}
	t.Project = project
	fs, err := b.OpenForges(ctx, []forge.Table{t})
	if err != nil {
		return nil, err
	}
	p, ok := fs[0].Forge.(forge.Puller)
	if !ok {
		return nil, ErrNoPuller
	}
	return p, nil
}

// refreshing is the cache as one issue's read sees it: a branch's PR
// asked again when stale (force: always) as Gather reads it.
type refreshing struct {
	ctx   context.Context
	b     *Board
	c     *PullCache
	force bool
}

func (r refreshing) PR(branch string) (*forge.PR, time.Time) {
	r.c.Refresh(r.ctx, r.b, []string{branch}, r.force)
	return r.c.PR(branch)
}
