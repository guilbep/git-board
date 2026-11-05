// Package code is a repository's code, beside its board: branches and
// commits, read with the git binary (ADR 0021: reads through git, not
// go-git). The tools read it: branch names, the release runner, the
// cycle's evidence.
package code

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Repo is a repository's code, at its work tree.
type Repo struct {
	Dir string // the work tree, or any folder inside it
	// Kept, when set, keeps the reads that name no commit (the branches,
	// a remote's HEAD, a ref's merges) for the Repo's copies to share: one
	// page about many issues reads them once. Nil reads git each time.
	Kept *Kept
}

// Kept is what a Repo read once, by the read and its arguments.
type Kept struct {
	mu sync.Mutex
	m  map[string]any
}

// NewKept is an empty Kept.
func NewKept() *Kept { return &Kept{m: map[string]any{}} }

// kept is read's answer for key, read once when r keeps its reads.
func kept[T any](r Repo, key string, read func() (T, error)) (T, error) {
	if r.Kept == nil {
		return read()
	}
	r.Kept.mu.Lock()
	defer r.Kept.mu.Unlock()
	if v, ok := r.Kept.m[key]; ok {
		return v.(T), nil
	}
	v, err := read()
	if err == nil {
		r.Kept.m[key] = v
	}
	return v, err
}

// Branch is a local branch, or a remote's as last fetched.
type Branch struct {
	Name   string
	Remote string // "" for a local branch
	Hash   string
}

// gitBugID is a git-bug entity id, 64 hex digits.
var gitBugID = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Branches lists the local branches, then the remotes' (refs/remotes),
// sorted by name then remote, without the copies of git-bug's issues and
// identities an older git-board left there (refs/remotes/origin/bugs/<id>).
// A remote's HEAD is symbolic: never listed.
func (r Repo) Branches(ctx context.Context) ([]Branch, error) {
	all, err := kept(r, "branches", func() ([]Branch, error) { return r.branches(ctx) })
	return slices.Clone(all), err
}

func (r Repo) branches(ctx context.Context) ([]Branch, error) {
	out, err := r.git(ctx, "for-each-ref", "--format=%(refname) %(objectname) %(symref)", "refs/heads/", "refs/remotes/")
	if err != nil {
		return nil, err
	}
	var res []Branch
	for _, l := range lines(out) {
		f := strings.Fields(l)
		if len(f) < 2 || len(f) > 2 { // a symbolic ref has its target third
			continue
		}
		if name, ok := strings.CutPrefix(f[0], "refs/heads/"); ok {
			res = append(res, Branch{Name: name, Hash: f[1]})
			continue
		}
		rem, name, ok := strings.Cut(strings.TrimPrefix(f[0], "refs/remotes/"), "/")
		if !ok || gitBugID.MatchString(filepath.Base(name)) {
			continue
		}
		res = append(res, Branch{Name: name, Remote: rem, Hash: f[1]})
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].Name != res[j].Name {
			return res[i].Name < res[j].Name
		}
		return res[i].Remote < res[j].Remote
	})
	return res, nil
}

// HasBranch tells whether a local branch, or origin's, has that name.
func (r Repo) HasBranch(ctx context.Context, name string) bool {
	all, err := r.Branches(ctx)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(all, func(br Branch) bool { return br.Name == name && (br.Remote == "" || br.Remote == "origin") })
}

// RemoteHead is the branch a remote's HEAD names, its default branch as
// the clone recorded it; "" when unknown.
func (r Repo) RemoteHead(ctx context.Context, remote string) string {
	name, _ := kept(r, "head "+remote, func() (string, error) {
		out, err := r.git(ctx, "symbolic-ref", "-q", "refs/remotes/"+remote+"/HEAD")
		if err != nil {
			return "", nil // none: kept as none
		}
		name, _ := strings.CutPrefix(strings.TrimSpace(string(out)), "refs/remotes/"+remote+"/")
		return name, nil
	})
	return name
}

// Ancestors is a commit and every commit before it.
func (r Repo) Ancestors(ctx context.Context, hash string) (map[string]bool, error) {
	out, err := r.git(ctx, "rev-list", "--end-of-options", hash)
	if err != nil {
		return nil, err
	}
	res := map[string]bool{}
	for _, l := range lines(out) {
		res[l] = true
	}
	return res, nil
}

// Commit is a commit as a branch shows its last one.
type Commit struct {
	SHA     string    `json:"sha"`
	Subject string    `json:"subject"`
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
}

// ReadCommit reads one commit, by hash.
func (r Repo) ReadCommit(ctx context.Context, hash string) (Commit, error) {
	out, err := r.git(ctx, "log", "-1", "--format=%H%x00%an%x00%aI%x00%s", "--end-of-options", hash, "--")
	if err != nil {
		return Commit{}, err
	}
	f := strings.SplitN(strings.TrimRight(string(out), "\n"), "\x00", 4)
	if len(f) != 4 {
		return Commit{}, fmt.Errorf("git log %s: %q", hash, out)
	}
	at, err := time.Parse(time.RFC3339, f[2])
	if err != nil {
		return Commit{}, err
	}
	return Commit{SHA: f[0], Author: f[1], Date: at, Subject: strings.TrimSpace(f[3])}, nil
}

// ChangesIn tells whether branch's changes are all in base already: what
// `git merge-tree` makes of the two is base's own tree. That's how a branch
// squashed or rebased into base shows, which ancestry can't tell. False
// whenever git can't say (a conflict, a git before 2.38).
func (r Repo) ChangesIn(ctx context.Context, base, branch string) bool {
	merged, err := r.git(ctx, "merge-tree", "--write-tree", base, branch)
	if err != nil {
		return false
	}
	tree, err := r.git(ctx, "rev-parse", base+"^{tree}")
	if err != nil {
		return false
	}
	first, _, _ := bytes.Cut(merged, []byte("\n"))
	return bytes.Equal(bytes.TrimSpace(first), bytes.TrimSpace(tree))
}

// MergeSubjects are the subjects of the last max merge commits on ref's
// first-parent line, where a merged branch's name stays once the branch
// is deleted; none when git can't read them.
func (r Repo) MergeSubjects(ctx context.Context, ref string, max int) []string {
	subjects, _ := kept(r, fmt.Sprintf("merges %d %s", max, ref), func() ([]string, error) {
		out, err := r.git(ctx, "log", "--first-parent", "--merges", "--format=%s", "-n", strconv.Itoa(max), "--end-of-options", ref, "--")
		if err != nil {
			return nil, nil // none: kept as none
		}
		return lines(out), nil
	})
	return slices.Clone(subjects)
}

// CreateBranch makes a local branch at start, tracking nothing: git
// branch --no-track. It never switches: the work tree keeps its checkout.
func (r Repo) CreateBranch(ctx context.Context, name, start string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", r.Dir, "branch", "--no-track", "--end-of-options", name, start)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git branch %s: %s", name, strings.TrimSpace(string(out)))
	}
	return nil
}

// PushBranch pushes a branch to a remote and makes it the branch's
// upstream: git push -u, with the credentials git finds, never a prompt.
func (r Repo) PushBranch(ctx context.Context, remote, name string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", r.Dir, "push", "-u", remote, "--end-of-options", "refs/heads/"+name+":refs/heads/"+name)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push %s %s: %s", remote, name, strings.TrimSpace(string(out)))
	}
	return nil
}

// ChangedFiles is `git diff --name-only base...branch`: the files branch
// changed since it left base.
func (r Repo) ChangedFiles(ctx context.Context, base, branch string) ([]string, error) {
	out, err := r.git(ctx, "diff", "--name-only", base+"..."+branch)
	return lines(out), err
}

// FirstCommitAdding is the first commit of base..branch that added path,
// "" when none did.
func (r Repo) FirstCommitAdding(ctx context.Context, base, branch, path string) string {
	out, _ := r.git(ctx, "log", "--reverse", "--format=%H", "--diff-filter=A", base+".."+branch, "--", path)
	if l := lines(out); len(l) > 0 {
		return l[0]
	}
	return ""
}

// GrepFiles lists the files under dir at ref whose content matches the
// extended regexp, paths from the root: `git grep -l -E`.
func (r Repo) GrepFiles(ctx context.Context, ref, dir, regexp string) ([]string, error) {
	out, err := r.git(ctx, "grep", "-l", "-E", "-e", regexp, ref, "--", dir)
	if ee := (*exec.ExitError)(nil); errors.As(err, &ee) && ee.ExitCode() == 1 {
		return nil, nil // no match
	}
	var res []string
	for _, l := range lines(out) {
		res = append(res, strings.TrimPrefix(l, ref+":"))
	}
	return res, err
}

// ReadFile is `git show ref:path`.
func (r Repo) ReadFile(ctx context.Context, ref, path string) ([]byte, error) {
	return r.git(ctx, "show", ref+":"+path)
}

// git runs git in the repository.
func (r Repo) git(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "git", append([]string{"-C", r.Dir}, args...)...).Output()
}

func lines(out []byte) []string {
	if s := strings.TrimRight(string(out), "\n"); s != "" {
		return strings.Split(s, "\n")
	}
	return nil
}
