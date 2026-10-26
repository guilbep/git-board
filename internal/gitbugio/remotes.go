package gitbugio

import (
	"regexp"
	"sort"
	"strings"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/oplog"
)

// Remote is a git remote, and what a push and a pull would move: the
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

// remoteURLs are each remote's URLs and push URLs, by its name, in the
// config's order and without credentials.
func (b *Backend) remoteURLs() (urls, pushURLs map[string][]string, err error) {
	out, err := b.git("config", "--get-regexp", `^remote\..*\.(url|pushurl)$`)
	if err != nil && len(out) > 0 {
		return nil, nil, err
	}
	// remote.<name>.url <url>: a name may hold dots, the key's last part never
	urls, pushURLs = map[string][]string{}, map[string][]string{}
	for _, l := range lines(out) {
		key, value, _ := strings.Cut(l, " ")
		key = strings.TrimPrefix(key, "remote.")
		dot := strings.LastIndexByte(key, '.')
		if dot < 0 {
			continue
		}
		if name := key[:dot]; key[dot+1:] == "url" {
			urls[name] = append(urls[name], NoCredentials(value))
		} else {
			pushURLs[name] = append(pushURLs[name], NoCredentials(value))
		}
	}
	return urls, pushURLs, nil
}

// Remotes lists the repository's git remotes, by name.
func (b *Backend) Remotes() ([]Remote, error) {
	urls, pushURLs, err := b.remoteURLs()
	if err != nil {
		return nil, err
	}
	res := []Remote{}
	for name, u := range urls {
		r := Remote{Name: name, URL: u[0], PushURLs: pushURLs[name]}
		if len(r.PushURLs) == 0 {
			r.PushURLs = u
		}
		var issues, people bool
		if r.Issues, issues, err = b.ahead(name, "issue"); err != nil {
			return nil, err
		}
		if r.People, people, err = b.ahead(name, "person"); err != nil {
			return nil, err
		}
		r.Tracked = issues || people
		res = append(res, r)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Name < res[j].Name })
	return res, nil
}

// ahead compares the local refs of a kind with the remote's copies here;
// tracked tells there are some.
func (b *Backend) ahead(remote, kind string) (a Ahead, tracked bool, err error) {
	localPrefix, theirPrefix := oplog.RefName(kind, ""), "refs/board-remotes/"+remote+"/"+kind+"/"
	local, err := b.s.Refs(localPrefix)
	if err != nil {
		return a, false, err
	}
	theirs, err := b.s.Refs(theirPrefix)
	if err != nil {
		return a, false, err
	}
	for name, mine := range local {
		their, ok := theirs[theirPrefix+strings.TrimPrefix(name, localPrefix)]
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
	for name := range theirs {
		if _, ok := local[localPrefix+strings.TrimPrefix(name, theirPrefix)]; !ok {
			a.Receive++
		}
	}
	return a, len(theirs) > 0, nil
}

// compare tells whether mine's history holds theirs (ahead) or theirs'
// holds mine (behind), walking both in step, every parent: it stops as soon
// as one reaches the other's tip, and walks whole histories only for a
// pair that diverged.
func (b *Backend) compare(mine, theirs gitstore.Hash) (ahead, behind bool, err error) {
	type walk struct {
		todo []gitstore.Hash
		seen map[gitstore.Hash]bool
		goal gitstore.Hash
	}
	walks := []*walk{{todo: []gitstore.Hash{mine}, seen: map[gitstore.Hash]bool{}, goal: theirs}, {todo: []gitstore.Hash{theirs}, seen: map[gitstore.Hash]bool{}, goal: mine}}
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
			c, err := b.s.ReadCommit(h)
			if err != nil {
				return false, false, err
			}
			w.todo = append(w.todo, c.Parents...)
		}
	}
	return false, false, nil
}

// githubURL is a remote on github.com: https://github.com/o/r(.git),
// git@github.com:o/r.git, ssh://git@github.com/o/r(.git).
var githubURL = regexp.MustCompile(`^(?:https://|ssh://(?:git@)?|git@)github\.com[:/]([\w.-]+/[\w.-]+?)(?:\.git)?/?$`)

// GitHubRepo is "owner/name" of the remote when it is on github.com, ""
// otherwise.
func (b *Backend) GitHubRepo(remote string) string {
	if b.checkRemote(remote) != nil {
		return ""
	}
	out, err := b.git("remote", "get-url", remote)
	if err != nil {
		return ""
	}
	return GitHubRepoOf(strings.TrimSpace(string(out)))
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
