package server

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// On a forge-only board, each write's issue is sent to the forges at once,
// and a send that fails is tried again. It swaps pushRetry: not parallel.
func TestForgeOnlySendsEachWrite(t *testing.T) {
	old := pushRetry
	pushRetry = 10 * time.Millisecond
	t.Cleanup(func() { pushRetry = old })
	dir := storetest.Repo(t, false)
	storetest.Git(t, dir, "config", store.DatastoreKey, store.ForgeOnly)
	storetest.Init(t, dir)
	b := storetest.Open(t, dir)
	s := &apiServer{b: b, cfg: &config.Config{}}
	var mu sync.Mutex
	var sent []string
	fail := 1
	s.runIssueSync = func(_ context.Context, _ []config.Forge, dry bool, id string, _ func(store.SyncEvent)) error {
		mu.Lock()
		defer mu.Unlock()
		if dry {
			t.Error("a dry run: the write goes at once")
		}
		if fail > 0 {
			fail--
			return errors.New("the forge is down")
		}
		sent = append(sent, id)
		return nil
	}
	s.startPushing(t.Context())
	id, err := b.NewIssue("sent at once", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		mu.Lock()
		n := len(sent)
		mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never sent")
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 1 || sent[0] != id {
		t.Fatalf("sent %v; want %s once, after its failure", sent, id)
	}
}
