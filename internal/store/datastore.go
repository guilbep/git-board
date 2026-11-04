package store

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/datastore/memstore"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/index"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/mdstore"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/core/pgstore"
	"github.com/guilbep/git-board/core/syncer"
)

// DatastoreEnv names the board's datastore, over git config's
// board.datastore: "git" (the default), a postgres:// URL, "forge" (the
// forges alone, ForgeOnly), or "markdown:<dir>" (a folder of Markdown
// files in the checkout, Markdown). Neither is
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
func openDatastore(ctx context.Context, s *gitstore.Store, root string) (oplog.Datastore, string, error) {
	ds, url, err := openNamed(ctx, s, root)
	return ds, cmp.Or(url, "git"), err
}

func openNamed(ctx context.Context, s *gitstore.Store, root string) (oplog.Datastore, string, error) {
	url, err := DatastoreURL(ctx, s)
	switch {
	case err != nil:
		return nil, url, err
	case url == "" || url == "git":
		return s, url, nil
	case url == ForgeOnly:
		ds, err := openForgeOnly(s)
		return ds, url, err
	case IsMarkdown(url):
		ds, err := mdstore.Open(markdownDir(root, url), s.Folder())
		return ds, url, err
	case IsPostgres(url):
		p, err := pgstore.Open(ctx, url, s.Folder())
		if err != nil {
			return nil, url, fmt.Errorf("opening the board's datastore (%s): %s", DatastoreKey, Redact(err.Error()))
		}
		return p, url, nil
	}
	return nil, url, fmt.Errorf("%s %q: git, forge, markdown:<dir>, or a postgres:// URL", DatastoreKey, Redact(url))
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
// clone names, the packs that aren't there yet and the counter if it has
// none: a board leaving git keeps its ids. It answers how many entities it
// copied. The git refs stay as they are.
func (b *Backend) CopyFromGit(ctx context.Context) (int, error) {
	if m, ok := b.ds.(*mdstore.Store); ok {
		n, err := datastore.Copy(ctx, gitCounter{b.s}, m)
		if err == nil {
			err = b.renderMarkdown()
		}
		return n, err
	}
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
var ErrNotOnForge = errors.New("issues with writes the forges may not have yet")

// DropCopy throws a forge-only board's issues and sync records away, and
// the sync journal: the next sync pulls them again. The persons and the
// release records, which no forge keeps, stay, so who you write as still
// names someone. It refuses while an issue has a write made here since the
// last whole sync, unless force; it answers how many have.
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
	// what no forge keeps goes to the new copy first: persons, releases
	keep := memstore.New("")
	for _, kind := range []string{"person", "release"} {
		heads, err := f.copy.Heads(ctx, kind)
		if err != nil {
			return len(waiting), err
		}
		for id := range heads {
			packs, _, err := f.copy.Packs(ctx, kind, id)
			if err != nil {
				return len(waiting), err
			}
			v := ""
			for _, p := range packs {
				if v, err = keep.Add(ctx, kind, id, p, v); err != nil {
					return len(waiting), err
				}
			}
		}
	}
	if err := os.RemoveAll(forgeCopyDir(f.clone)); err != nil {
		return len(waiting), err
	}
	c, err := openForgeOnly(f.clone)
	if err != nil {
		return len(waiting), err
	}
	f.copy = c.(*forgeCopy).copy
	if _, err := datastore.Copy(ctx, keep, f.copy); err != nil {
		return len(waiting), err
	}
	b.mu.Lock()
	b.issues, b.persons, b.heads = map[string]*issue.Issue{}, map[string]*people.Person{}, map[string]string{}
	b.mu.Unlock()
	return len(waiting), nil
}

// notOnForge are the issues the forges may lack something of: linked to
// none, or with a write made here (no forge its origin) since the last
// whole sync. A write the server sent at once counts until a whole sync
// runs: it errs on keeping.
func (b *Backend) notOnForge() ([]string, error) {
	ids, err := issue.List(b.ds)
	if err != nil {
		return nil, err
	}
	records, err := syncer.Records(b.ds)
	if err != nil {
		return nil, err
	}
	last, err := syncer.LastRun(b.ds)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, id := range ids {
		if len(records[id]) == 0 {
			out = append(out, id)
			continue
		}
		l, err := oplog.Load(b.ds, "issue", id)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(l.Entries, func(e oplog.Entry) bool { return e.Origin == nil && e.Time >= last }) {
			out = append(out, id)
		}
	}
	return out, nil
}

// wrote tells OnWrite an issue changed.
func (b *Backend) wrote(id string) {
	if _, ok := b.ds.(*mdstore.Store); ok {
		if err := b.renderMarkdown(); err != nil {
			log.Printf("markdown: %v", err)
		}
	}
	if f := b.OnWrite; f != nil {
		f(id)
	}
}

// Markdown is board.datastore's prefix for a board kept in a folder of
// Markdown files, relative to the checkout's top: markdown:issues.
const Markdown = "markdown:"

// IsMarkdown says a board.datastore value names a Markdown folder.
func IsMarkdown(v string) bool { return strings.HasPrefix(v, Markdown) }

// markdownDir is the folder a markdown:<dir> value names, from the
// checkout's top.
func markdownDir(root, v string) string {
	dir := strings.TrimSpace(strings.TrimPrefix(v, Markdown))
	if filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(root, dir)
}

// readMarkdown reads a Markdown board's hand edits in before a write
// (ImportMarkdown), while the files still say what the board held when
// they were written: a write then never takes its own issue's file, older
// than itself, for a hand edit. The import's own writes skip it. A hand
// edit to an issue changed here since its file was written is a conflict,
// logged; the file takes the board's at the render after the write.
func (b *Backend) readMarkdown() {
	m, ok := b.ds.(*mdstore.Store)
	if !ok || !b.mdImporting.CompareAndSwap(false, true) {
		return
	}
	defer b.mdImporting.Store(false)
	b.mdMu.Lock()
	defer b.mdMu.Unlock()
	rep, err := b.ImportMarkdown(m.Dir())
	if err != nil {
		log.Printf("markdown: reading %s: %v", m.Dir(), err)
	}
	for _, c := range rep.Conflicts {
		log.Printf("markdown: %s", c)
	}
	if len(rep.Created)+len(rep.Updated) > 0 {
		if _, err := b.ExportMarkdown(m.Dir(), nil); err != nil {
			log.Printf("markdown: %v", err)
		}
	}
}

// renderMarkdown writes a Markdown board's files where an issue changed.
// ponytail: reads and writes every file on each write; per issue when
// boards in a folder grow past a few hundred issues
func (b *Backend) renderMarkdown() error {
	if b.mdImporting.Load() {
		return nil // the import renders once it is done
	}
	b.mdMu.Lock()
	defer b.mdMu.Unlock()
	_, err := b.ExportMarkdown(b.ds.(*mdstore.Store).Dir(), nil)
	return err
}

// MarkdownDir is the folder a Markdown board is kept in, "" for another.
func (b *Backend) MarkdownDir() string {
	if m, ok := b.ds.(*mdstore.Store); ok {
		return m.Dir()
	}
	return ""
}

// renderTo writes a Markdown folder's files from its packs, read through
// a backend of their own: a copy's, before the clone names it.
func (b *Backend) renderTo(m *mdstore.Store) error {
	o := &Backend{s: b.s, ds: m, root: b.root, issues: map[string]*issue.Issue{}, persons: map[string]*people.Person{}, heads: map[string]string{}}
	ix, err := index.Open(context.Background(), memIndexFolder{m, b.s.Folder()})
	if err != nil {
		return err
	}
	o.ix = ix
	defer ix.Close()
	_, err = o.ExportMarkdown(m.Dir(), nil)
	return err
}

// memIndexFolder keeps a throwaway index beside a copy, not over the
// clone's own.
type memIndexFolder struct {
	*mdstore.Store
	parent string
}

func (m memIndexFolder) Folder() string { return filepath.Join(m.parent, "copy-index") }

// testMarkdown says what a Markdown folder holds, writing nothing.
func (b *Backend) testMarkdown(ctx context.Context, v string) TestResult {
	dir := markdownDir(b.root, v)
	rel, _ := filepath.Rel(b.root, dir)
	if strings.HasPrefix(rel, "..") {
		return TestResult{Line: dir + " is outside the checkout: the board wouldn't go with the code"}
	}
	n := 0
	for _, kind := range datastore.Kinds {
		entries, _ := os.ReadDir(filepath.Join(dir, mdstore.Board, kind))
		n += len(entries)
	}
	if n == 0 {
		return TestResult{OK: true, Line: "the folder " + rel + ": no board there yet; git-board makes " + rel + "/" + mdstore.Board + " when it opens it"}
	}
	return TestResult{OK: true, Line: fmt.Sprintf("the folder %s: a board of %d entities", rel, n)}
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

// Redact hides a datastore value's password: a URL's, in its user part or
// its query, or a key=value string's.
func Redact(v string) string {
	if u, err := url.Parse(v); err == nil && u.Scheme != "" {
		if _, has := u.User.Password(); u.User != nil && has {
			u.User = url.UserPassword(u.User.Username(), "xxxxx")
		}
		if q := u.Query(); q.Has("password") {
			q.Set("password", "xxxxx")
			u.RawQuery = q.Encode()
		}
		return u.String()
	}
	return kvPassword.ReplaceAllString(v, "${1}xxxxx")
}

// kvPassword is a key=value string's password, quoted or not.
var kvPassword = regexp.MustCompile(`(password\s*=\s*)('(?:[^'\\]|\\.)*'|\S+)`)

// CheckDatastore refuses a board.datastore value git-board can't open.
func CheckDatastore(v string) error {
	switch {
	case v == "git", v == ForgeOnly:
		return nil
	case IsMarkdown(v):
		if strings.TrimSpace(strings.TrimPrefix(v, Markdown)) == "" {
			return fmt.Errorf("%q: name the folder, markdown:issues", v)
		}
		return nil
	case IsPostgres(v):
		_, err := pgx.ParseConfig(v)
		return err
	}
	return fmt.Errorf("%q: git, forge, markdown:<dir>, or a postgres:// URL", v)
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
	case IsMarkdown(v):
		return b.testMarkdown(ctx, v)
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

// CopyGitTo copies the board in git into the datastore v names, as git
// board datastore copy does: only what isn't there yet is written.
func (b *Backend) CopyGitTo(ctx context.Context, v string) (int, error) {
	if IsMarkdown(v) {
		m, err := mdstore.Open(markdownDir(b.root, v), b.s.Folder())
		if err != nil {
			return 0, err
		}
		n, err := datastore.Copy(ctx, gitCounter{b.s}, m)
		if err == nil {
			err = b.renderTo(m)
		}
		return n, err
	}
	if !IsPostgres(v) {
		return 0, errors.New("a copy goes to a postgres:// URL or a markdown:<dir> folder")
	}
	p, err := pgstore.Open(ctx, v, b.s.Folder())
	if err != nil {
		return 0, errors.New(Redact(err.Error()))
	}
	defer p.Close()
	return p.Copy(ctx, gitCounter{b.s})
}

// whoStore is where who you write as is kept: this clone's git config
// (board.me), whatever keeps the board, for it is yours, not the board's.
// A datastore with no clone keeps it in its folder (people.Me).
func (b *Backend) whoStore() oplog.Datastore { return withConfig{b.ds, b.s} }

// withConfig is a datastore with the clone's git config.
type withConfig struct {
	oplog.Datastore
	clone *gitstore.Store
}

func (w withConfig) Config(ctx context.Context, key string) (string, error) {
	return w.clone.Config(ctx, key)
}

func (w withConfig) SetConfig(ctx context.Context, key, value string) error {
	return w.clone.SetConfig(ctx, key, value)
}
