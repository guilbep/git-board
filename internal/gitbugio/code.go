package gitbugio

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/git-bug/git-bug/entity"
	"github.com/git-bug/git-bug/repository"
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

// Ancestors is a commit and every commit before it.
func (b *Backend) Ancestors(hash string) (map[string]bool, error) {
	list, err := b.repo.ListCommits(repository.Hash(hash))
	if err != nil {
		return nil, err
	}
	res := make(map[string]bool, len(list))
	for _, h := range list {
		res[h.String()] = true
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
func (b *Backend) ReadCommit(hash string) (Commit, error) {
	log, err := b.repo.CommitLog(hash, "", 1, "", nil, nil)
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
	merged, err := exec.Command("git", "--git-dir", b.dir, "merge-tree", "--write-tree", base, branch).Output()
	if err != nil { // a conflict, or an older git
		return false
	}
	tree, err := exec.Command("git", "--git-dir", b.dir, "rev-parse", base+"^{tree}").Output()
	if err != nil {
		return false
	}
	first, _, _ := bytes.Cut(merged, []byte("\n"))
	return bytes.Equal(bytes.TrimSpace(first), bytes.TrimSpace(tree))
}
