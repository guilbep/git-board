package issuesync

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
)

// slowReads is a fake whose reads of an issue take a moment, as a round
// trip does, and which counts how many run at once.
type slowReads struct {
	*fake.Forge
	mu        sync.Mutex
	now, most int
}

func (r *slowReads) Complete(ctx context.Context, is forge.Issue) (forge.Issue, error) {
	r.mu.Lock()
	r.now++
	r.most = max(r.most, r.now)
	r.mu.Unlock()
	time.Sleep(5 * time.Millisecond) // a round trip: the reads overlap, nothing is timed
	defer func() {
		r.mu.Lock()
		r.now--
		r.mu.Unlock()
	}()
	return r.Forge.Complete(ctx, is)
}

// A pull reads its issues DefaultReaders at a time, never more, and pulls
// every one (in the forge's order: it takes the answers by their place):
// a board of a thousand issues no longer waits a thousand round trips in
// a row.
func TestPullReadsAhead(t *testing.T) {
	t.Parallel()
	f := fake.New(key)
	for i := range 20 {
		f.Add(forge.Issue{Author: ann(), Title: fmt.Sprint("i", i), Status: "open", Created: int64(1000 + i)})
	}
	r := &slowReads{Forge: f}
	s := newStore(t)
	rep := syncAll(t, s, r)
	if r.most < 2 || r.most > DefaultReaders {
		t.Errorf("at most %d reads at once, want 2 to %d", r.most, DefaultReaders)
	}
	if rep.Created != 20 {
		t.Errorf("%d pulled, want 20", rep.Created)
	}
	if n := len(allIssues(t, s)); n != 20 {
		t.Errorf("%d issues on the board, want 20", n)
	}
}
