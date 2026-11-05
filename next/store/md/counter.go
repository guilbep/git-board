package md

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/guilbep/git-board/next/store"
)

// The project numbers' counter in a folder (docs/spec/datastores.md, The
// counter in a folder): .board/numbers, written whole, on the default
// branch alone. A branch that numbered too would hand out the default
// branch's numbers again, and no merge could tell which is right; so a
// checkout on another branch reads the counter and refuses to write it,
// saying why. merge=binary keeps a merge from mixing two counters.

const numbersFile = "numbers"

// numbersAttributes is the counter's line in .board/.gitattributes.
const numbersAttributes = "/" + numbersFile + " merge=binary\n"

func (s *Store) numbers() string { return filepath.Join(s.root(), numbersFile) }

// Count reads the counter: nil and "" while none was started.
func (s *Store) Count(context.Context) ([]byte, string, error) {
	data, err := readFile(s.numbers())
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return nil, "", err
	}
	return data, version(data), nil
}

// SetCount writes the counter if its version is still prev, on the
// default branch alone (CountWaits): to numbers.new, renamed over it.
func (s *Store) SetCount(ctx context.Context, value []byte, prev string) (string, error) {
	if err := s.CountWaits(ctx); err != nil {
		return "", err
	}
	path := s.numbers()
	unlock, err := lock(path + ".lock")
	if err != nil {
		return "", err
	}
	defer unlock()
	data, err := readFile(path)
	if err != nil {
		return "", err
	}
	if version(data) != prev {
		return "", fmt.Errorf("%w: %s", store.ErrMoved, numbersFile)
	}
	if err := s.numbersAttributes(); err != nil {
		return "", err
	}
	tmp := path + ".new"
	if err := os.WriteFile(tmp, value, 0o644); err != nil {
		return "", fmt.Errorf("md: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("md: %w", err)
	}
	return version(value), nil
}

// numbersAttributes adds the counter's line to .board/.gitattributes
// when it isn't there: a folder made before the counter has the pack
// files' line alone.
func (s *Store) numbersAttributes() error {
	path := filepath.Join(s.root(), ".gitattributes")
	data, err := readFile(path)
	if err != nil || bytes.Contains(data, []byte(numbersAttributes)) {
		return err
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	if err := os.WriteFile(path, append(data, numbersAttributes...), 0o644); err != nil {
		return fmt.Errorf("md: %w", err)
	}
	return nil
}

// CountWaits says why the counter can't be written here, nil when it
// can: the checkout isn't on the default branch, origin's HEAD, else
// main, else master. It wraps store.ErrMissing, with the sentence a user
// reads. A folder outside any git checkout has no branch, and writes.
func (s *Store) CountWaits(ctx context.Context) error {
	if _, err := s.git(ctx, "rev-parse", "--is-inside-work-tree"); err != nil {
		return nil
	}
	def := s.defaultBranch(ctx)
	current, err := s.git(ctx, "symbolic-ref", "-q", "--short", "HEAD")
	var why string
	switch {
	case err != nil:
		why = fmt.Sprintf("project numbers are handed out on %s alone: this checkout is on no branch. Numbering waits until it is on %s", def, def)
	case def == "":
		if current == "main" || current == "master" { // a new repository: its branch, unborn
			return nil
		}
		why = "project numbers are handed out on the default branch alone, and this repository has none: origin names no HEAD, and there is no main nor master. This checkout is on " + current
	case current != def:
		why = fmt.Sprintf("project numbers are handed out on %s alone: this checkout is on %s. Its issues take their numbers once merged into %s", def, current, def)
	default:
		return nil
	}
	return fmt.Errorf("%w: %s", store.ErrMissing, why)
}

// defaultBranch is origin's HEAD, else main, else master, as branches
// here; "" for none of them.
func (s *Store) defaultBranch(ctx context.Context) string {
	if head, err := s.git(ctx, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if b, ok := strings.CutPrefix(head, "origin/"); ok && b != "" {
			return b
		}
	}
	for _, b := range []string{"main", "master"} {
		if _, err := s.git(ctx, "show-ref", "--verify", "-q", "refs/heads/"+b); err == nil {
			return b
		}
	}
	return ""
}

// git runs git in the folder, and answers its output, trimmed.
func (s *Store) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", s.dir}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		err = fmt.Errorf("git %s: %w: %s", args[0], err, bytes.TrimSpace(ee.Stderr))
	}
	return strings.TrimSpace(string(out)), err
}
