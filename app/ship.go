package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/guilbep/git-board/domain/releases"
	"github.com/guilbep/git-board/domain/releases/releasesync"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/tools/ship"
)

// The release runner (tools/ship), wired: the board's records, the
// tools it runs, and the forge it publishes on, whose token is here.

// ErrNoRelease is a release command on a project without releases.
var ErrNoRelease = errors.New("no [release] table with environments or binaries in .git-board.toml")

// ShipTools makes the tools a runner runs from its publisher: a var, for
// the tests to swap in fakes.
var ShipTools = ship.Exec

// Runner is the release runner of the board's repository: [release] and
// [branches], its records written as the clone's board.me, its commits
// signed by their login, and its binaries published on the authority
// remote's forge.
func (b *Board) Runner(ctx context.Context) (*ship.Runner, error) {
	if !b.Config.Release.Enabled() {
		return nil, ErrNoRelease
	}
	who := ""
	if me, ok, err := b.People.WhoAmI(ctx); err == nil && ok {
		who = me.Login
	}
	name, open := b.releaseForge(ctx)
	publish := func(ctx context.Context, tag, notes string, files []string) error {
		f, err := open(ctx)
		if err != nil {
			return err
		}
		v, _ := releases.ParseTag(tag)
		return releasesync.Publish(ctx, f, forge.Release{Tag: tag, Name: tag, Body: strings.TrimSpace(notes), Prerelease: v.RC != 0}, files)
	}
	r, err := ship.New(ctx, b.Config.Release, b.Config.Branches, b.Dir, b.Book(), ShipTools(publish), who)
	if err != nil {
		return nil, err
	}
	r.Forge = name
	return r, nil
}

// releaseForge is the forge of the authority remote, as people name it
// ("" when the remote is on none), and how to open it: a token is asked
// for only when a release is published.
func (b *Board) releaseForge(ctx context.Context) (string, func(context.Context) (forge.Releaser, error)) {
	remote := b.Config.Release.RemoteName()
	out, _ := gitRun(ctx, b.Dir, "remote", "get-url", remote)
	t, _ := forge.FromRemote(NoCredentials(strings.TrimSpace(out)))
	open := func(ctx context.Context) (forge.Releaser, error) {
		if t.Kind == "" {
			return nil, fmt.Errorf("%s isn't on github.com nor a GitLab host: git-board publishes releases there only", remote)
		}
		t.Name, t.Sync = t.Kind, true
		project, err := resolveProject(ctx, t)
		if err != nil {
			return nil, fmt.Errorf("opening %s %s: %w", t.Host, t.Project, err)
		}
		t.Project = project
		fs, err := b.OpenForges(ctx, []forge.Table{t})
		if err != nil {
			return nil, err
		}
		rel, ok := fs[0].Forge.(forge.Releaser)
		if !ok {
			return nil, fmt.Errorf("%s carries no releases", t.Host)
		}
		return rel, nil
	}
	switch t.Kind {
	case "github":
		return "GitHub", open
	case "gitlab":
		return "GitLab", open
	}
	return "", open
}

// releaseMapper is the releases' mapper over the board's records and the
// repository's tags.
func (b *Board) releaseMapper(dry bool, did func(key, what, tag string)) *releasesync.Mapper {
	return &releasesync.Mapper{Dry: dry, Did: did,
		Records: b.Book().Records,
		Tags:    func(ctx context.Context) ([]releases.Tag, error) { return ship.ReleaseTags(ctx, b.Dir) },
	}
}
