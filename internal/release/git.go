package release

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // a cache directory name, not security
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Git runs git in dir and returns trimmed stdout; the error carries stderr.
func Git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// IsAncestor reports whether a is an ancestor of b (a == b counts).
func IsAncestor(ctx context.Context, dir, a, b string) (bool, error) {
	err := exec.CommandContext(ctx, "git", "-C", dir, "merge-base", "--is-ancestor", a, b).Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &ee) && ee.ExitCode() == 1:
		return false, nil
	}
	return false, fmt.Errorf("git merge-base --is-ancestor %s %s: %w", a, b, err)
}

// Tag is a release tag.
type Tag struct {
	Name    string    `json:"name"`
	Commit  string    `json:"commit"`
	Message string    `json:"message"`
	Version Version   `json:"version"`
	Date    time.Time `json:"date"`
}

// ReleaseTags lists the tags ParseTag accepts, newest version first.
func ReleaseTags(ctx context.Context, dir string) ([]Tag, error) {
	out, err := Git(ctx, dir, "for-each-ref", "--format=%(refname:short)%09%(objectname)%09%(*objectname)%09%(creatordate:iso-strict)%09%(contents:subject)", "refs/tags")
	if err != nil {
		return nil, err
	}
	var tags []Tag
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, "\t", 5)
		if len(f) < 5 {
			continue
		}
		v, ok := ParseTag(f[0])
		if !ok {
			continue
		}
		commit := f[1]
		if f[2] != "" { // annotated: the commit it points at
			commit = f[2]
		}
		date, _ := time.Parse(time.RFC3339, f[3])
		tags = append(tags, Tag{Name: f[0], Commit: commit, Message: f[4], Version: v, Date: date})
	}
	sort.Slice(tags, func(i, j int) bool { return tags[j].Version.Less(tags[i].Version) })
	return tags, nil
}

// TagsAt returns the tags pointing at commit.
func TagsAt(tags []Tag, commit string) []Tag {
	var at []Tag
	for _, t := range tags {
		if t.Commit == commit {
			at = append(at, t)
		}
	}
	return at
}

// PushTag creates the annotated tag locally and pushes it. A remote that
// already has the name refuses: the local tag is dropped and taken is true.
// Nothing is ever moved or deleted on the remote.
func PushTag(ctx context.Context, dir, remote string, t Tag) (taken bool, err error) {
	if _, err := Git(ctx, dir, "tag", "-a", t.Name, "-m", t.Message, t.Commit); err != nil {
		return false, err
	}
	_, err = Git(ctx, dir, "push", remote, "refs/tags/"+t.Name)
	if err == nil {
		return false, nil
	}
	if strings.Contains(err.Error(), "[rejected]") || strings.Contains(err.Error(), "already exists") {
		_, derr := Git(ctx, dir, "tag", "-d", t.Name)
		return true, derr
	}
	return false, err
}

// Change is one commit that touched an overlay.
type Change struct {
	Commit  string    `json:"commit"`
	Author  string    `json:"author"`
	Version string    `json:"version"`
	Date    time.Time `json:"date"`
}

// ConfigRepo is a local clone of a GitOps repository.
type ConfigRepo struct{ URL, Dir string }

// OpenConfigRepo clones url into <cacheDir>/<first 12 hex of sha1(url)>, or reuses that clone.
func OpenConfigRepo(ctx context.Context, cacheDir, url string) (*ConfigRepo, error) {
	sum := sha1.Sum([]byte(url)) //nolint:gosec
	dir := filepath.Join(cacheDir, hex.EncodeToString(sum[:])[:12])
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return nil, err
		}
		if _, err := Git(ctx, cacheDir, "clone", url, dir); err != nil {
			return nil, err
		}
	}
	return &ConfigRepo{URL: url, Dir: dir}, nil
}

// sync fetches and resets the clone to the remote default branch, which it returns.
func (c *ConfigRepo) sync(ctx context.Context) (string, error) {
	if _, err := Git(ctx, c.Dir, "fetch", "origin"); err != nil {
		return "", err
	}
	if _, err := Git(ctx, c.Dir, "remote", "set-head", "origin", "-a"); err != nil {
		return "", err
	}
	ref, err := Git(ctx, c.Dir, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", err
	}
	branch := strings.TrimPrefix(ref, "refs/remotes/origin/")
	_, err = Git(ctx, c.Dir, "reset", "--hard", "origin/"+branch)
	return branch, err
}

func overlayFile(path string) string {
	return filepath.ToSlash(filepath.Join(path, "kustomization.yaml"))
}

// Read returns the overlay in directory path at the remote default branch.
func (c *ConfigRepo) Read(ctx context.Context, path string) (*Overlay, error) {
	if _, err := c.sync(ctx); err != nil {
		return nil, err
	}
	return c.load(path)
}

func (c *ConfigRepo) load(path string) (*Overlay, error) {
	data, err := os.ReadFile(filepath.Join(c.Dir, overlayFile(path)))
	if err != nil {
		return nil, err
	}
	return ParseOverlay(data)
}

// Update applies change to the overlay in directory path and pushes one
// commit to the default branch. change returns false when there is nothing to
// write (Update returns ""). A refused push starts over, up to 3 tries.
func (c *ConfigRepo) Update(ctx context.Context, path, message string, change func(*Overlay) (bool, error)) (commit string, err error) {
	for range 3 {
		branch, err := c.sync(ctx)
		if err != nil {
			return "", err
		}
		o, err := c.load(path)
		if err != nil {
			return "", err
		}
		changed, err := change(o)
		if err != nil || !changed {
			return "", err
		}
		data, err := o.Bytes()
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(c.Dir, overlayFile(path)), data, 0o644); err != nil {
			return "", err
		}
		if _, err := Git(ctx, c.Dir, "add", overlayFile(path)); err != nil {
			return "", err
		}
		if _, err := Git(ctx, c.Dir, "commit", "-m", message); err != nil {
			return "", err
		}
		_, perr := Git(ctx, c.Dir, "push", "origin", "HEAD:refs/heads/"+branch)
		if perr == nil {
			return Git(ctx, c.Dir, "rev-parse", "HEAD")
		}
		if !strings.Contains(perr.Error(), "[rejected]") && !strings.Contains(perr.Error(), "non-fast-forward") {
			return "", perr
		}
	}
	return "", fmt.Errorf("config repo %s moved 3 times while writing: try again", c.URL)
}

// History lists the last n commits touching the overlay in directory path, newest first.
func (c *ConfigRepo) History(ctx context.Context, path string, n int) ([]Change, error) {
	if _, err := c.sync(ctx); err != nil {
		return nil, err
	}
	out, err := Git(ctx, c.Dir, "log", fmt.Sprintf("-n%d", n), "--format=%H%x09%an%x09%aI%x09%s", "--", overlayFile(path))
	if err != nil || out == "" {
		return nil, err
	}
	var cs []Change
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, "\t", 4)
		if len(f) < 4 {
			continue
		}
		date, _ := time.Parse(time.RFC3339, f[2])
		cs = append(cs, Change{Commit: f[0], Author: f[1], Version: versionOfSubject(f[3]), Date: date})
	}
	return cs, nil
}

// versionOfSubject extracts <version> from "chore(manifest): update <app> (<env>) <version> [by …]".
func versionOfSubject(s string) string {
	if !strings.HasPrefix(s, "chore(manifest): update ") {
		return ""
	}
	head, _, ok := strings.Cut(s, " [by ")
	if !ok {
		return ""
	}
	f := strings.Fields(head)
	return f[len(f)-1]
}
