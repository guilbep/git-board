package syncer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/guilbep/git-board/core/gitstore"
)

// goOnMargin is how long before the start of the run that read it a forge
// issue must have last changed for the next run to skip it: more than the
// adapters' own margins (GitHub's minute, GitLab's two), for the two
// clocks.
const goOnMargin = 5 * 60

// journal is what a run's pull read, kept in the lock file under the
// run's own line, for the run after a cut one to go on from (sync.md's "A
// run"). A line per forge issue read, the later winning, and one per
// listing, its total. It is a shortcut: a line it fails to write costs a
// read next time, nothing more.
type journal struct {
	f      *os.File
	head   journalLine
	read   map[[2]string]int64 // (forge key, forge issue id) -> the start of the run that read it
	totals map[string]int      // forge key -> how many issues its last listing held
}

// journalLine is one line: the run's (PID, Started), an issue read (Forge,
// ID, Run), or a listing (Forge, Total).
type journalLine struct {
	PID     int    `json:"pid,omitempty"`
	Started int64  `json:"started,omitempty"`
	Forge   string `json:"forge,omitempty"`
	ID      string `json:"id,omitempty"`
	Run     int64  `json:"run,omitempty"`
	Total   int    `json:"total,omitempty"`
}

// readJournal reads a lock file's text: its run's line, and what the
// journal holds. A line it can't read, as a cut write leaves, is skipped.
func readJournal(data []byte) (head journalLine, read map[[2]string]int64, totals map[string]int) {
	read, totals = map[[2]string]int64{}, map[string]int{}
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
			totals[l.Forge] = l.Total
		}
	}
	return head, read, totals
}

// openJournal reads the journal a locked file holds, then writes it back
// under this run's line.
func openJournal(f *os.File, started int64) (*journal, error) {
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	_, read, totals := readJournal(data)
	j := &journal{f: f, head: journalLine{PID: os.Getpid(), Started: started}, read: read, totals: totals}
	return j, j.rewrite()
}

// rewrite writes the whole file again: the run's line, then the journal.
func (j *journal) rewrite() error {
	var b bytes.Buffer
	for _, l := range j.lines() {
		data, _ := json.Marshal(l)
		b.Write(append(data, '\n'))
	}
	if err := j.f.Truncate(0); err != nil {
		return err
	}
	_, err := j.f.WriteAt(b.Bytes(), 0)
	if err == nil {
		_, err = j.f.Seek(int64(b.Len()), io.SeekStart)
	}
	return err
}

func (j *journal) lines() []journalLine {
	out := []journalLine{j.head}
	for k, n := range j.totals {
		out = append(out, journalLine{Forge: k, Total: n})
	}
	for k, run := range j.read {
		out = append(out, journalLine{Forge: k[0], ID: k[1], Run: run})
	}
	return out
}

func (j *journal) add(l journalLine) {
	data, _ := json.Marshal(l)
	_, _ = j.f.Write(append(data, '\n'))
}

// since is the start of the run that read a forge issue, if one did.
func (j *journal) since(key, id string) (int64, bool) {
	run, ok := j.read[[2]string{key, id}]
	return run, ok
}

// reached records a forge issue the pull read, in the run that started at
// run.
func (j *journal) reached(key, id string, run int64) {
	j.read[[2]string{key, id}] = run
	j.add(journalLine{Forge: key, ID: id, Run: run})
}

// listed records how many issues a project's listing held.
func (j *journal) listed(key string, n int) {
	j.totals[key] = n
	j.add(journalLine{Forge: key, Total: n})
}

// drop forgets what the journal holds of projects whose run is recorded:
// the next run lists them from its start.
func (j *journal) drop(keys []string) {
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

// unlock lets the lock go.
func (j *journal) unlock() {
	unlockFile(j.f) // first: a process forking as it closes shares the open file, and with it the lock
	j.f.Close()
}

// CutRun is what the journal says of runs that recorded no run: when it
// was last written, and the forges whose listing the pull didn't read to
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
func Cut(s *gitstore.Store) (*CutRun, error) {
	f, err := os.OpenFile(filepath.Join(s.CommonDir(), "board", "sync.lock"), os.O_RDWR, 0)
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
		n := 0
		for k := range read {
			if k[0] == key {
				n++
			}
		}
		if n < totals[key] {
			c.Forges = append(c.Forges, CutForge{Key: key, Read: n, Total: totals[key]})
		}
	}
	if len(c.Forges) == 0 {
		return nil, nil
	}
	return c, nil
}
