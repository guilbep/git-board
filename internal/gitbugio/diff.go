package gitbugio

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// A branch's changes, for the Changes tab (docs/prd/work-unit.md, M4):
// git itself, run on the repository, as ChangesIn does. go-git has no
// rename detection, numstat or attributes.

// ErrNoRepo: the backend has no git directory (an in-memory one).
var ErrNoRepo = errors.New("no git repository")

// run runs git on the repository; revs are checked by the callers (rev),
// paths are names as written, never patterns.
func (b *Backend) run(stdin []byte, args ...string) ([]byte, error) {
	if b.dir == "" {
		return nil, ErrNoRepo
	}
	cmd := exec.Command("git", append([]string{"--git-dir", b.dir, "--literal-pathspecs"}, args...)...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.Output()
	if ee, ok := err.(*exec.ExitError); ok {
		return out, fmt.Errorf("git %s: %w: %s", args[0], err, bytes.TrimSpace(ee.Stderr))
	}
	return out, err
}

// rev refuses a revision git would read as an option.
func rev(revs ...string) error {
	for _, r := range revs {
		if r == "" || strings.HasPrefix(r, "-") {
			return fmt.Errorf("not a revision: %q", r)
		}
	}
	return nil
}

// RevParse is a revision's commit, its full SHA.
func (b *Backend) RevParse(r string) (string, error) {
	if err := rev(r); err != nil {
		return "", err
	}
	out, err := b.run(nil, "rev-parse", "--verify", "--quiet", r+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("no commit %q", r)
	}
	return strings.TrimSpace(string(out)), nil
}

// MergeBase is where two revisions' histories meet.
func (b *Backend) MergeBase(x, y string) (string, error) {
	if err := rev(x, y); err != nil {
		return "", err
	}
	out, err := b.run(nil, "merge-base", x, y)
	return strings.TrimSpace(string(out)), err
}

// HasCommit tells whether the repository has that commit.
func (b *Backend) HasCommit(sha string) bool {
	if rev(sha) != nil {
		return false
	}
	_, err := b.run(nil, "cat-file", "-e", sha+"^{commit}")
	return err == nil
}

// FileStat is one file a diff changes: lines added and deleted.
type FileStat struct {
	Path      string `json:"path"`
	From      string `json:"from,omitempty"` // renamed from
	Added     int    `json:"added"`
	Deleted   int    `json:"deleted"`
	Binary    bool   `json:"binary,omitempty"`
	Generated bool   `json:"generated,omitempty"` // linguist-generated
	Blob      string `json:"blob"`                // the file's blob at to: what "viewed" remembers
}

// DiffStat lists the files changed from one commit to another, renames
// found, and which of them .gitattributes at to marks linguist-generated.
func (b *Backend) DiffStat(from, to string) ([]FileStat, error) {
	if err := rev(from, to); err != nil {
		return nil, err
	}
	out, err := b.run(nil, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "-M", "--raw", "--no-abbrev", "--numstat", "-z", from, to, "--")
	if err != nil {
		return nil, err
	}
	// -z: the --raw entries (":modes blobs status\0path\0[path\0]"), then
	// the --numstat ones ("added\tdeleted\tpath\0", or for a rename
	// "added\tdeleted\t\0from\0to\0"); binary counts are "-"
	tok := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	blobs := map[string]string{}
	var files []FileStat
	for i := 0; i < len(tok); i++ {
		t := tok[i]
		if strings.HasPrefix(t, ":") {
			f := strings.Fields(t)
			if len(f) < 5 || i+1 >= len(tok) {
				return nil, fmt.Errorf("git diff --raw: %q", t)
			}
			path := tok[i+1]
			i++
			if f[4][0] == 'R' || f[4][0] == 'C' {
				path = tok[i+1]
				i++
			}
			blobs[path] = f[3]
			continue
		}
		counts := strings.SplitN(t, "\t", 3)
		if len(counts) != 3 {
			return nil, fmt.Errorf("git diff --numstat: %q", t)
		}
		f := FileStat{Path: counts[2]}
		if f.Path == "" && i+2 < len(tok) {
			f.From, f.Path = tok[i+1], tok[i+2]
			i += 2
		}
		if counts[0] == "-" {
			f.Binary = true
		} else {
			f.Added, _ = strconv.Atoi(counts[0])
			f.Deleted, _ = strconv.Atoi(counts[1])
		}
		f.Blob = blobs[f.Path]
		files = append(files, f)
	}
	if len(files) == 0 {
		return files, nil
	}
	var paths bytes.Buffer
	for _, f := range files {
		paths.WriteString(f.Path + "\x00")
	}
	attrs, err := b.run(paths.Bytes(), "check-attr", "--source="+to, "-z", "--stdin", "linguist-generated")
	if err != nil {
		return nil, err
	}
	// "path\0attribute\0value\0" for each path, in order
	at := strings.Split(string(attrs), "\x00")
	generated := map[string]bool{}
	for i := 0; i+2 < len(at); i += 3 {
		generated[at[i]] = at[i+2] == "set" || at[i+2] == "true"
	}
	for i := range files {
		files[i].Generated = generated[files[i].Path]
	}
	return files, nil
}

// DiffFile is the unified diff of one file from one commit to another,
// cut after limit lines (0: whole); paths are the file and, renamed,
// what it was. ponytail: git's whole output is read before the cut; a
// stream that stops at limit when files of 100k lines show up.
func (b *Backend) DiffFile(from, to string, limit int, paths ...string) (string, bool, error) {
	if err := rev(from, to); err != nil {
		return "", false, err
	}
	out, err := b.run(nil, append([]string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "-M", from, to, "--"}, paths...)...)
	if err != nil {
		return "", false, err
	}
	if limit <= 0 {
		return string(out), false, nil
	}
	end := 0
	for n := 0; n < limit; n++ {
		i := bytes.IndexByte(out[end:], '\n')
		if i < 0 {
			return string(out), false, nil
		}
		end += i + 1
	}
	return string(out[:end]), end < len(out), nil
}

// Log lists the commits from (excluded) to to (included), newest first.
func (b *Backend) Log(from, to string) ([]Commit, error) {
	if err := rev(from, to); err != nil {
		return nil, err
	}
	out, err := b.run(nil, "log", "--no-color", "--format=%H%x1f%an%x1f%aI%x1f%s", from+".."+to)
	if err != nil {
		return nil, err
	}
	res := []Commit{}
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.SplitN(l, "\x1f", 4)
		if len(f) != 4 {
			continue
		}
		at, _ := time.Parse(time.RFC3339, f[2])
		res = append(res, Commit{SHA: f[0], Author: f[1], Date: at, Subject: f[3]})
	}
	return res, nil
}

// FileLines is a file's lines at a commit.
func (b *Backend) FileLines(r, path string) ([]string, error) {
	if err := rev(r); err != nil {
		return nil, err
	}
	out, err := b.run(nil, "show", r+":"+path)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSuffix(string(out), "\n"), "\n"), nil
}

// LineAt is where line of path at from is at to: moved by what changed
// above it, false when a change covers it (the comment on it is
// outdated) or the file is gone.
func (b *Backend) LineAt(from, to, path string, line int) (int, bool, error) {
	if err := rev(from, to); err != nil {
		return 0, false, err
	}
	out, err := b.run(nil, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--no-renames", "-U0", from, to, "--", path)
	if err != nil {
		return 0, false, err
	}
	shift := 0
	for _, l := range strings.Split(string(out), "\n") {
		hunk, ok := strings.CutPrefix(l, "@@ -")
		if !ok {
			continue
		}
		f := strings.Fields(hunk) // "a,b +c,d @@ …"
		if len(f) < 2 {
			continue
		}
		start, count := hunkRange(f[0])
		_, newCount := hunkRange(strings.TrimPrefix(f[1], "+"))
		if count == 0 { // lines added after old line start
			if start >= line {
				break
			}
			shift += newCount
			continue
		}
		if start > line {
			break
		}
		if line < start+count {
			return 0, false, nil
		}
		shift += newCount - count
	}
	return line + shift, true, nil
}

// hunkRange reads "start,count" of a hunk header; a lone start counts one.
func hunkRange(s string) (int, int) {
	a, c, ok := strings.Cut(s, ",")
	start, _ := strconv.Atoi(a)
	if !ok {
		return start, 1
	}
	count, _ := strconv.Atoi(c)
	return start, count
}
