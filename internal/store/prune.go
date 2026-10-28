package store

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// gitBugRef is a ref of git-bug's: its issues, its identities and the
// fork's release records, here or a remote's copy of them, fetched by id.
// A branch named bugs/x or releases/1.2 isn't one.
var gitBugRef = regexp.MustCompile(`^refs/(bugs|identities|releases)/|^refs/remotes/[^/]+/(bugs|identities|releases)/[0-9a-f]{40,64}$`)

// PruneReport is what a prune found: removed, unless a dry run.
type PruneReport struct {
	Refs   map[string]int // namespace, as "refs/bugs/" -> its refs here
	Cache  string         // git-bug's cache folder, when there
	Keys   []string       // the git-bug.* keys of .git/config
	Remote int            // git-bug's refs on the remote
	Backup string         // the bundle of the refs removed here
}

// Prune removes git-bug's data from a repository switched to git-board:
// the refs gitBugRef matches, its cache .git/git-bug, its git-bug.* keys,
// and, with a remote, its refs there. The refs here go to a bundle in the
// git directory first; nothing backs up the remote's. A repository whose
// issues are still git-bug's is refused: ErrNotSwitched. A dry run finds,
// and removes nothing.
func (b *Backend) Prune(remote string, dryRun bool) (PruneReport, error) {
	rep := PruneReport{Refs: map[string]int{}}
	if err := b.Switched(); err != nil {
		return rep, err
	}
	all, err := b.s.Refs("refs/")
	if err != nil {
		return rep, err
	}
	var refs []string
	for name := range all {
		if gitBugRef.MatchString(name) {
			refs = append(refs, name)
			rep.Refs[name[:strings.LastIndex(name, "/")+1]]++
		}
	}
	slices.Sort(refs)
	cache := filepath.Join(b.s.CommonDir(), "git-bug")
	if _, err := os.Stat(cache); err == nil {
		rep.Cache = cache
	}
	out, err := b.run(nil, "config", "--local", "--name-only", "--get-regexp", `^git-bug\.`)
	if ee := (*exec.ExitError)(nil); errors.As(err, &ee) && ee.ExitCode() == 1 {
		err = nil // no key
	}
	if err != nil {
		return rep, err
	}
	rep.Keys = strings.Fields(string(out))
	var theirs []string
	if remote != "" {
		if err := b.checkRemote(remote); err != nil {
			return rep, err
		}
		out, err := b.run(nil, "ls-remote", "--refs", remote, "refs/bugs/*", "refs/identities/*", "refs/releases/*")
		if err != nil {
			return rep, err
		}
		for line := range strings.Lines(string(out)) {
			if _, name, ok := strings.Cut(strings.TrimSpace(line), "\t"); ok {
				theirs = append(theirs, name)
			}
		}
		rep.Remote = len(theirs)
	}
	if dryRun {
		return rep, nil
	}

	if rep.Backup, err = b.backup("git-bug-backup", refs); err != nil {
		return rep, err
	}
	if err := b.dropRefs(refs, all); err != nil {
		return rep, err
	}
	if rep.Cache != "" {
		if err := os.RemoveAll(rep.Cache); err != nil {
			return rep, err
		}
	}
	for _, k := range rep.Keys {
		if _, err := b.run(nil, "config", "--local", "--unset-all", k); err != nil {
			return rep, err
		}
	}
	for batch := range slices.Chunk(theirs, 200) { // ponytail: 200 refs a push keeps the command line short
		if _, err := b.run(nil, append([]string{"push", "--quiet", remote, "--delete"}, batch...)...); err != nil {
			return rep, err
		}
	}
	return rep, nil
}
