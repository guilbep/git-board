package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
)

// The PR cache asks the forge once an answer is PullsEvery old, or after
// Stale (a sync, a fetch), or forced; a failed ask keeps the last PR; a
// forge that doesn't open is asked again only after PullsEvery.
func TestPullCache(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	f := fake.New("github.com/o/r")
	f.AddPR("feat/x", forge.PR{Head: "abc"})
	now := time.Unix(1_700_000_000, 0)
	c := NewPullCache()
	c.now = func() time.Time { return now }
	c.Open = func(context.Context, *Board) (forge.Puller, error) { return f, nil }
	if !c.Refresh(ctx, nil, []string{"feat/x"}, false) || f.PRReads() != 1 {
		t.Fatalf("first read: %d", f.PRReads())
	}
	if pr, at := c.PR("feat/x"); pr == nil || pr.Number != 1 || !at.Equal(now) {
		t.Fatalf("cached %+v at %v", pr, at)
	}
	c.Refresh(ctx, nil, []string{"feat/x"}, false)
	if f.PRReads() != 1 {
		t.Fatal("asked again within PullsEvery")
	}
	now = now.Add(PullsEvery)
	if c.Refresh(ctx, nil, []string{"feat/x"}, false) || f.PRReads() != 2 {
		t.Fatalf("after PullsEvery: %d reads, or a change told for the same PR", f.PRReads())
	}
	c.Stale()
	f.SetPR(1, func(p *forge.PR) { p.State = "MERGED" })
	if !c.Refresh(ctx, nil, []string{"feat/x"}, false) || f.PRReads() != 3 {
		t.Fatalf("after Stale: %d reads", f.PRReads())
	}
	c.Refresh(ctx, nil, []string{"feat/x"}, true)
	if f.PRReads() != 4 {
		t.Fatal("force asks nothing")
	}

	// no forge: none asked again before PullsEvery
	opens := 0
	d := NewPullCache()
	d.now = func() time.Time { return now }
	d.Open = func(context.Context, *Board) (forge.Puller, error) { opens++; return nil, errors.New("no token") }
	d.Refresh(ctx, nil, []string{"a"}, false)
	d.Refresh(ctx, nil, []string{"b"}, false)
	if opens != 1 {
		t.Fatalf("opened %d times", opens)
	}
	if pr, _ := d.PR("a"); pr != nil {
		t.Fatalf("a PR without a forge: %+v", pr)
	}
}
