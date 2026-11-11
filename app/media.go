package app

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/forge"
)

// The pictures of issue text that a forge keeps (v1's media proxy): a
// private repository's need the forge's token, which the page hasn't. The
// server fetches them, with the token, from a forge's image addresses
// alone (so it reaches nothing else from this machine), and keeps them
// on disk beside the board.

// maxMedia caps a picture the server fetches.
const maxMedia = 20 << 20

// The kept pictures are bounded: a picture unused for mediaMaxAge goes,
// then the least recently used, until they weigh mediaKeep at most. Issue
// text anyone writes on a public repository names them.
const (
	mediaKeep   = 256 << 20
	mediaMaxAge = 30 * 24 * time.Hour
)

// githubAssetBucket is the S3 bucket GitHub's attachments
// (github.com/user-attachments/assets/…) redirect to: that one bucket by
// its exact name, never a prefix, since anyone can name an S3 bucket.
const githubAssetBucket = "github-production-user-asset-6210df.s3.amazonaws.com"

// ForgeMedia tells whether u is where a forge keeps the pictures of
// issues: GitHub's attachments and their S3, GitLab's uploads on
// gitlab.com or one of gitlabHosts. *.githubusercontent.com is GitHub's
// own domain (private-user-images, user-images, raw…); its S3 is one
// bucket, githubAssetBucket.
func ForgeMedia(u *url.URL, gitlabHosts []string) bool {
	if u.Scheme != "https" {
		return false
	}
	host, p := u.Hostname(), u.Path
	switch {
	case host == "github.com":
		return strings.HasPrefix(p, "/user-attachments/") || strings.Contains(p, "/assets/")
	case strings.HasSuffix(host, ".githubusercontent.com"):
		return true
	case host == githubAssetBucket:
		return true
	case host == "gitlab.com" || slices.Contains(gitlabHosts, host):
		return strings.Contains(p, "/uploads/")
	}
	return false
}

// gitlabForges are the board's GitLab forges, host and login: its
// [forges] tables, else those its remotes name (no network: the hosts
// alone).
// They are kept while git's config files are unchanged (forgeCache): a
// picture asked ran `git remote -v` each time.
func (b *Board) gitlabForges(ctx context.Context) []forge.Table {
	c := &b.forges
	out, _ := c.gitlab.get(c.stamp(ctx, b.Dir), func() ([]forge.Table, error) {
		tables := b.Config.Forges.List
		var err error // a read that failed (a request gone) keeps nothing
		if len(tables) == 0 || b.Config.Forges.Discover {
			var out string
			out, err = gitRun(ctx, b.Dir, "remote", "-v")
			for _, l := range strings.Split(out, "\n") {
				if f := strings.Fields(l); len(f) >= 2 {
					if t, _ := forge.FromRemote(NoCredentials(f[1])); t.Kind != "" {
						tables = append(tables, t)
					}
				}
			}
		}
		var out []forge.Table
		for _, t := range tables {
			if t.Kind == "gitlab" && t.Host != "" {
				out = append(out, t)
			}
		}
		return out, err
	})
	return slices.Clone(out)
}

// MediaURL is where a page loads a picture of issue text from: the
// server, for a forge's, else its own address. The GitLab hosts are read
// once, the first time a picture could be on one.
func (b *Board) MediaURL(ctx context.Context) func(raw string) string {
	hosts := sync.OnceValue(func() []string {
		var hs []string
		for _, t := range b.gitlabForges(ctx) {
			hs = append(hs, t.Host)
		}
		return hs
	})
	return func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" {
			return raw
		}
		var gl []string
		if strings.Contains(u.Path, "/uploads/") {
			gl = hosts()
		}
		if !ForgeMedia(u, gl) {
			return raw
		}
		return "/api/media?url=" + url.QueryEscape(raw)
	}
}

// Picture is a forge's picture as the server answers it: its type, its
// bytes, their ETag, and whether its address names these bytes for good.
type Picture struct {
	Type      string
	Data      []byte
	ETag      string // the bytes' sum, quoted: If-None-Match answers 304
	Immutable bool   // immutableMedia: a browser keeps it a year
}

// mediaFresh is how long a kept picture whose address may show other
// bytes later (an avatar, a file in a repository) is answered from disk.
const mediaFresh = 24 * time.Hour

// Media is a forge's picture, fetched with the forge's token, kept on
// disk (the board's own folder) and answered from there next time: for
// good when immutableMedia, for mediaFresh otherwise, then fetched again;
// a fetch that fails then answers the kept one.
func (b *Board) Media(ctx context.Context, raw string) (Picture, error) {
	u, err := url.Parse(raw)
	gitlabs := b.gitlabForges(ctx)
	var hosts []string
	for _, t := range gitlabs {
		hosts = append(hosts, t.Host)
	}
	if err != nil || !ForgeMedia(u, hosts) {
		return Picture{}, &Error{Class: Usage, Err: fmt.Errorf("not a forge's picture: %q", raw)}
	}
	immutable := immutableMedia(u, hosts)
	var file string
	if folder := b.Kernel.Store().Folder(); folder != "" { // this machine's, never a checkout's
		sum := sha256.Sum256([]byte(raw))
		file = filepath.Join(folder, "v2", "media", hex.EncodeToString(sum[:]))
	}
	var kept *Picture
	if file != "" {
		if contentType, data, err := cachedMedia(file); err == nil {
			p := picture(contentType, data, immutable)
			if immutable {
				touchMedia(file, time.Now())
				return p, nil
			}
			// a mutable one's time is when it was fetched: never touched
			if st, err := os.Stat(file); err == nil && time.Since(st.ModTime()) < mediaFresh {
				return p, nil
			}
			kept = &p
		}
	}
	token, key := b.mediaToken(ctx, u.Hostname(), gitlabs)
	contentType, data, err := fetchMedia(ctx, u, token, hosts)
	if fa := (forgeAnswered{}); key != "" && errors.As(err, &fa) && (fa.code == http.StatusUnauthorized || fa.code == http.StatusForbidden) {
		b.forges.dropMediaToken(key) // revoked, or another account's: looked up again next time
	}
	if err != nil {
		if kept != nil {
			return *kept, nil // the forge out of reach: the picture as it was
		}
		return Picture{}, err
	}
	if file != "" { // a failure costs a fetch next time
		if os.MkdirAll(filepath.Dir(file), 0o755) == nil {
			_ = os.WriteFile(file, append([]byte(contentType+"\n"), data...), 0o644)
			trimMedia(filepath.Dir(file), mediaKeep, time.Now())
		}
	}
	return picture(contentType, data, immutable), nil
}

func picture(contentType string, data []byte, immutable bool) Picture {
	sum := sha256.Sum256(data)
	return Picture{Type: contentType, Data: data, ETag: `"` + hex.EncodeToString(sum[:12]) + `"`, Immutable: immutable}
}

// immutableMedia says u's address names one picture for good: GitHub's
// attachments (github.com/user-attachments, private-user-images and
// user-images.githubusercontent.com) and GitLab's uploads, by their hash.
// Any other (raw.githubusercontent.com, an avatar) may show other bytes
// later.
func immutableMedia(u *url.URL, gitlabHosts []string) bool {
	switch host := u.Hostname(); {
	case host == "github.com":
		return strings.HasPrefix(u.Path, "/user-attachments/")
	case host == "private-user-images.githubusercontent.com", host == "user-images.githubusercontent.com":
		return true
	case host == "gitlab.com" || slices.Contains(gitlabHosts, host):
		return strings.Contains(u.Path, "/uploads/")
	}
	return false
}

// mediaToken is the token host's pictures are fetched with, and the
// forge it is kept for (key, "" for none): GitHub's for github.com and
// its githubusercontent.com hosts, one token for all of them; a GitLab
// forge's for its host; none for any other (S3's bucket: the token never
// goes there). Yours on that host, else the forge's posting account's,
// else any (LoginFor's order). Looked up once and kept
// (forgeCache.mediaToken), whatever becomes of the request that asked.
func (b *Board) mediaToken(ctx context.Context, host string, gitlabs []forge.Table) (token, key string) {
	var f forge.Table
	switch {
	case host == "github.com" || strings.HasSuffix(host, ".githubusercontent.com"):
		f = forge.Table{Kind: "github", Host: "github.com"}
		if found, err := b.FoundForges(ctx, func(string) {}); err == nil {
			if i := slices.IndexFunc(found, func(t forge.Table) bool { return t.Kind == "github" && t.Host == "github.com" }); i >= 0 {
				f.Login = found[i].Login
			}
		}
	default:
		for _, t := range append(gitlabs, forge.Table{Kind: "gitlab", Host: "gitlab.com"}) {
			if t.Host == host {
				f = t
				break
			}
		}
	}
	if f.Kind == "" {
		return "", ""
	}
	key = f.Kind + " " + f.Host
	ctx = context.WithoutCancel(ctx) // the others waiting on it keep its answer
	return b.forges.mediaToken(key, func() string {
		for _, login := range slices.Compact([]string{cmp.Or(b.myLogin(ctx, f.Host), f.Login), ""}) {
			if t, err := FindToken(ctx, f, login); err == nil && t != "" {
				return t
			}
		}
		return ""
	}), key
}

// mediaToken is a forge's token for its pictures (key: its kind and
// host), find's answer kept: a
// minute found, 10 s not, and until a token is stored here (tokenFound's
// rule). The lock is held across find, so thirty pictures missing at
// once run one git credential fill, not thirty. The token stays in this
// process's memory alone, never on disk nor in an answer.
// ponytail: one lock for every host; a lock a host if pictures from
// several forges load at once.
func (c *forgeCache) mediaToken(key string, find func() string) string {
	c.mediaMu.Lock()
	defer c.mediaMu.Unlock()
	stored := tokensStored.Load()
	if k, ok := c.media[key]; ok && k.stored == stored {
		if age := time.Since(k.at); k.token != "" && age < time.Minute || age < 10*time.Second {
			return k.token
		}
	}
	t := find()
	if c.media == nil {
		c.media = map[string]mediaToken{}
	}
	c.media[key] = mediaToken{token: t, at: time.Now(), stored: stored}
	return t
}

// dropMediaToken forgets a forge's token: it refused it.
func (c *forgeCache) dropMediaToken(key string) {
	c.mediaMu.Lock()
	delete(c.media, key)
	c.mediaMu.Unlock()
}

// mediaToken is a forge's token as mediaToken found it ("" for none).
type mediaToken struct {
	token  string
	at     time.Time
	stored int64 // tokensStored then
}

// forgeAnswered is a forge's answer to a picture other than 200.
type forgeAnswered struct {
	code   int
	status string
}

func (e forgeAnswered) Error() string { return "the forge answered " + e.status }

func fetchMedia(ctx context.Context, u *url.URL, token string, gitlabHosts []string) (string, []byte, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		// GitHub redirects to S3: each hop is a forge's picture too
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 5 || !ForgeMedia(req.URL, gitlabHosts) {
				return errors.New("redirect off the forge")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", nil, err
	}
	// Go drops the header on a redirect to another host (S3)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, forgeAnswered{resp.StatusCode, resp.Status}
	}
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return "", nil, &Error{Class: Usage, Err: fmt.Errorf("not a picture: %s", contentType)}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMedia+1))
	if err != nil {
		return "", nil, err
	}
	if len(data) > maxMedia {
		return "", nil, &Error{Class: Usage, Err: fmt.Errorf("a picture over %d MB", maxMedia>>20)}
	}
	return contentType, data, nil
}

// cachedMedia is a kept picture: its type on the first line, then its
// bytes.
func cachedMedia(file string) (string, []byte, error) {
	all, err := os.ReadFile(file)
	if err != nil {
		return "", nil, err
	}
	contentType, data, ok := strings.Cut(string(all), "\n")
	if !ok {
		return "", nil, errors.New("a kept picture without its type")
	}
	return contentType, []byte(data), nil
}

// touchMedia marks a kept picture used now: its modification time.
func touchMedia(file string, now time.Time) { _ = os.Chtimes(file, now, now) }

// trimMedia bounds the kept pictures of dir: those unused for
// mediaMaxAge go, then the least recently used until the rest weigh max
// bytes at most. A file it can't read or remove stays.
func trimMedia(dir string, max int64, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type kept struct {
		path string
		size int64
		used time.Time
	}
	var files []kept
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		f := kept{filepath.Join(dir, e.Name()), info.Size(), info.ModTime()}
		if now.Sub(f.used) > mediaMaxAge {
			_ = os.Remove(f.path)
			continue
		}
		files = append(files, f)
		total += f.size
	}
	slices.SortFunc(files, func(a, b kept) int { return a.used.Compare(b.used) })
	for _, f := range files {
		if total <= max {
			return
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}
