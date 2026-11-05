package app

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// A forge's pictures are GitHub's attachments and their S3, and GitLab's
// uploads on its hosts: nothing else goes through the server.
func TestForgeMedia(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]bool{
		"https://github.com/user-attachments/assets/1234":                      true,
		"https://github.com/o/r/assets/5678/abc":                               true,
		"https://private-user-images.githubusercontent.com/1/x.png":            true,
		"https://github-production-user-asset-6210df.s3.amazonaws.com/1/x.png": true,
		"https://gitlab.com/o/r/uploads/abc/x.png":                             true,
		"https://gitlab.epfl.ch/o/r/uploads/abc/x.png":                         true,
		"https://gitlab.example.org/o/r/uploads/abc/x.png":                     false,
		"http://github.com/user-attachments/assets/1234":                       false,
		"https://github.com/o/r/issues/1":                                      false,
		"https://example.com/a.png":                                            false,
		"https://s3.amazonaws.com/github-production-user-asset-6210df/1/x.png": false,
		// a bucket anyone can name so (review finding 5)
		"https://github-production-user-asset-attacker123.s3.amazonaws.com/x.png":         false,
		"https://github-production-user-asset-6210df.s3.amazonaws.com.evil.example/x.png": false,
		"https://githubusercontent.com.evil.example/x.png":                                false,
	} {
		u, _ := url.Parse(raw)
		if got := ForgeMedia(u, []string{"gitlab.epfl.ch"}); got != want {
			t.Errorf("ForgeMedia(%s) = %v, want %v", raw, got, want)
		}
	}
}

// The kept pictures are bounded (review finding 5): those unused for
// mediaMaxAge go, then the least recently used, until the folder holds
// at most max bytes; one read is a use.
func TestTrimMedia(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	now := time.Now()
	write := func(name string, size int, used time.Duration) string {
		f := filepath.Join(dir, name)
		if err := os.WriteFile(f, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(f, now.Add(-used), now.Add(-used)); err != nil {
			t.Fatal(err)
		}
		return f
	}
	write("old", 10, mediaMaxAge+time.Hour)
	for i := range 5 {
		write(fmt.Sprintf("f%d", i), 100, time.Duration(5-i)*time.Minute) // f4 the most recent
	}
	// f0 read now: the most recently used
	if _, _, err := cachedMedia(filepath.Join(dir, "f0")); err == nil {
		t.Fatal("a kept picture without its type read")
	}
	touchMedia(filepath.Join(dir, "f0"), now)
	trimMedia(dir, 300, now)
	var left []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if want := []string{"f0", "f3", "f4"}; !slices.Equal(left, want) {
		t.Fatalf("kept %v, want %v", left, want)
	}
}
