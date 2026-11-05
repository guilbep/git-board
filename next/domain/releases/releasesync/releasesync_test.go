package releasesync

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/guilbep/git-board/next/domain/releases"
	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/forge/fake"
	bsync "github.com/guilbep/git-board/next/sync"
)

// flat is a forge without releases.
type flat struct{ forge.Forge }

func tag(name, commit string) releases.Tag {
	v, _ := releases.ParseTag(name)
	return releases.Tag{Name: name, Commit: commit, Version: v}
}

// A push publishes each recorded tag, once: a second run writes nothing,
// a notes edit or a yank is an edit, a tag without a record waits, and a
// forge without releases is never asked. A pull writes nothing.
func TestPublishesTheRecords(t *testing.T) {
	t.Parallel()
	f := fake.New("example.com/p")
	recs := []releases.Record{{ID: "r1", Commit: "c1", Notes: "First.\n"}, {ID: "r2", Commit: "c2", Notes: "Second."}}
	tags := []releases.Tag{tag("v0.3.0", "c3"), tag("v0.2.0", "c2"), tag("v0.2.0-rc.1", "c2"), tag("v0.1.0", "c1")}
	var heard []string
	m := &Mapper{
		Records: func(context.Context) ([]releases.Record, error) { return recs, nil },
		Tags:    func(context.Context) ([]releases.Tag, error) { return tags, nil },
		Did:     func(key, what, tag string) { heard = append(heard, what+" "+tag) },
	}
	run := func(way bsync.Way) {
		t.Helper()
		if err := bsync.Run(t.Context(), nil, way, []forge.Forge{flat{f}, f}, m); err != nil {
			t.Fatal(err)
		}
	}
	run(bsync.Pull)
	if len(heard) != 0 {
		t.Fatalf("a pull wrote %v", heard)
	}
	run(bsync.Push)
	if want := []string{"created v0.1.0", "created v0.2.0-rc.1", "created v0.2.0"}; !slices.Equal(heard, want) {
		t.Fatalf("heard %v, want %v", heard, want)
	}
	rs, _ := f.Releases(t.Context())
	if len(rs) != 3 || rs[0].Tag != "v0.2.0" || rs[0].Body != "Second." || rs[0].Prerelease || !rs[1].Prerelease || rs[2].Body != "First." {
		t.Errorf("releases = %+v", rs)
	}
	heard = nil
	run(bsync.Both)
	if len(heard) != 0 {
		t.Fatalf("a second run wrote %v", heard)
	}
	recs[0].Yanked = "breaks the export"
	run(bsync.Push)
	if want := []string{"edited v0.1.0"}; !slices.Equal(heard, want) {
		t.Fatalf("heard %v, want %v", heard, want)
	}
	rs, _ = f.Releases(t.Context())
	if rs[2].Body != "**Yanked:** breaks the export\n\nFirst." {
		t.Errorf("yanked body %q", rs[2].Body)
	}
}

// A dry run hears what it would write, and writes nothing.
func TestDry(t *testing.T) {
	t.Parallel()
	f := fake.New("example.com/p")
	var heard []string
	m := &Mapper{Dry: true,
		Records: func(context.Context) ([]releases.Record, error) { return []releases.Record{{Commit: "c1"}}, nil },
		Tags:    func(context.Context) ([]releases.Tag, error) { return []releases.Tag{tag("v1.0.0", "c1")}, nil },
		Did:     func(key, what, tag string) { heard = append(heard, what) },
	}
	if err := m.Run(t.Context(), nil, bsync.Push, []forge.Forge{f}); err != nil {
		t.Fatal(err)
	}
	if rs, _ := f.Releases(t.Context()); len(rs) != 0 || !slices.Equal(heard, []string{"created"}) || m.Report.Created != 1 {
		t.Errorf("releases %+v, heard %v, report %+v", rs, heard, m.Report)
	}
}

// Publish makes the release and attaches the files it lacks; again, it
// attaches none twice.
func TestPublishAttaches(t *testing.T) {
	t.Parallel()
	f := fake.New("example.com/p")
	dir := t.TempDir()
	var files []string
	for _, n := range []string{"app_linux_amd64.tar.gz", "checksums.txt"} {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	want := forge.Release{Tag: "v1.0.0", Name: "v1.0.0", Body: "notes"}
	for range 2 {
		if err := Publish(t.Context(), f, want, files); err != nil {
			t.Fatal(err)
		}
	}
	rs, _ := f.Releases(t.Context())
	if len(rs) != 1 || !slices.Equal(rs[0].Assets, []string{"app_linux_amd64.tar.gz", "checksums.txt"}) {
		t.Errorf("releases %+v", rs)
	}
	if b, _ := f.File("v1.0.0", "checksums.txt"); string(b) != "checksums.txt" {
		t.Errorf("file %q", b)
	}
}
