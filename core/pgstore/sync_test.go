package pgstore_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/pgstore"
	"github.com/guilbep/git-board/core/pgstore/pgtest"
	"github.com/guilbep/git-board/core/syncer"
)

// A board kept in PostgreSQL syncs with a forge as a git one does: its
// bookkeeping is sync entities, packs like any other (ADR 0002, section 2).
func TestSyncWithAForge(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := t.Context()
	f := fake.New("example.com/p")
	ann := forge.Account{ID: "7", Login: "ann"}
	f.Add(forge.Issue{ID: "100", Number: "1", URL: "https://example.com/p/1", Author: ann, Created: 1000,
		Title: "T", Body: "B", Status: "open",
		Comments: []forge.Comment{{ID: "c1", Body: "hi", Author: ann, Time: 1100}}})
	if _, err := syncer.Sync(ctx, s, syncer.Options{}, f); err != nil {
		t.Fatal(err)
	}
	ids, err := issue.List(s)
	if err != nil || len(ids) != 1 {
		t.Fatalf("issues %v, %v; want the one pulled", ids, err)
	}
	is, err := issue.Load(s, ids[0])
	if err != nil || is.Title != "T" || len(is.Comments) != 1 {
		t.Fatalf("pulled %+v, %v", is, err)
	}

	// a comment written here goes to the forge, once
	if _, err := issue.Do(ctx, s, is.ID, issue.NewComment(is.Author, "from git-board")); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := syncer.Sync(ctx, s, syncer.Options{}, f); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := f.Get("1")
	if len(got.Comments) != 2 || got.Comments[1].Body == "" {
		t.Fatalf("forge comments %+v; want the one written here, once", got.Comments)
	}
	if is, _ = issue.Load(s, is.ID); len(is.Comments) != 2 {
		t.Fatalf("comments here %d; want 2, no echo", len(is.Comments))
	}
	if ids, _ := issue.List(s); len(ids) != 1 {
		t.Fatalf("issues %v; want no duplicate", ids)
	}
}

// held is a forge whose run waits, at its first read, until let is closed:
// a run caught in the middle.
type held struct {
	forge.Forge
	in  chan struct{} // closed once the run reached the forge
	let chan struct{}
}

func (h *held) Visibility(ctx context.Context) (forge.Visibility, error) {
	close(h.in)
	<-h.let
	return h.Forge.Visibility(ctx)
}

// Two machines on one board in PostgreSQL sync one at a time: the board's
// lock is the database's, not each machine's folder's (issue #83).
func TestTwoMachinesSyncOneAtATime(t *testing.T) {
	t.Parallel()
	dsn := pgtest.DSN(t)
	machine := func() *pgstore.Store {
		s, err := pgstore.Open(t.Context(), dsn, t.TempDir()) // a folder each, as two machines have
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}
	a, b := machine(), machine()
	f := fake.New("example.com/p")
	f.Add(forge.Issue{ID: "100", Number: "1", URL: "https://example.com/p/1", Author: forge.Account{ID: "7", Login: "ann"},
		Created: 1000, Title: "T", Status: "open"})
	h := &held{Forge: f, in: make(chan struct{}), let: make(chan struct{})}
	done := make(chan error)
	go func() {
		_, err := syncer.Sync(t.Context(), a, syncer.Options{}, h)
		done <- err
	}()
	<-h.in
	if !syncer.Running(b) {
		t.Error("the other machine doesn't see the run")
	}
	if _, err := syncer.Sync(t.Context(), b, syncer.Options{}, f); !errors.Is(err, syncer.ErrLocked) {
		t.Errorf("a second machine's sync: %v, want ErrLocked", err)
	}
	close(h.let)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if syncer.Running(b) {
		t.Error("running once the run is over")
	}
	if _, err := syncer.Sync(t.Context(), b, syncer.Options{}, f); err != nil {
		t.Fatalf("a sync once the first is over: %v", err)
	}
	if ids, _ := issue.List(b); len(ids) != 1 {
		t.Fatalf("issues %v; want the one pulled", ids)
	}
}

// holdEnv names the DSN TestHoldTheLock takes the lock on, in a process of
// its own.
const holdEnv = "GIT_BOARD_TEST_HOLD_LOCK"

// TestHoldTheLock is TestAKilledHolderKeepsNoOneOut's process: it takes
// the sync's lock, says so, and waits to be killed.
func TestHoldTheLock(t *testing.T) {
	dsn := os.Getenv(holdEnv)
	if dsn == "" {
		t.Skip("the process TestAKilledHolderKeepsNoOneOut starts")
	}
	s, err := pgstore.Open(t.Context(), dsn, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lock(t.Context(), "sync"); err != nil {
		t.Fatal(err)
	}
	fmt.Println("held")
	select {}
}

// A process killed holding the board's lock keeps no one out: its session
// ends, and the server lets the lock go (issue #83).
func TestAKilledHolderKeepsNoOneOut(t *testing.T) {
	t.Parallel()
	dsn := pgtest.DSN(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestHoldTheLock$")
	cmd.Env = append(os.Environ(), holdEnv+"="+dsn)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "held\n" {
		t.Fatalf("the holder said %q, %v", line, err)
	}
	s, err := pgstore.Open(t.Context(), dsn, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.Lock(t.Context(), "sync"); !errors.Is(err, datastore.ErrHeld) {
		t.Fatalf("lock while another process holds it: %v, want ErrHeld", err)
	}
	if !syncer.Running(s) {
		t.Fatal("not running while another process holds the lock")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	for start := time.Now(); syncer.Running(s); time.Sleep(20 * time.Millisecond) {
		if time.Since(start) > 10*time.Second {
			t.Fatal("the lock outlived its holder's process")
		}
	}
	unlock, err := s.Lock(t.Context(), "sync")
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}
