package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/pgstore"
)

// DatastoreEnv names the board's datastore, over git config's
// board.datastore: "git" (the default) or a postgres:// URL. Neither is
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
	case IsPostgres(url):
		p, err := pgstore.Open(ctx, url, s.Folder())
		if err != nil {
			return nil, fmt.Errorf("opening the board's datastore (%s): %w", DatastoreKey, err)
		}
		return p, nil
	}
	return nil, fmt.Errorf("%s %q: git, or a postgres:// URL", DatastoreKey, url)
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
	Replica   bool   `json:"replica"` // fetch, push, merge with remotes; start over
	Counter   bool   `json:"counter"` // project numbers
}

// Capabilities says what the board's datastore can do.
func (b *Backend) Capabilities() Capabilities {
	_, counts := b.ds.(datastore.Counter)
	return Capabilities{Datastore: b.kind(), Replica: b.inGit(), Counter: b.inGit() || counts}
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
