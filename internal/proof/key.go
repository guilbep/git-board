// Package proof keeps check results as proof in git, as ADR 0001 decides
// (docs/adr/0001-check-results-are-signed-git-notes.md): a run in a clean
// checkout, one signed line in a git note per result, the note's address
// the content key, one notes ref per writer. Every read and write shells
// out to git and ssh-keygen; go-git has no notes API.
package proof

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"slices"
	"sort"
	"strings"

	"github.com/guilbep/git-board/internal/config"
)

// git runs git in dir, stdin on its input, and returns its output; the
// error carries git's own message.
func git(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(stdin), &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

// rev refuses a revision git would read as an option.
func rev(r string) error {
	if r == "" || strings.HasPrefix(r, "-") {
		return fmt.Errorf("not a revision: %q", r)
	}
	return nil
}

// File is a tracked file of a commit: its mode, its blob, its path.
type File struct{ Mode, Blob, Path string }

// Files are the tracked files of commit that the globs match (git's glob
// pathspecs, `**/*.go`), sorted by path; no globs is every file. It is
// `git --glob-pathspecs diff-tree -r` from the empty tree, which a
// reviewer's script runs too (docs/guide/configuration.md).
func Files(ctx context.Context, dir, commit string, globs []string) ([]File, error) {
	if err := rev(commit); err != nil {
		return nil, err
	}
	empty, err := git(ctx, dir, nil, "hash-object", "-t", "tree", "/dev/null")
	if err != nil {
		return nil, err
	}
	args := append([]string{"--glob-pathspecs", "diff-tree", "-r", "-z", "--no-renames", strings.TrimSpace(string(empty)), commit, "--"}, globs...)
	out, err := git(ctx, dir, nil, args...)
	if err != nil {
		return nil, err
	}
	// -z: ":000000 <mode> <zero> <blob> A" NUL <path> NUL, per file
	parts := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	var files []File
	for i := 0; i+1 < len(parts); i += 2 {
		f := strings.Fields(parts[i])
		if len(f) != 5 {
			return nil, fmt.Errorf("git diff-tree: unexpected %q", parts[i])
		}
		files = append(files, File{Mode: f[1], Blob: f[3], Path: parts[i+1]})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// Touched lists the files head changed since it left base that the globs
// match: what a suite reads and the branch changed. No globs is every
// changed file.
func Touched(ctx context.Context, dir, base, head string, globs []string) ([]string, error) {
	if err := errors.Join(rev(base), rev(head)); err != nil {
		return nil, err
	}
	out, err := git(ctx, dir, nil, append([]string{"--glob-pathspecs", "diff", "--name-only", "-z", "--no-renames", base + "..." + head, "--"}, globs...)...)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(strings.Split(string(out), "\x00"), func(s string) bool { return s == "" }), nil
}

// KeyText is the canonical text the content key hashes (ADR 0001, section
// 3): the check, its argv, its directory when set, its variables, and the
// files it reads. dir is a ruling of M4: a check run elsewhere is another
// check.
// ponytail: a path holding a newline would break a line; git allows it, no
// project does.
func KeyText(ctx context.Context, dir, commit, name string, s config.Suite) ([]byte, error) {
	files, err := Files(ctx, dir, commit, s.Files)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "git-board-ci/key/v1\ncheck %s\nrun %s\n", name, argv(s.Run))
	if s.Dir != "" {
		fmt.Fprintf(&b, "dir %s\n", s.Dir)
	}
	for _, k := range slices.Sorted(maps.Keys(s.Env)) {
		fmt.Fprintf(&b, "env %s=%s\n", k, s.Env[k])
	}
	for _, f := range files {
		fmt.Fprintf(&b, "file %s %s %s\n", f.Mode, f.Blob, f.Path)
	}
	return b.Bytes(), nil
}

// Key is the content key of a check at a commit: `git hash-object --stdin`
// over KeyText, in the repository's own hash format.
func Key(ctx context.Context, dir, commit, name string, s config.Suite) (string, error) {
	text, err := KeyText(ctx, dir, commit, name, s)
	if err != nil {
		return "", err
	}
	out, err := git(ctx, dir, text, "hash-object", "--stdin")
	return strings.TrimSpace(string(out)), err
}

// argv is the argv as compact JSON, `<` and `&` as written.
func argv(run []string) string {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(run) // a []string always encodes
	return strings.TrimSuffix(b.String(), "\n")
}
