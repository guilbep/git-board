package releasetest

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/guilbep/git-board/internal/release"
)

// FakeImage is an image the fake registry holds.
type FakeImage struct{ Digest, Revision string }

// Tools is an in-memory release.Tools. Images is keyed by ref, and every
// registered image also answers under <repo>@<digest>. Copied and Scanned
// hold "src → dst" / ref; Tagged holds "ref → tag".
type Tools struct {
	mu            sync.Mutex
	Images        map[string]FakeImage
	Built         []release.Build
	Scanned       []string
	Copied        []string
	Tagged        []string
	FailScan      map[string]bool
	Charts        map[string]string // registry + " " + version → digest
	Published     []Published       // the forge releases made
	CopyPublished string            // a directory Publish copies its files into, for tests to open: the checkout is gone once the cut returns
}

// Published is a forge release Publish made.
type Published struct {
	Repo, Tag, Notes string
	Files            []string
}

func NewTools() *Tools {
	return &Tools{Images: map[string]FakeImage{}, FailScan: map[string]bool{}, Charts: map[string]string{}}
}

var _ release.Tools = (*Tools)(nil)

// repo is ref without its :tag or @digest.
func repo(ref string) string {
	if i := strings.Index(ref, "@"); i >= 0 {
		return ref[:i]
	}
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") && i >= 0 {
		return ref[:i]
	}
	return ref
}

// put registers img under ref and under <repo>@<digest>; callers hold mu.
func (f *Tools) put(ref string, img FakeImage) {
	f.Images[ref] = img
	f.Images[repo(ref)+"@"+img.Digest] = img
}

func (f *Tools) Revision(_ context.Context, ref string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	img, ok := f.Images[ref]
	if !ok {
		return "", "", fmt.Errorf("%s: %w", ref, release.ErrNoImage)
	}
	return img.Digest, img.Revision, nil
}

func (f *Tools) Build(_ context.Context, b release.Build) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Built = append(f.Built, b)
	d := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(b.Ref+b.Revision)))[:7+12]
	f.put(b.Ref, FakeImage{Digest: d, Revision: b.Revision})
	return d, nil
}

func (f *Tools) Scan(_ context.Context, ref string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Scanned = append(f.Scanned, ref)
	if f.FailScan[ref] {
		return fmt.Errorf("scan %s: vulnerabilities found", ref)
	}
	return nil
}

func (f *Tools) Copy(_ context.Context, src, dst string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	img, ok := f.Images[src]
	if !ok {
		return fmt.Errorf("%s: %w", src, release.ErrNoImage)
	}
	f.Copied = append(f.Copied, src+" → "+dst)
	f.put(dst, img)
	return nil
}

func (f *Tools) Tag(_ context.Context, ref, tag string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	img, ok := f.Images[ref]
	if !ok {
		return fmt.Errorf("%s: %w", ref, release.ErrNoImage)
	}
	f.Tagged = append(f.Tagged, ref+" → "+tag)
	f.put(repo(ref)+":"+tag, img)
	return nil
}

func (f *Tools) PackageChart(_ context.Context, _, version, dir string) (string, error) {
	return dir + "/chart-" + version + ".tgz", nil
}

func (f *Tools) PushChart(_ context.Context, file, registry string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	version := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(file), "chart-"), ".tgz")
	d := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(registry+" "+version)))[:7+12]
	f.Charts[registry+" "+version] = d
	return d, nil
}

func (f *Tools) Publish(_ context.Context, repo, tag, notes string, files []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Published = append(f.Published, Published{Repo: repo, Tag: tag, Notes: notes, Files: files})
	if f.CopyPublished == "" {
		return nil
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(f.CopyPublished, filepath.Base(file)), raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}
