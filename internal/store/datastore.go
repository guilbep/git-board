package store

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

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
func openDatastore(ctx context.Context, s *gitstore.Store) (oplog.Datastore, string, error) {
	ds, url, err := openNamed(ctx, s)
	return ds, cmp.Or(url, "git"), err
}

func openNamed(ctx context.Context, s *gitstore.Store) (oplog.Datastore, string, error) {
	url, err := DatastoreURL(ctx, s)
	switch {
	case err != nil:
		return nil, url, err
	case url == "" || url == "git":
		return s, url, nil
	case url == ForgeOnly:
		ds, err := openForgeOnly(s)
		return ds, url, err
	case IsPostgres(url):
		p, err := pgstore.Open(ctx, url, s.Folder())
		if err != nil {
			return nil, url, fmt.Errorf("opening the board's datastore (%s): %s", DatastoreKey, Redact(err.Error()))
		}
		return p, url, nil
	}
	return nil, url, fmt.Errorf("%s %q: git, forge, or a postgres:// URL", DatastoreKey, Redact(url))
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

// DatastoreSetting is this clone's board.datastore as a page may show it,
// its password hidden: "git" when unset. env says GIT_BOARD_DATASTORE sets
// it, over git config, and a page can't change it.
func (b *Backend) DatastoreSetting() (value string, env, restart bool, err error) {
	v, err := DatastoreURL(ctx, b.s)
	if err != nil {
		return "", false, false, err
	}
	v = cmp.Or(v, "git")
	return Redact(v), os.Getenv(DatastoreEnv) != "", v != b.openedAs, nil
}

// Unredact is the setting a page sent back: its own value when the page
// sent the redacted one it was shown, so the password stays.
func (b *Backend) Unredact(v string) string {
	cur, err := DatastoreURL(ctx, b.s)
	if err == nil && cur != "" && v == Redact(cur) {
		return cur
	}
	return v
}

// Redact hides a URL's password.
func Redact(v string) string {
	if u, err := url.Parse(v); err == nil && u.User != nil {
		if _, has := u.User.Password(); has {
			return u.Redacted()
		}
	}
	return v
}

// CheckDatastore refuses a board.datastore value git-board can't open.
func CheckDatastore(v string) error {
	switch {
	case v == "git", v == ForgeOnly:
		return nil
	case IsPostgres(v):
		_, err := pgx.ParseConfig(v)
		return err
	}
	return fmt.Errorf("%q: git, forge, or a postgres:// URL", v)
}

// SetDatastore writes board.datastore in this clone's git config ("git"
// unsets it). It takes effect when git-board opens the repository again.
func (b *Backend) SetDatastore(v string) error {
	if err := CheckDatastore(v); err != nil {
		return err
	}
	if v == "git" {
		_, err := b.run(nil, "config", "--local", "--unset", DatastoreKey)
		if ee := (*exec.ExitError)(nil); errors.As(err, &ee) && ee.ExitCode() == 5 {
			return nil // wasn't set
		}
		return err
	}
	_, err := b.run(nil, "config", "--local", DatastoreKey, v)
	return err
}

// TestResult is what a connection test found.
type TestResult struct {
	OK     bool           `json:"ok"`
	Line   string         `json:"line"` // what a user reads
	Probe  *pgstore.Probe `json:"probe,omitempty"`
	Copied int            `json:"copied,omitempty"`
}

// TestDatastore tries the datastore v names, writing nothing.
func (b *Backend) TestDatastore(ctx context.Context, v string) TestResult {
	if err := CheckDatastore(v); err != nil {
		return TestResult{Line: err.Error()}
	}
	switch {
	case v == "git":
		return TestResult{OK: true, Line: "the board in this repository's refs/board/*"}
	case v == ForgeOnly:
		return TestResult{OK: true, Line: "a copy in .git/board, filled from the forges with sync on"}
	}
	p, err := pgstore.Try(ctx, v)
	if err != nil {
		return TestResult{Line: Redact(err.Error()), Probe: &p}
	}
	line := fmt.Sprintf("connected to PostgreSQL %s at %s: ", p.Server, p.Where)
	switch {
	case !p.Tables:
		line += "no board there yet; git-board makes its tables when it opens it"
	case p.Entities == 0:
		line += "git-board's tables, empty"
	default:
		line += fmt.Sprintf("a board of %d entities", p.Entities)
	}
	return TestResult{OK: true, Line: line, Probe: &p}
}

// CopyGitTo copies the board in git into the PostgreSQL database v names,
// as git board datastore copy does: what is there already is left alone.
func (b *Backend) CopyGitTo(ctx context.Context, v string) (int, error) {
	if !IsPostgres(v) {
		return 0, errors.New("a copy goes to a postgres:// URL")
	}
	p, err := pgstore.Open(ctx, v, b.s.Folder())
	if err != nil {
		return 0, errors.New(Redact(err.Error()))
	}
	defer p.Close()
	return p.Copy(ctx, gitCounter{b.s})
}
