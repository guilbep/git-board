package app

import (
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

	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/forge/github"
	"github.com/guilbep/git-board/next/forge/gitlab"
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
func (b *Board) gitlabForges(ctx context.Context) []forge.Table {
	tables := b.Config.Forges.List
	if len(tables) == 0 || b.Config.Forges.Discover {
		out, _ := gitRun(ctx, b.Dir, "remote", "-v")
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
	return out
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

// Media is a forge's picture, fetched with the forge's token, kept on
// disk (a git board's folder) and answered from there next time.
func (b *Board) Media(ctx context.Context, raw string) (contentType string, data []byte, err error) {
	u, err := url.Parse(raw)
	gitlabs := b.gitlabForges(ctx)
	var hosts []string
	for _, t := range gitlabs {
		hosts = append(hosts, t.Host)
	}
	if err != nil || !ForgeMedia(u, hosts) {
		return "", nil, &Error{Class: Usage, Err: fmt.Errorf("not a forge's picture: %q", raw)}
	}
	var file string
	if b.inGit() {
		sum := sha256.Sum256([]byte(raw))
		file = filepath.Join(b.Kernel.Store().Folder(), "v2", "media", hex.EncodeToString(sum[:]))
		if contentType, data, err := cachedMedia(file); err == nil {
			touchMedia(file, time.Now())
			return contentType, data, nil
		}
	}
	token := ""
	switch host := u.Hostname(); {
	case host == "github.com" || strings.HasSuffix(host, ".githubusercontent.com"):
		token, _ = github.Token(ctx, "github.com", "")
	default:
		for _, t := range append(gitlabs, forge.Table{Host: "gitlab.com"}) {
			if t.Host == host {
				token, _ = gitlab.Token(ctx, "https://"+host, t.Login)
				break
			}
		}
	}
	contentType, data, err = fetchMedia(ctx, u, token, hosts)
	if err != nil {
		return "", nil, err
	}
	if file != "" { // a failure costs a fetch next time
		if os.MkdirAll(filepath.Dir(file), 0o755) == nil {
			_ = os.WriteFile(file, append([]byte(contentType+"\n"), data...), 0o644)
			trimMedia(filepath.Dir(file), mediaKeep, time.Now())
		}
	}
	return contentType, data, nil
}

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
		return "", nil, fmt.Errorf("the forge answered %s", resp.Status)
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
