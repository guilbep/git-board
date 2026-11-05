// Package releasesync is the releases' mapper on the sync engine
// (docs/rewrite/design.md, Sync): ours to the forge, never back. Each
// release tag whose commit has a record is the forge's release of that
// tag: made when missing, its notes and its yank kept the same, the
// files a cut built attached. It reads git's tags as given, and the
// records; it writes nothing here. A forge release is found by its tag,
// so no sync record is kept: a second run finds what the first made.
package releasesync

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/guilbep/git-board/domain/releases"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/kernel"
	bsync "github.com/guilbep/git-board/sync"
)

// Mapper publishes the release records on the forges that carry releases.
type Mapper struct {
	// Records are the release records, one per commit.
	Records func(ctx context.Context) ([]releases.Record, error)
	// Tags are the repository's release tags, newest first.
	Tags func(ctx context.Context) ([]releases.Tag, error)
	// Dry plans, and writes nothing: each write is heard as it would be.
	Dry bool
	// Did hears each write, by the forge's key: created, edited or
	// attached, and the tag. Nil hears nothing.
	Did func(key, what, tag string)
	// Report counts the last run's writes.
	Report Report
}

// Report is what a run wrote.
type Report struct{ Created, Edited, Attached int }

var _ bsync.Mapper = (*Mapper)(nil)

func (m *Mapper) Kind() string { return releases.Kind.Name() }

// Carries tells whether the forge has releases.
func (m *Mapper) Carries(f forge.Forge) bool {
	_, ok := f.(forge.Releaser)
	return ok
}

// Run publishes when the way pushes; a pull takes nothing.
func (m *Mapper) Run(ctx context.Context, _ *kernel.Board, way bsync.Way, fs []forge.Forge) error {
	m.Report = Report{}
	if !way.Pushes() {
		return nil
	}
	recs, err := m.Records(ctx)
	if err != nil {
		return err
	}
	tags, err := m.Tags(ctx)
	if err != nil {
		return err
	}
	byCommit := map[string]releases.Record{}
	for _, r := range recs {
		byCommit[r.Commit] = r
	}
	for _, f := range fs {
		rel := f.(forge.Releaser)
		theirs, err := rel.Releases(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Key(), err)
		}
		for i := len(tags) - 1; i >= 0; i-- { // oldest first: the forge lists its newest first
			t := tags[i]
			r, ok := byCommit[t.Commit]
			if !ok {
				continue // a tag with no record yet: the runner's Reconcile makes it
			}
			if err := m.publish(ctx, f.Key(), rel, theirs, Want(t, r), nil); err != nil {
				return fmt.Errorf("%s %s: %w", f.Key(), t.Name, err)
			}
		}
	}
	return nil
}

// Want is the forge release a tag and its record make: titled by the tag,
// a candidate a prerelease, its body the record's notes, a yank said
// first.
func Want(t releases.Tag, r releases.Record) forge.Release {
	body := strings.TrimSpace(r.Notes)
	if r.Yanked != "" {
		body = strings.TrimSpace("**Yanked:** " + r.Yanked + "\n\n" + body)
	}
	return forge.Release{Tag: t.Name, Name: t.Name, Body: body, Prerelease: t.Version.RC != 0}
}

// Publish makes, or brings up to date, the forge's release of want.Tag,
// and attaches the files it lacks, by their base names: what a cut does
// once its binaries are built.
func Publish(ctx context.Context, f forge.Releaser, want forge.Release, files []string) error {
	theirs, err := f.Releases(ctx)
	if err != nil {
		return err
	}
	return (&Mapper{}).publish(ctx, "", f, theirs, want, files)
}

// publish makes or edits want on f, given its releases there, and
// attaches files.
func (m *Mapper) publish(ctx context.Context, key string, f forge.Releaser, theirs []forge.Release, want forge.Release, files []string) error {
	did := func(what string) {
		if m.Did != nil {
			m.Did(key, what, want.Tag)
		}
	}
	i := slices.IndexFunc(theirs, func(r forge.Release) bool { return r.Tag == want.Tag })
	var got forge.Release
	switch {
	case i < 0:
		m.Report.Created++
		did("created")
		if m.Dry {
			return nil
		}
		var err error
		if got, err = f.CreateRelease(ctx, want); err != nil {
			return err
		}
	case same(theirs[i], want):
		got = theirs[i]
	default:
		got = theirs[i]
		m.Report.Edited++
		did("edited")
		if !m.Dry {
			if err := f.EditRelease(ctx, got.ID, want); err != nil {
				return err
			}
		}
	}
	for _, file := range files {
		name := filepath.Base(file)
		if slices.Contains(got.Assets, name) {
			continue
		}
		m.Report.Attached++
		did("attached")
		if m.Dry {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if err := f.Attach(ctx, got.ID, name, data); err != nil {
			return err
		}
	}
	return nil
}

// same tells whether a forge release says what want does: GitLab keeps no
// prerelease, so a release there that isn't one says the same.
func same(have, want forge.Release) bool {
	return cmp.Or(have.Name, have.Tag) == want.Name && strings.TrimSpace(have.Body) == want.Body &&
		(have.Prerelease == want.Prerelease || !have.Prerelease && have.ID == have.Tag)
}
