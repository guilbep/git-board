// Package app opens a board and wires its domains: the one API every
// surface calls (docs/rewrite/design.md, The application). A surface
// decodes a request, calls a service here, and encodes the answer; it
// holds no rule. Errors carry their class: the exit code and the HTTP
// status, decided once.
package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/guilbep/git-board/conf"
	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/domain/issues/mdfiles"
	"github.com/guilbep/git-board/domain/people"
	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/store"
	"github.com/guilbep/git-board/store/forgecopy"
	"github.com/guilbep/git-board/store/git"
	"github.com/guilbep/git-board/store/md"
	"github.com/guilbep/git-board/store/pg"
	"github.com/guilbep/git-board/view"
)

// Board is an open board and its services.
type Board struct {
	Issues *issues.Service
	People *people.Service
	Config *Config
	Views  map[string]issues.View
	Kernel *kernel.Board
	Dir    string // the repository's working directory, or the folder opened
	// Files are a Markdown folder's rendered files; nil on another
	// datastore.
	Files *mdfiles.Folder
	// PRs are the pull requests the cycle reads, the server's cache for
	// its life; nil reads none (the CLI).
	PRs   *PullCache
	clone *clone
	work  workKept // what the Work page computed (work.go)
}

// Options say how to open a board.
type Options struct {
	// Who reads the clone's board.me and board.avatars: what show and log
	// print of people, @me, and every write.
	Who bool
	// Warm starts reading an entity's packs at once: show and log read
	// one.
	Warm bool
	// Notice is told what a read leaves aside (an entity broken, a
	// counter that doesn't read), as v1 logs it.
	Notice issues.Notice
}

// Open opens the board of the repository holding dir: the datastore the
// clone names (GIT_BOARD_DATASTORE, else its board.datastore: git, a
// markdown:<dir> folder, a postgres:// URL), its view (in
// <common dir>/board/v2/view.db, or in the board's PostgreSQL database;
// v1's index is v1's), .git-board.toml, and with o.Who who writes.
func Open(ctx context.Context, dir string, o Options) (*Board, error) {
	settled := make(chan set, 1)
	go func() { settled <- settings(ctx, dir) }() // git config runs while the rest opens
	g, err := git.Open(ctx, dir)
	if err != nil {
		return nil, err
	}
	f, err := conf.Load(configPath(dir))
	if err != nil {
		return nil, err
	}
	set := <-settled
	if set.err != nil {
		return nil, set.err
	}
	cfg, err := ReadConfig(f, set.experimental)
	if err != nil {
		return nil, err
	}
	if o.Notice != nil {
		if v := f.Version(); v > conf.Current {
			o.Notice("warning: %s is schema %d, newer than this git-board's %d: it reads what it knows, writes nothing to it, and ignores the rest; upgrade git-board", f.Path(), v, conf.Current)
		}
		for _, k := range f.Unknown() {
			o.Notice("warning: %s: %s: git-board doesn't know this key, ignored (git board lint)", f.Path(), k)
		}
	}
	schema, views := cfg.Schema, cfg.Views
	ds, err := OpenDatastore(ctx, g, dir, set.datastore)
	if err != nil {
		return nil, err
	}
	if o.Warm && ds == store.Datastore(g) {
		go g.Warm(ctx)
	}
	var v kernel.View
	if c := ds.Caps(); c.Shared && c.SQL != nil {
		v, err = view.OpenPG(ctx, c.SQL.DSN(), filepath.Join(ds.Folder(), "v2", "pg"))
	} else {
		v, err = view.Open(ctx, filepath.Join(ds.Folder(), "v2", "view.db"))
	}
	if err != nil {
		ds.Close()
		return nil, fmt.Errorf("opening the view: %w", err)
	}
	kb, err := kernel.Open(ctx, ds, v, issues.Kind, people.Kind)
	if err != nil {
		v.Close()
		return nil, err
	}
	c := &clone{dir: dir}
	b := &Board{Kernel: kb, Config: cfg, Views: views, Dir: dir, clone: c,
		Issues: &issues.Service{Board: kb, Schema: schema, Avatars: people.Forge, Notice: o.Notice, Folder: ds.Folder(), Views: views, Held: cfg.Held,
			GitHub: cfg.gitHubRepo()},
		People: &people.Service{Board: kb, Avatars: people.Forge, SetMe: c.setMe}}
	if o.Who {
		b.Issues.Me, b.Issues.Avatars = kernel.ID(set.me), people.ParseAvatars(set.avatars)
		b.People.Me, b.People.Avatars = b.Issues.Me, b.Issues.Avatars
	}
	if m, ok := ds.(*md.Store); ok { // its files: read in before each write, rendered after
		notice := o.Notice
		if notice == nil {
			notice = func(string, ...any) {}
		}
		b.Files = mdfiles.Open(m.Dir(), ds.Folder(), m.LockFiles, b.Issues, notice)
	}
	return b, nil
}

// DatastoreEnv names the board's datastore, over board.datastore.
const DatastoreEnv = "GIT_BOARD_DATASTORE"

// DatastoreKey is the clone's git config key naming its datastore.
const DatastoreKey = "board.datastore"

// OpenDatastore opens the datastore a board.datastore value names, for
// the repository g at dir: g itself for "" and git. Its own folder stays
// g's (<common dir>/board): the view, the inbox, the sync lock.
func OpenDatastore(ctx context.Context, g *git.Store, dir, name string) (store.Datastore, error) {
	switch {
	case name == "" || name == "git":
		return g, nil
	case strings.HasPrefix(name, md.Prefix):
		folder := strings.TrimSpace(strings.TrimPrefix(name, md.Prefix))
		if !filepath.IsAbs(folder) {
			top, err := gitRun(ctx, dir, "rev-parse", "--show-toplevel")
			if err != nil {
				return nil, err
			}
			folder = filepath.Join(strings.TrimSpace(top), folder)
		}
		return md.Open(folder, g.Folder())
	case pg.IsURL(name):
		p, err := pg.Open(ctx, name, g.Folder())
		if err != nil {
			return nil, fmt.Errorf("opening the board's datastore (%s): %s", DatastoreKey, pg.Redact(err.Error()))
		}
		return p, nil
	case name == forgecopy.Name: // filled by the sync, M4's
		return forgecopy.Open(ctx, g.Folder())
	}
	return nil, fmt.Errorf("%s %q: git, forge, markdown:<dir>, or a postgres:// URL", DatastoreKey, pg.Redact(name))
}

// Close lets the board go.
func (b *Board) Close() error { return b.Kernel.Close() }

// configPath is the nearest .git-board.toml up to the repository's root,
// else where it goes at that root.
func configPath(dir string) string {
	for {
		p := filepath.Join(dir, conf.FileName)
		if _, err := os.Stat(p); err == nil {
			return p
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return conf.FileName
		}
		dir = parent
	}
}

// set is the clone's settings.
type set struct {
	me, avatars, datastore string
	experimental           []string // nil: unset
	err                    error
}

// settings are the clone's board.me and board.datastore, from its own
// config only (data-model.md, Me; v1's board.datastore), and
// board.avatars, from any level: one git. GIT_BOARD_DATASTORE wins over
// board.datastore.
func settings(ctx context.Context, dir string) (s set) {
	defer func() {
		if v := os.Getenv(DatastoreEnv); v != "" {
			s.datastore = v
		}
	}()
	out, err := gitRun(ctx, dir, "config", "--show-scope", "--get-regexp", `^board\.(me|avatars|datastore|experimental)$`)
	if ee := (*exec.ExitError)(nil); errors.As(err, &ee) && ee.ExitCode() == 1 {
		return s // none is set
	}
	if err != nil {
		return set{err: err}
	}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		scope, kv, _ := strings.Cut(line, "\t")
		k, v, _ := strings.Cut(kv, " ")
		switch {
		case k == "board.me" && scope == "local":
			s.me = v
		case k == "board.datastore" && scope == "local":
			s.datastore = v
		case k == "board.avatars":
			s.avatars = v // the last level wins, as git's --get
		case k == LocalExperimental:
			s.experimental = SplitFeatures(v)
		}
	}
	return s
}

// clone is this clone's own git config: who writes.
type clone struct{ dir string }

// setMe writes board.me in the clone's own config, never the global one.
func (c *clone) setMe(ctx context.Context, id kernel.ID) error {
	_, err := gitRun(ctx, c.dir, "config", "--local", "board.me", string(id))
	return err
}

// GitConfig is a git config value at any level, "" when unset: user.name
// for me --new.
func (b *Board) GitConfig(ctx context.Context, key string) string {
	out, err := gitRun(ctx, b.Dir, "config", "--get", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func gitRun(ctx context.Context, dir string, args ...string) (string, error) {
	return gitStdin(ctx, dir, nil, args...)
}

// gitStdin runs git as gitRun does, stdin given.
func gitStdin(ctx context.Context, dir string, stdin []byte, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// Views reads the views .git-board.toml names, without opening the board.
func Views(dir string) (map[string]issues.View, error) {
	f, err := conf.Load(configPath(dir))
	if err != nil {
		return nil, err
	}
	views, probs := issues.Declare(f).Views()
	if len(probs) > 0 {
		return nil, fmt.Errorf("%s: %w", conf.FileName, probs[0])
	}
	return views, nil
}

// AvatarsKey is the git config key saying where pictures come from.
const AvatarsKey = "board.avatars"

// SetAvatars writes board.avatars in the clone's own config: forge,
// gravatar or none; "" unsets it, back to forge.
func (b *Board) SetAvatars(ctx context.Context, mode string) error {
	switch mode {
	case "":
		_, err := gitRun(ctx, b.Dir, "config", "--local", "--unset-all", AvatarsKey)
		if ee := (*exec.ExitError)(nil); errors.As(err, &ee) && ee.ExitCode() == 5 {
			return nil // it wasn't set
		}
		return err
	case string(people.Forge), string(people.Gravatar), string(people.None):
		_, err := gitRun(ctx, b.Dir, "config", "--local", AvatarsKey, mode)
		return err
	}
	return &Error{Class: Usage, Err: fmt.Errorf("%s %q: forge, gravatar or none", AvatarsKey, mode)}
}

// Remotes are the repository's git remotes, by name.
func (b *Board) Remotes(ctx context.Context) ([]string, error) {
	out, err := gitRun(ctx, b.Dir, "remote")
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

// IndexTexts builds the issues' free-text index now: a server does it in
// the background once it serves, so that its first search is fast; the
// CLI leaves it to the first query that searches.
func (b *Board) IndexTexts(ctx context.Context) error { return b.Kernel.Index(ctx, issues.Kind) }
