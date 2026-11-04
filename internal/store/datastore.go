package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/core/pgstore"
	"github.com/guilbep/git-board/core/syncer"
)

// DatastoreEnv names the board's datastore, over git config's
// board.datastore: "git" (the default), a postgres:// URL, or "forge"
// (the forges alone, ForgeOnly). Neither is
// shared: a URL may hold a password, so it never goes in .git-board.toml.
const DatastoreEnv = "GIT_BOARD_DATASTORE"

// DatastoreKey is the git config key that names the datastore.
const DatastoreKey = "board.datastore"

// DatastoreURL is where this clone keeps its board, as configured:
// GIT_BOARD_DATASTORE, else board.datastore; "" for git.
func DatastoreURL(ctx context.Context, s *gitstore.Store) (string, error) {
	if v := os.Getenv(DatastoreEnv); v != "" {
		return v, nil
	}
	v, err := s.Config(ctx, DatastoreKey)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", DatastoreKey, err)
	}
	return v, nil
}

// IsPostgres says a datastore URL names PostgreSQL.
func IsPostgres(url string) bool {
	return strings.HasPrefix(url, "postgres://") || strings.HasPrefix(url, "postgresql://")
}

// openDatastore opens the datastore the clone names: s itself for git.
// The machine's folder stays <common dir>/board.
func openDatastore(ctx context.Context, s *gitstore.Store) (oplog.Datastore, error) {
	url, err := DatastoreURL(ctx, s)
	switch {
	case err != nil:
		return nil, err
	case url == "" || url == "git":
		return s, nil
	case url == ForgeOnly:
		return openForgeOnly(s)
	case IsPostgres(url):
		p, err := pgstore.Open(ctx, url, s.Folder())
		if err != nil {
			return nil, fmt.Errorf("opening the board's datastore (%s): %w", DatastoreKey, err)
		}
		return p, nil
	}
	return nil, fmt.Errorf("%s %q: git, forge, or a postgres:// URL", DatastoreKey, url)
}

// closeDatastore closes ds when it isn't the git repository.
func closeDatastore(ds oplog.Datastore, s *gitstore.Store) error {
	if c, ok := ds.(io.Closer); ok && ds != oplog.Datastore(s) {
		return c.Close()
	}
	return nil
}

// Where says where the board is kept, for a user to read: "git", or the
// datastore's name and server without its password.
func (b *Backend) Where() string {
	if w, ok := b.ds.(interface{ Where() string }); ok {
		return b.kind() + " " + w.Where()
	}
	return b.kind()
}

// Capabilities are what the board's datastore can do beyond keeping packs
// (ADR 0002, section 3): a control that needs one it lacks is shown
// disabled, with the sentence of missing as its title.
type Capabilities struct {
	Datastore string `json:"datastore"`
	Where     string `json:"where"`      // the server, without its password
	ForgeOnly bool   `json:"forge_only"` // each write goes to the forges at once
	Replica   bool   `json:"replica"`    // fetch, push, merge with remotes; start over
	Counter   bool   `json:"counter"`    // project numbers
}

// Capabilities says what the board's datastore can do.
func (b *Backend) Capabilities() Capabilities {
	_, counts := b.ds.(datastore.Counter)
	return Capabilities{Datastore: b.kind(), Where: b.Where(), ForgeOnly: b.forgeOnly(), Replica: b.inGit(), Counter: b.inGit() || counts}
}

// CopyFromGit copies the git repository's board into the datastore this
// clone names, entities that aren't there yet and the counter if it has
// none: a board leaving git keeps its ids. It answers how many entities it
// copied. The git refs stay as they are.
func (b *Backend) CopyFromGit(ctx context.Context) (int, error) {
	p, ok := b.ds.(*pgstore.Store)
	if !ok {
		return 0, errors.New("the board is kept in git: name a datastore first (git config " + DatastoreKey + " postgres://…)")
	}
	return p.Copy(ctx, gitCounter{b.s})
}

// gitCounter is the git repository as a datastore whose counter is
// refs/board/seq's numbers file, for a copy to carry the numbers.
type gitCounter struct {
	*gitstore.Store
}

func (g gitCounter) Count(context.Context) ([]byte, string, error) {
	c, ok, err := readCounter(g.Store)
	if err != nil || !ok {
		return nil, "", err
	}
	return c.Bytes(), "git", nil
}

func (g gitCounter) SetCount(context.Context, []byte, string) (string, error) {
	return "", errors.New("the git counter is pushed, not set")
}

// inGit says the board is kept in git: what only git does (a remote's fetch,
// push and merge, the reset and switched marks, the git counter) works.
func (b *Backend) inGit() bool { return b.ds == b.s }

// missing is the error for what the board's datastore can't do: the
// sentence the CLI prints and the server answers.
func (b *Backend) missing(what string) error {
	return datastore.Missing(what + " needs a board kept in git; this board is kept in " + b.kind())
}

// kind names where the board is kept, for a user to read.
func (b *Backend) kind() string {
	if n, ok := b.ds.(interface{ Name() string }); ok {
		return n.Name()
	}
	if b.inGit() {
		return "git"
	}
	return "another datastore"
}

// needGit is nil for a board kept in git, else missing(what).
func (b *Backend) needGit(what string) error {
	if b.inGit() {
		return nil
	}
	return b.missing(what)
}

// copyJournal copies the sync journal at from to to, when there is one.
func copyJournal(from, to string) error {
	data, err := os.ReadFile(from)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o644)
}

// ForgeOnly is board.datastore's value for a board kept on the forges
// alone (ADR 0002, section 4).
const ForgeOnly = "forge"

// forgeCopy is a forge-only board's datastore: a disposable git copy in
// the machine's folder, with no remote, that pull fills from the forges.
// Each write goes to the forges at once (OnWrite); throwing the copy away
// loses only what they hadn't got yet. Who I am stays in the clone's git
// config, and the machine's folder is the clone's.
type forgeCopy struct {
	copy, clone *gitstore.Store
}

// forgeCopyDir is where a clone keeps its forge-only copy.
func forgeCopyDir(clone *gitstore.Store) string {
	return filepath.Join(clone.Folder(), "forge-copy.git")
}

func openForgeOnly(clone *gitstore.Store) (oplog.Datastore, error) {
	dir := forgeCopyDir(clone)
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return nil, err
		}
		if out, err := exec.Command("git", "init", "-q", "--bare", dir).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("making the forge-only copy: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	c, err := gitstore.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("opening the forge-only copy: %w", err)
	}
	return &forgeCopy{copy: c, clone: clone}, nil
}

func (f *forgeCopy) Heads(ctx context.Context, kind string) (map[string]string, error) {
	return f.copy.Heads(ctx, kind)
}

func (f *forgeCopy) Packs(ctx context.Context, kind, id string) ([][]byte, string, error) {
	return f.copy.Packs(ctx, kind, id)
}

func (f *forgeCopy) Add(ctx context.Context, kind, id string, pack []byte, prev string) (string, error) {
	return f.copy.Add(ctx, kind, id, pack, prev)
}

func (f *forgeCopy) Folder() string { return f.clone.Folder() }

// Config and SetConfig are the clone's: board.me stays where it was.
func (f *forgeCopy) Config(ctx context.Context, key string) (string, error) {
	return f.clone.Config(ctx, key)
}

func (f *forgeCopy) SetConfig(ctx context.Context, key, value string) error {
	return f.clone.SetConfig(ctx, key, value)
}

// Name is where the board is kept, for a user to read.
func (f *forgeCopy) Name() string { return "the forges alone" }

// forgeOnly says the board is kept on the forges alone.
func (b *Backend) forgeOnly() bool { _, ok := b.ds.(*forgeCopy); return ok }

// ErrNotOnForge: a forge-only copy holds writes the forges haven't got:
// throwing it away would lose them.
var ErrNotOnForge = errors.New("issues the forges haven't got yet")

// DropCopy throws a forge-only board's copy away, and its sync journal: the
// next sync pulls the board again. It refuses while an issue isn't on a
// forge yet, unless force.
func (b *Backend) DropCopy(force bool) (int, error) {
	f, ok := b.ds.(*forgeCopy)
	if !ok {
		return 0, errors.New("the board isn't kept on the forges alone: there is no copy to throw away")
	}
	waiting, err := b.notOnForge()
	if err != nil {
		return 0, err
	}
	if len(waiting) > 0 && !force {
		return len(waiting), fmt.Errorf("%d %w (sync first, or --force to lose them)", len(waiting), ErrNotOnForge)
	}
	if err := syncer.Forget(b.ds); err != nil {
		return len(waiting), err
	}
	if err := os.RemoveAll(forgeCopyDir(f.clone)); err != nil {
		return len(waiting), err
	}
	c, err := openForgeOnly(f.clone)
	if err != nil {
		return len(waiting), err
	}
	f.copy = c.(*forgeCopy).copy
	b.mu.Lock()
	b.issues, b.persons, b.heads = map[string]*issue.Issue{}, map[string]*people.Person{}, map[string]string{}
	b.mu.Unlock()
	return len(waiting), nil
}

// notOnForge are the issues linked to no forge yet.
func (b *Backend) notOnForge() ([]string, error) {
	ids, err := issue.List(b.ds)
	if err != nil {
		return nil, err
	}
	records, err := syncer.Records(b.ds)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, id := range ids {
		if len(records[id]) == 0 {
			out = append(out, id)
		}
	}
	return out, nil
}

// wrote tells OnWrite an issue changed.
func (b *Backend) wrote(id string) {
	if f := b.OnWrite; f != nil {
		f(id)
	}
}
