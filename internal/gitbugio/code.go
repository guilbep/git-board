package gitbugio

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/git-bug/git-bug/entity"
	"github.com/git-bug/git-bug/repository"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// The repository's code, beside its issues: branches and commits, through
// git-bug's repository (go-git), as the issues are read.

// Branch is a local branch, or a remote's as last fetched.
type Branch struct {
	Name   string
	Remote string // "" for a local branch
	Hash   string
}

// Branches lists the local branches, then the remotes' (refs/remotes),
// without git-bug's own copies of its issues and identities there.
func (b *Backend) Branches() ([]Branch, error) {
	local, err := b.repo.ListRefs("heads")
	if err != nil {
		return nil, err
	}
	remote, err := b.repo.ListRefs("remotes")
	if err != nil {
		return nil, err
	}
	var res []Branch
	for name, h := range local {
		res = append(res, Branch{Name: name, Hash: h.String()})
	}
	// a remote's HEAD is symbolic: never listed
	for key, h := range remote {
		r, name, ok := strings.Cut(key, "/")
		if !ok || entity.Id(filepath.Base(name)).Validate() == nil { // refs/remotes/origin/bugs/<id>
			continue
		}
		res = append(res, Branch{Name: name, Remote: r, Hash: h.String()})
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].Name != res[j].Name {
			return res[i].Name < res[j].Name
		}
		return res[i].Remote < res[j].Remote
	})
	return res, nil
}

// RemoteHead is the branch a remote's HEAD names, its default branch as
// the clone recorded it; "" when unknown.
func (b *Backend) RemoteHead(remote string) string {
	// ponytail: the repository's refs skip symbolic ones, and git never
	// packs them, so the loose file is the ref; a linked worktree (refs in
	// the common directory) reads "" and falls back to main/master/trunk
	gitDir := filepath.Dir(b.c.LocalStorage().Root())
	data, err := os.ReadFile(filepath.Join(gitDir, "refs", "remotes", remote, "HEAD"))
	if err != nil {
		return ""
	}
	name, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "ref: refs/remotes/"+remote+"/")
	if !ok {
		return ""
	}
	return name
}

// Ancestors is a commit and every commit before it. Read with go-git, not
// git-bug's ListCommits: that walks through ReadCommit, which decodes every
// signature as OpenPGP armor and fails with EOF on an SSH-signed commit
// (gpg.format ssh), as a repository's own commits may be.
func (b *Backend) Ancestors(hash string) (map[string]bool, error) {
	if b.dir == "" { // NewForRepo: no git directory to open, git-bug's own commits only
		list, err := b.repo.ListCommits(context.Background(), repository.Hash(hash))
		if err != nil {
			return nil, err
		}
		res := make(map[string]bool, len(list))
		for _, h := range list {
			res[h.String()] = true
		}
		return res, nil
	}
	r, err := b.codeRepo()
	if err != nil {
		return nil, err
	}
	head, err := r.CommitObject(plumbing.NewHash(hash))
	if err != nil {
		return nil, err
	}
	res := map[string]bool{}
	err = object.NewCommitPreorderIter(head, nil, nil).ForEach(func(c *object.Commit) error {
		res[c.Hash.String()] = true
		return nil
	})
	return res, err
}

// codeRepo is the repository opened with go-git, once: opening it again
// for each branch would read its pack indexes again each time.
func (b *Backend) codeRepo() (*git.Repository, error) {
	b.codeOnce.Do(func() { b.code, b.codeErr = git.PlainOpen(b.dir) })
	return b.code, b.codeErr
}

// Commit is a commit as a branch shows its last one.
type Commit struct {
	SHA     string    `json:"sha"`
	Subject string    `json:"subject"`
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
}

// ReadCommit reads one commit, by hash.
func (b *Backend) ReadCommit(hash string) (Commit, error) {
	log, err := b.repo.CommitLog(context.Background(), hash, "", 1, "", nil, nil)
	if err != nil {
		return Commit{}, err
	}
	if len(log) == 0 {
		return Commit{}, repository.ErrNotFound
	}
	c := log[0]
	subject, _, _ := strings.Cut(c.Message, "\n")
	return Commit{SHA: c.Hash.String(), Subject: strings.TrimSpace(subject), Author: c.AuthorName, Date: c.Date}, nil
}

// ChangesIn tells whether branch's changes are all in base already: what
// `git merge-tree` makes of the two is base's own tree. That's how a branch
// squashed or rebased into base shows, which ancestry can't tell.
// ponytail: runs git (2.38+), which leaves the merged tree's objects
// unreferenced until git gc; false whenever git can't say
func (b *Backend) ChangesIn(base, branch string) bool {
	if b.dir == "" {
		return false
	}
	merged, err := b.git("merge-tree", "--write-tree", base, branch)
	if err != nil { // a conflict, or an older git
		return false
	}
	tree, err := b.git("rev-parse", base+"^{tree}")
	if err != nil {
		return false
	}
	first, _, _ := bytes.Cut(merged, []byte("\n"))
	return bytes.Equal(bytes.TrimSpace(first), bytes.TrimSpace(tree))
}

// Worktree is a linked worktree of the repository (git worktree list).
type Worktree struct {
	Path   string `json:"path"`
	Head   string `json:"head"`
	Branch string `json:"branch,omitempty"` // "" when detached
}

// Worktrees lists the linked worktrees, the main one left out.
func (b *Backend) Worktrees() ([]Worktree, error) {
	out, err := b.git("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var res []Worktree
	var cur Worktree
	first, bare := true, false
	flush := func() {
		if cur.Path != "" && !first && !bare {
			res = append(res, cur)
		}
		cur, first, bare = Worktree{}, false, false
	}
	for _, l := range lines(out) {
		switch k, v, _ := strings.Cut(l, " "); k {
		case "worktree":
			cur.Path = v
		case "HEAD":
			cur.Head = v
		case "branch":
			cur.Branch = strings.TrimPrefix(v, "refs/heads/")
		case "bare":
			bare = true
		case "":
			flush()
		}
	}
	flush()
	return res, nil
}

// git runs git on the repository; nil, nil without a git directory (a
// NewForRepo backend), so every reader below answers empty there.
func (b *Backend) git(args ...string) ([]byte, error) {
	if b.dir == "" {
		return nil, nil
	}
	return exec.Command("git", append([]string{"--git-dir", b.dir}, args...)...).Output()
}

func lines(out []byte) []string {
	if s := strings.TrimRight(string(out), "\n"); s != "" {
		return strings.Split(s, "\n")
	}
	return nil
}

// ChangedFiles is `git diff --name-only base...branch`: the files branch
// changed since it left base.
func (b *Backend) ChangedFiles(base, branch string) ([]string, error) {
	out, err := b.git("diff", "--name-only", base+"..."+branch)
	return lines(out), err
}

// FirstCommitAdding is the first commit of base..branch that added path,
// "" when none did.
func (b *Backend) FirstCommitAdding(base, branch, path string) string {
	out, _ := b.git("log", "--reverse", "--format=%H", "--diff-filter=A", base+".."+branch, "--", path)
	if l := lines(out); len(l) > 0 {
		return l[0]
	}
	return ""
}

// GrepFiles lists the files under dir at ref whose content matches the
// extended regexp, paths from the root: `git grep -l -E`.
func (b *Backend) GrepFiles(ref, dir, regexp string) ([]string, error) {
	out, err := b.git("grep", "-l", "-E", "-e", regexp, ref, "--", dir)
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
func (b *Backend) ReadFile(ref, path string) ([]byte, error) {
	return b.git("show", ref+":"+path)
}
