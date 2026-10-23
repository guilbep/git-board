package gitbugio

import (
	"context"
	"errors"
	"regexp"
	"sort"

	"github.com/go-git/go-git/v5/plumbing/transport"

	bugent "github.com/git-bug/git-bug/entities/bug"
	"github.com/git-bug/git-bug/entities/identity"
	"github.com/git-bug/git-bug/repository"
)

// Remote is a git remote, and what git bug push and pull would move: the
// issues and people that differ from the remote's, as of the last fetch
// or push.
type Remote struct {
	Name string `json:"name"`
	URL  string `json:"url"` // without credentials (NoCredentials)
	// PushURLs are where a push goes, every one: its pushurls, else its
	// urls, as git's does
	PushURLs []string `json:"push_urls"`
	// Tracked: the remote's refs are known, fetched or pushed once
	Tracked bool  `json:"tracked"`
	Issues  Ahead `json:"issues"`
	People  Ahead `json:"people"`
}

// Ahead is how many entities a push would send and a pull would receive.
// Changed on both sides counts both ways: a pull merges, a push sends the
// merge.
type Ahead struct {
	Send    int `json:"send"`
	Receive int `json:"receive"`
}

// Remotes lists the repository's git remotes, by name.
func (b *Backend) Remotes() ([]Remote, error) {
	urls, err := b.repo.GetRemotes()
	if err != nil {
		return nil, err
	}
	cfg, err := b.c.Config().Read()
	if err != nil {
		return nil, err
	}
	res := []Remote{}
	for name, u := range urls {
		r := Remote{Name: name, URL: NoCredentials(u), PushURLs: []string{}}
		for _, key := range []string{"remote." + name + ".pushurl", "remote." + name + ".url"} {
			for _, e := range cfg.GetAll(key) {
				r.PushURLs = append(r.PushURLs, NoCredentials(e.Value))
			}
			if len(r.PushURLs) > 0 {
				break
			}
		}
		var issues, people bool
		if r.Issues, issues, err = b.ahead(name, bugent.Namespace); err != nil {
			return nil, err
		}
		if r.People, people, err = b.ahead(name, identity.Namespace); err != nil {
			return nil, err
		}
		r.Tracked = issues || people
		res = append(res, r)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Name < res[j].Name })
	return res, nil
}

// ahead compares the local refs of a namespace with the remote's tracking
// ones; tracked tells there are some.
func (b *Backend) ahead(remote, namespace string) (a Ahead, tracked bool, err error) {
	local, err := b.repo.ListRefs(namespace)
	if err != nil {
		return a, false, err
	}
	theirs, err := b.repo.ListTrackingRefs(remote, namespace)
	if err != nil {
		return a, false, err
	}
	for key, mine := range local {
		their, ok := theirs[key]
		switch {
		case !ok:
			a.Send++
		case their == mine:
		default:
			// one side's history holds the other's: only it moves
			ahead, behind, err := b.compare(mine, their)
			if err != nil {
				return a, true, err
			}
			if !behind {
				a.Send++
			}
			if !ahead {
				a.Receive++
			}
		}
	}
	for key := range theirs {
		if _, ok := local[key]; !ok {
			a.Receive++
		}
	}
	return a, len(theirs) > 0, nil
}

// compare tells whether mine's history holds theirs (ahead) or theirs'
// holds mine (behind), walking both in step, every parent: it stops as soon
// as one reaches the other's tip, and walks whole histories only for a
// pair that diverged.
func (b *Backend) compare(mine, theirs repository.Hash) (ahead, behind bool, err error) {
	type walk struct {
		todo []repository.Hash
		seen map[repository.Hash]bool
		goal repository.Hash
	}
	walks := []*walk{{todo: []repository.Hash{mine}, seen: map[repository.Hash]bool{}, goal: theirs}, {todo: []repository.Hash{theirs}, seen: map[repository.Hash]bool{}, goal: mine}}
	for len(walks[0].todo)+len(walks[1].todo) > 0 {
		for i, w := range walks {
			if len(w.todo) == 0 {
				continue
			}
			h := w.todo[0]
			w.todo = w.todo[1:]
			if h == w.goal {
				return i == 0, i == 1, nil
			}
			if w.seen[h] {
				continue
			}
			w.seen[h] = true
			c, err := b.repo.ReadCommit(h)
			if err != nil {
				return false, false, err
			}
			w.todo = append(w.todo, c.Parents...)
		}
	}
	return false, false, nil
}

// Fetch brings a remote's issues and people in as its tracking refs, as
// git bug pull does first: nothing local changes. An empty remote is
// nothing to fetch.
func (b *Backend) Fetch(remote string) error {
	_, err := b.c.Fetch(context.Background(), remote)
	if errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return nil
	}
	return err
}

// githubURL is a remote on github.com: https://github.com/o/r(.git),
// git@github.com:o/r.git, ssh://git@github.com/o/r(.git).
var githubURL = regexp.MustCompile(`^(?:https://|ssh://(?:git@)?|git@)github\.com[:/]([\w.-]+/[\w.-]+?)(?:\.git)?/?$`)

// GitHubRepo is "owner/name" of the remote when it is on github.com, ""
// otherwise.
func (b *Backend) GitHubRepo(remote string) string {
	urls, err := b.repo.GetRemotes()
	if err != nil {
		return ""
	}
	return GitHubRepoOf(urls[remote])
}

// GitHubRepoOf is "owner/name" of a remote URL on github.com, "" otherwise.
func GitHubRepoOf(url string) string {
	if m := githubURL.FindStringSubmatch(NoCredentials(url)); m != nil {
		return m[1]
	}
	return ""
}

// credentials are a URL's user and password: https://oauth2:<token>@host
var credentials = regexp.MustCompile(`://[^/\s@]+@`)

// NoCredentials is a remote's URL, or a text quoting one (git's errors
// do), without the user and password it may carry. An scp-like URL
// (git@host:path) carries none.
func NoCredentials(s string) string {
	return credentials.ReplaceAllString(s, "://")
}
