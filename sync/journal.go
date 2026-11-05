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
// run"). A line per forge issue read, the later winning, and one per
// listing, its total. It is a shortcut: a line it fails to write costs a
// read next time, nothing more.
type Journal struct {
	f       *os.File
	release func() // lets the board's lock go, in a store.Locker
	head    journalLine
	read    map[[2]string]int64 // (forge key, forge issue id) -> the start of the run that read it
	totals  map[string]listing  // forge key -> its last listing
}

// listing is a project's last listing: how many issues it held, and
// whether the pull walked it to its end.
type listing struct {
	total int
	done  bool
}

// journalLine is one line: the run's (PID, Started), an issue read (Forge,
// ID, Run), or a listing (Forge, Total, and Done once walked to its end).
type journalLine struct {
	PID     int    `json:"pid,omitempty"`
	Started int64  `json:"started,omitempty"`
	Forge   string `json:"forge,omitempty"`
	ID      string `json:"id,omitempty"`
	Run     int64  `json:"run,omitempty"`
	Total   int    `json:"total,omitempty"`
	Done    bool   `json:"done,omitempty"`
}

// readJournal reads a lock file's text: its run's line, and what the
// Journal holds. A line it can't read, as a cut write leaves, is skipped.
func readJournal(data []byte) (head journalLine, read map[[2]string]int64, totals map[string]listing) {
	read, totals = map[[2]string]int64{}, map[string]listing{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for first := true; sc.Scan(); first = false {
		var l journalLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		switch {
		case first:
			head = l
		case l.ID != "":
			read[[2]string{l.Forge, l.ID}] = l.Run
		case l.Forge != "":
			totals[l.Forge] = listing{l.Total, l.Done}
		}
	}
	return head, read, totals
}

// openJournal reads the Journal a locked file holds, then writes it back
// under this run's line.
func openJournal(f *os.File, started int64) (*Journal, error) {
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	_, read, totals := readJournal(data)
	j := &Journal{f: f, head: journalLine{PID: os.Getpid(), Started: started}, read: read, totals: totals}
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

func (j *Journal) lines() []journalLine {
	out := []journalLine{j.head}
	for k, l := range j.totals {
		out = append(out, journalLine{Forge: k, Total: l.total, Done: l.done})
	}
	for k, run := range j.read {
		out = append(out, journalLine{Forge: k[0], ID: k[1], Run: run})
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
// run.
func (j *Journal) Reached(key, id string, run int64) {
	j.read[[2]string{key, id}] = run
	j.add(journalLine{Forge: key, ID: id, Run: run})
}

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
	for k := range j.read {
		for _, key := range keys {
			if k[0] == key {
				delete(j.read, k)
			}
		}
	}
	for _, key := range keys {
		delete(j.totals, key)
	}
	_ = j.rewrite()
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
	_, read, totals := readJournal(data)
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
