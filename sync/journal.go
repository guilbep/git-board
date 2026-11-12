package sync

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/guilbep/git-board/store"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// GoOnMargin is how long before the start of the run that read it a forge
// issue must have last changed for the next run to skip it: more than the
// adapters' own margins (GitHub's minute, GitLab's two), for the two
// clocks.
const GoOnMargin = 5 * 60

// Journal is what a run's pull read, kept in the lock file under the
// run's own line, for the run after a cut one to go on from (sync.md's "A
// run"). A line per forge issue read, the later winning, one per listing,
// its total, and one per project a run left quiet, the run's start. It
// is a shortcut: a line it fails to write costs a read next time, nothing
// more.
type Journal struct {
	f       *os.File
	release func() // lets the board's lock go, in a store.Locker
	journal
}

// journal is what a lock file holds.
type journal struct {
	head   journalLine
	read   map[[2]string]int64 // (forge key, forge issue id) -> the start of the run that read it
	same   map[[2]string]bool  // of those, the ones whose pull wrote nothing (Adopt's)
	totals map[string]listing  // forge key -> its last listing
	quiet  map[string]int64    // forge key -> the start of the run that left it quiet
}

// listing is a project's last listing: how many issues it held, and
// whether the pull walked it to its end.
type listing struct {
	total int
	done  bool
}

// journalLine is one line: the run's (PID, Started), an issue read (Forge,
// ID, Run, and Same when its pull wrote nothing), a listing (Forge, Total,
// and Done once walked to its end), or a project left quiet (Forge,
// Since).
type journalLine struct {
	PID     int    `json:"pid,omitempty"`
	Started int64  `json:"started,omitempty"`
	Forge   string `json:"forge,omitempty"`
	ID      string `json:"id,omitempty"`
	Run     int64  `json:"run,omitempty"`
	Same    bool   `json:"same,omitempty"`
	Total   int    `json:"total,omitempty"`
	Done    bool   `json:"done,omitempty"`
	Since   int64  `json:"since,omitempty"`
}

// readJournal reads a lock file's text: its run's line, and what the
// Journal holds. A line it can't read, as a cut write leaves, is skipped.
func readJournal(data []byte) journal {
	j := journal{read: map[[2]string]int64{}, same: map[[2]string]bool{}, totals: map[string]listing{}, quiet: map[string]int64{}}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for first := true; sc.Scan(); first = false {
		var l journalLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		switch k := [2]string{l.Forge, l.ID}; {
		case first:
			j.head = l
		case l.ID != "":
			j.read[k] = l.Run
			if l.Same {
				j.same[k] = true
			} else {
				delete(j.same, k)
			}
		case l.Since != 0:
			j.quiet[l.Forge] = l.Since
		case l.Forge != "":
			j.totals[l.Forge] = listing{l.Total, l.Done}
		}
	}
	return j
}

// openJournal reads the Journal a locked file holds, then writes it back
// under this run's line.
func openJournal(f *os.File, started int64) (*Journal, error) {
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	j := &Journal{f: f, journal: readJournal(data)}
	j.head = journalLine{PID: os.Getpid(), Started: started}
	return j, j.rewrite()
}

// rewrite writes the whole file again: the run's line, then the journal.
// It keeps the file's time, which only an added line moves: Cut's.
func (j *Journal) rewrite() error {
	info, err := j.f.Stat()
	if err != nil {
		return err
	}
	defer os.Chtimes(j.f.Name(), info.ModTime(), info.ModTime())
	var b bytes.Buffer
	for _, l := range j.lines() {
		data, _ := json.Marshal(l)
		b.Write(append(data, '\n'))
	}
	if err := j.f.Truncate(0); err != nil {
		return err
	}
	_, err = j.f.WriteAt(b.Bytes(), 0)
	if err == nil {
		_, err = j.f.Seek(int64(b.Len()), io.SeekStart)
	}
	return err
}

func (j *journal) lines() []journalLine {
	out := []journalLine{j.head}
	for k, l := range j.totals {
		out = append(out, journalLine{Forge: k, Total: l.total, Done: l.done})
	}
	for k, run := range j.read {
		out = append(out, journalLine{Forge: k[0], ID: k[1], Run: run, Same: j.same[k]})
	}
	for k, since := range j.quiet {
		out = append(out, journalLine{Forge: k, Since: since})
	}
	return out
}

func (j *Journal) add(l journalLine) {
	data, _ := json.Marshal(l)
	_, _ = j.f.Write(append(data, '\n'))
}

// Since is the start of the run that read a forge issue, if one did.
func (j *Journal) Since(key, id string) (int64, bool) {
	run, ok := j.read[[2]string{key, id}]
	return run, ok
}

// Reached records a forge issue the pull read, in the run that started at
// run; same says its pull wrote nothing, the board holding it as read.
func (j *Journal) Reached(key, id string, run int64, same bool) {
	k := [2]string{key, id}
	j.read[k] = run
	if same {
		j.same[k] = true
	} else {
		delete(j.same, k)
	}
	j.add(journalLine{Forge: key, ID: id, Run: run, Same: same})
}

// Quiet records that the run that started at since wrote nothing for a
// project: the next lists it from then (QuietSince), as after a run
// recorded in its sync entity, which a quiet run doesn't grow. What the
// Journal held of it is older.
func (j *Journal) Quiet(key string, since int64) {
	j.forget(key)
	j.quiet[key] = since
	_ = j.rewrite()
}

// QuietSince is the start of the run that last left a project quiet, 0
// for none: a listing goes from the later of it and the last recorded run.
func (j *Journal) QuietSince(key string) int64 { return j.quiet[key] }

// Listed records how many issues a project's listing held.
func (j *Journal) Listed(key string, n int) {
	j.totals[key] = listing{total: n}
	j.add(journalLine{Forge: key, Total: n})
}

// Finished records that the pull walked a project's listing to its end.
func (j *Journal) Finished(key string) {
	l := j.totals[key]
	l.done = true
	j.totals[key] = l
	j.add(journalLine{Forge: key, Total: l.total, Done: true})
}

// Drop forgets what the Journal holds of projects whose run is recorded:
// the next run lists them from its start.
func (j *Journal) Drop(keys []string) {
	for _, key := range keys {
		j.forget(key)
		delete(j.quiet, key)
	}
	_ = j.rewrite()
}

// forget drops a project's issues read and its listing.
func (j *journal) forget(key string) {
	for k := range j.read {
		if k[0] == key {
			delete(j.read, k)
			delete(j.same, k)
		}
	}
	delete(j.totals, key)
}

// Keep forgets what the Journal holds of projects other than keys': a
// forge taken out of the sync, or turned off, takes it along.
func (j *Journal) Keep(keys []string) {
	var out []string
	for k := range j.totals {
		if !slices.Contains(keys, k) {
			out = append(out, k)
		}
	}
	for k := range j.read {
		if !slices.Contains(keys, k[0]) {
			out = append(out, k[0])
		}
	}
	for k := range j.quiet {
		if !slices.Contains(keys, k) {
			out = append(out, k)
		}
	}
	if len(out) > 0 {
		j.Drop(out)
	}
}

// Unlock lets the lock go.
func (j *Journal) Unlock() {
	unlockFile(j.f) // first: a process forking as it closes shares the open file, and with it the lock
	j.f.Close()
	if j.release != nil {
		j.release()
	}
}

// CutRun is what the Journal says of runs that recorded no run: when it
// last gained a line, and the forges whose listing the pull didn't walk to
// its end.
type CutRun struct {
	At     time.Time
	Forges []CutForge
}

// CutForge is how many of its last listing's issues a forge's pulls read.
type CutForge struct {
	Key         string
	Read, Total int
}

// Cut is the run cut before it went through, from its journal: nil when
// there is none, or while a run holds the lock.
func Cut(s store.Datastore) (*CutRun, error) {
	f, err := os.OpenFile(filepath.Join(s.Folder(), "sync.lock"), os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := tryLock(f); errors.Is(err, errHeld) {
		return nil, nil // a run goes on: it says where it is
	} else if err != nil {
		return nil, err
	}
	defer unlockFile(f)
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	jd := readJournal(data)
	read, totals := jd.read, jd.totals
	c := &CutRun{At: info.ModTime()}
	for _, key := range slices.Sorted(maps.Keys(totals)) {
		if totals[key].done {
			continue
		}
		n := 0
		for k := range read {
			if k[0] == key {
				n++
			}
		}
		if n < totals[key].total {
			c.Forges = append(c.Forges, CutForge{Key: key, Read: n, Total: totals[key].total})
		}
	}
	if len(c.Forges) == 0 {
		return nil, nil
	}
	return c, nil
}

// Adopt hands board what a check on its copy, scratch, found (the copy's
// journal, its run done): the projects it left quiet, listed from its
// start next time, and the issues its pull found the board holding as
// listed, which the next run skips while unchanged (goesOn). Nothing
// else: what the check pulled into the copy isn't on the board. was is
// board's journal as the copy took it: after a run or a reset since, or
// with a run holding the lock now, the check's word is old, and nothing
// is adopted.
// ponytail: after a check that found changes, the sync still lists from
// the last recorded run, and reads only the issues that changed; the
// listing itself isn't kept.
func Adopt(board, scratch store.Datastore, was []byte) error {
	data, err := os.ReadFile(filepath.Join(scratch.Folder(), "sync.lock"))
	if err != nil {
		return nil // the copy's run journaled nothing
	}
	checked := readJournal(data)
	started := checked.head.Started
	var same [][2]string
	for k, run := range checked.read {
		if checked.same[k] && run == started {
			same = append(same, k)
		}
	}
	quiet := map[string]int64{}
	for key, since := range checked.quiet {
		if since == started {
			quiet[key] = since
		}
	}
	if started == 0 || len(same)+len(quiet) == 0 {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(board.Folder(), "sync.lock"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := tryLock(f); errors.Is(err, errHeld) {
		return nil // a run goes on: it reads for itself
	} else if err != nil {
		return err
	}
	defer unlockFile(f)
	now, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	if !bytes.Equal(now, was) {
		return nil
	}
	j := &Journal{f: f, journal: readJournal(now)}
	for _, k := range same {
		j.read[k], j.same[k] = started, true
	}
	for key, since := range quiet {
		j.forget(key)
		j.quiet[key] = since
	}
	return j.rewrite()
}

// Forget empties the Journal, as a reset of the board's refs must: what it
// says was pulled went with them. ErrLocked while a run holds the lock.
func Forget(s store.Datastore) error {
	path := filepath.Join(s.Folder(), "sync.lock")
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	if err := takeLock(f); errors.Is(err, errHeld) {
		return fmt.Errorf("%w (process %s): %s", ErrLocked, holder(path), path)
	} else if err != nil {
		return err
	}
	defer unlockFile(f)
	return f.Truncate(0)
}
