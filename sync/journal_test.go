package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/guilbep/git-board/store/mem"
)

// A quiet project's start and an issue read the same stay through the
// next run's lock; a recorded run drops the quiet start.
func TestTheJournalKeepsQuietAndSame(t *testing.T) {
	t.Parallel()
	s := mem.New(t.TempDir())
	j, err := Lock(s, 100)
	if err != nil {
		t.Fatal(err)
	}
	j.Reached("p", "1", 100, true)
	j.Reached("p", "2", 100, false)
	j.Quiet("q", 100)
	j.Unlock()
	if j, err = Lock(s, 200); err != nil {
		t.Fatal(err)
	}
	if got := j.QuietSince("q"); got != 100 {
		t.Errorf("q's quiet start after a lock: %d, want 100", got)
	}
	if !j.same[[2]string{"p", "1"}] || j.same[[2]string{"p", "2"}] {
		t.Errorf("same after a lock: %v, want p 1 alone", j.same)
	}
	j.Quiet("p", 200) // what it held of p is older
	if _, ok := j.Since("p", "1"); ok {
		t.Error("a quiet run kept p's older reads")
	}
	j.Drop([]string{"q"})
	if got := j.QuietSince("q"); got != 0 {
		t.Errorf("q's quiet start after a recorded run: %d", got)
	}
	j.Unlock()
}

// Adopt hands the board what its copy's run found the same, and its
// quiet projects; not when the board's journal moved since the copy.
func TestAdoptTakesWhatTheCheckFoundHere(t *testing.T) {
	t.Parallel()
	board, scratch := mem.New(t.TempDir()), mem.New(t.TempDir())
	was := journalOf(t, board, 50, func(j *Journal) { j.Reached("p", "0", 50, true) })
	journalOf(t, scratch, 100, func(j *Journal) {
		j.Reached("p", "0", 50, true)   // the board's, copied: not the check's
		j.Reached("p", "1", 100, true)  // found here
		j.Reached("p", "2", 100, false) // pulled into the copy alone
		j.Quiet("q", 100)
	})
	if err := Adopt(board, scratch, was); err != nil {
		t.Fatal(err)
	}
	j, err := Lock(board, 300)
	if err != nil {
		t.Fatal(err)
	}
	if run, ok := j.Since("p", "1"); !ok || run != 100 {
		t.Errorf("p 1: %d %t, want read at the check's start", run, ok)
	}
	if _, ok := j.Since("p", "2"); ok {
		t.Error("an issue the check pulled into its copy was adopted")
	}
	if run, _ := j.Since("p", "0"); run != 50 {
		t.Errorf("the board's own line: %d", run)
	}
	if got := j.QuietSince("q"); got != 100 {
		t.Errorf("q's quiet start: %d", got)
	}
	j.Unlock()

	// a run came between: the check's word is old
	board2 := mem.New(t.TempDir())
	was = journalOf(t, board2, 50, nil)
	journalOf(t, board2, 60, nil)
	if err := Adopt(board2, scratch, was); err != nil {
		t.Fatal(err)
	}
	if j, err = Lock(board2, 300); err != nil {
		t.Fatal(err)
	}
	defer j.Unlock()
	if _, ok := j.Since("p", "1"); ok || j.QuietSince("q") != 0 {
		t.Error("adopted after a run came between")
	}
}

// journalOf runs a run that started at started on s, doing do, and
// answers the journal it left.
func journalOf(t *testing.T, s *mem.Store, started int64, do func(*Journal)) []byte {
	t.Helper()
	j, err := Lock(s, started)
	if err != nil {
		t.Fatal(err)
	}
	if do != nil {
		do(j)
	}
	j.Unlock()
	data, err := os.ReadFile(filepath.Join(s.Folder(), "sync.lock"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
