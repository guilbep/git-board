// Package index keeps an index of the issues and persons, for lists, search
// and lookups by account. It is never the source of truth: every query
// first brings it up to the datastore.
package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
)

// version is the schema's, in PRAGMA user_version. 2: PostgreSQL's texts
// weigh the title, body and comments apart, for Query.In. 3: each issue's
// state, for Folded.
const version = 3

// schema is the index's tables. entities holds every indexed entity's head;
// error is NULL when it loaded, and its error when it is broken. An issue's
// state is the JSON of what issue.Apply folded at that head. texts' rowid
// is its issue's n.
const schema = `
CREATE TABLE entities (kind TEXT NOT NULL, id TEXT NOT NULL, head TEXT NOT NULL, error TEXT,
	PRIMARY KEY (kind, id));
CREATE TABLE issues (n INTEGER PRIMARY KEY, id TEXT NOT NULL UNIQUE, title TEXT NOT NULL,
	status TEXT NOT NULL, author TEXT NOT NULL, created INTEGER NOT NULL, updated INTEGER NOT NULL,
	comments INTEGER NOT NULL, labels TEXT NOT NULL, state TEXT NOT NULL);
CREATE TABLE labels (issue TEXT NOT NULL, label TEXT NOT NULL, PRIMARY KEY (label, issue));
CREATE INDEX labels_issue ON labels (issue);
CREATE TABLE persons (id TEXT PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE accounts (server TEXT NOT NULL, id TEXT NOT NULL, person TEXT NOT NULL, login TEXT NOT NULL,
	PRIMARY KEY (server, id, person));
CREATE INDEX accounts_person ON accounts (person);
CREATE VIRTUAL TABLE texts USING fts5 (title, body, comments, tokenize = 'unicode61');
`

// kinds are the indexed kinds. Sync entities aren't.
var kinds = []string{"issue", "person"}

// errRebuild says the file must be built again.
var errRebuild = errors.New("index: not an index of this version")

// Index is the index of one board: a sqlite file in the datastore's
// folder, or the datastore's own server when it keeps one (SQL).
type Index struct {
	s  oplog.Datastore
	db *sql.DB
	pg bool // PostgreSQL's SQL, not sqlite's
}

// SQL is a datastore whose server keeps the index too: PostgreSQL. Every
// machine then reads one index, which replaces the sqlite file.
type SQL interface {
	// IndexDB is a pool on the server whose search path is schema, made
	// when it isn't there: the index's tables (PGSchema).
	IndexDB(ctx context.Context, schema string) (*sql.DB, error)
}

// PGSchema is the index's schema on PostgreSQL, one per version of its
// tables: two git-board versions sharing a database each keep their own
// index, instead of dropping the other's at every open. An older one's is
// left as it is, as a git-board of that version may still read it; it is
// dropped by hand (docs/adr/0009).
var PGSchema = "git_board_index_v" + strconv.Itoa(version)

// Broken is an entity that failed to load.
type Broken struct{ Kind, ID, Error string }

// Open opens the index at index.db in the datastore's folder (for git,
// <common dir>/board). A file that is missing, that sqlite can't read, or
// of another version is built again.
func Open(ctx context.Context, s oplog.Datastore) (*Index, error) {
	if q, ok := s.(SQL); ok {
		return openPG(ctx, s, q)
	}
	dir := s.Folder()
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return nil, fmt.Errorf("index: %w", err)
	}
	path := filepath.Join(dir, "index.db")
	db, err := open(ctx, path)
	if errors.Is(err, errRebuild) {
		// ponytail: another process with the old file open keeps it, unlinked,
		// as its own cache until it reopens; it still answers from the refs.
		for _, p := range []string{path, path + "-wal", path + "-shm"} {
			if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("index: rebuild: %w", err)
			}
		}
		db, err = open(ctx, path)
	}
	if err != nil {
		return nil, err
	}
	return &Index{s: s, db: db}, nil
}

// open opens path and checks its version. An empty file gets the schema.
// It is errRebuild when the file isn't an index of this version.
func open(ctx context.Context, path string) (*sql.DB, error) {
	// _txlock=immediate: a refresh takes the write lock when it begins, so
	// it waits for another's (busy_timeout) instead of failing on upgrade.
	// The path is escaped: sqlite reads a file: name as a URI.
	dsn := "file:" + (&url.URL{Path: path}).EscapedPath() + "?_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("index: %w", err)
	}
	if err := setup(ctx, db); err != nil {
		db.Close()
		var se *sqlite.Error
		if errors.As(err, &se) && (se.Code()&0xff == sqlite3.SQLITE_NOTADB || se.Code()&0xff == sqlite3.SQLITE_CORRUPT) {
			err = fmt.Errorf("%w: %w", errRebuild, err)
		}
		return nil, fmt.Errorf("index: open %s: %w", path, err)
	}
	return db, nil
}

// setup checks the version, and writes the schema into an empty file. The
// check is made again under the write lock, as another process may be
// writing the schema too.
func setup(ctx context.Context, db *sql.DB) error {
	var v, objects int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil || v == version {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, "SELECT user_version, (SELECT count(*) FROM sqlite_schema) FROM pragma_user_version").Scan(&v, &objects)
	switch {
	case err != nil || v == version:
		return err
	case v != 0 || objects > 0:
		return fmt.Errorf("%w: version %d", errRebuild, v)
	}
	if _, err := tx.ExecContext(ctx, schema+fmt.Sprintf("PRAGMA user_version = %d;", version)); err != nil {
		return err
	}
	return tx.Commit()
}

// Close closes the index.
func (ix *Index) Close() error { return ix.db.Close() }

type key struct{ kind, id string }

// loaded is an entity read at head: its state, or the error loading it.
type loaded struct {
	key
	head   string
	issue  *issue.Issue
	person *people.Person
	err    error
}

// Refresh brings the index up to the datastore. An entity that is new, or
// whose version moved, is loaded again; one that is gone is dropped. One
// that fails to load is kept as broken. The writes are one transaction.
func (ix *Index) Refresh(ctx context.Context) error {
	_, err := ix.refresh(ctx)
	return err
}

// refresh is Refresh, and the issues it folded and wrote, at their heads.
func (ix *Index) refresh(ctx context.Context) (map[string]State, error) {
	refs := map[key]string{}
	for _, kind := range kinds {
		m, err := ix.s.Heads(ctx, kind)
		if err != nil {
			return nil, fmt.Errorf("index: heads: %w", err)
		}
		for id, v := range m {
			if oplog.IsID(id) {
				refs[key{kind, id}] = v
			}
		}
	}
	stored, err := ix.heads(ctx)
	if err != nil {
		return nil, err
	}
	var gone []key
	for k := range stored {
		if _, ok := refs[k]; !ok {
			gone = append(gone, k)
		}
	}
	// Loading happens before the write lock is taken, so a long rebuild
	// doesn't keep another process waiting.
	var todo []loaded
	for k, head := range refs {
		if stored[k] != head {
			todo = append(todo, loaded{key: k, head: head})
		}
	}
	ix.load(ctx, todo)
	if err := ctx.Err(); err != nil { // a load cut short isn't a broken entity
		return nil, err
	}
	var changed []loaded
	for _, l := range todo {
		if errors.Is(l.err, oplog.ErrNotFound) { // its ref went since the listing
			gone = append(gone, l.key)
			continue
		}
		changed = append(changed, l)
	}
	if len(gone)+len(changed) == 0 {
		return nil, nil
	}
	wrote := map[string]State{}
	if err := ix.write(ctx, stored, gone, changed, wrote); err != nil {
		return nil, fmt.Errorf("index: refresh: %w", err)
	}
	return wrote, nil
}

// loaders are how many entities a refresh loads at once: a fold is CPU,
// a read from PostgreSQL is a round trip.
var loaders = max(runtime.GOMAXPROCS(0), 4)

// load loads each entity at its head, several at once.
func (ix *Index) load(ctx context.Context, todo []loaded) {
	next := make(chan *loaded)
	var wg sync.WaitGroup
	for range min(loaders, len(todo)) {
		wg.Go(func() {
			for l := range next {
				switch {
				case ctx.Err() != nil: // cut short: Refresh returns
				case l.kind == "issue":
					l.issue, l.err = issue.LoadAt(ctx, ix.s, l.id, l.head)
				default:
					l.person, l.err = people.LoadAt(ctx, ix.s, l.id, l.head)
				}
			}
		})
	}
	for i := range todo {
		next <- &todo[i]
	}
	close(next)
	wg.Wait()
}

// heads are the stored heads, broken entities' included.
func (ix *Index) heads(ctx context.Context) (map[key]string, error) {
	rows, err := ix.db.QueryContext(ctx, "SELECT kind, id, head FROM entities")
	if err != nil {
		return nil, fmt.Errorf("index: heads: %w", err)
	}
	defer rows.Close()
	out := map[key]string{}
	for rows.Next() {
		var k key
		var head string
		if err := rows.Scan(&k.kind, &k.id, &head); err != nil {
			return nil, err
		}
		out[k] = head
	}
	return out, rows.Err()
}

// write drops the gone entities and replaces the changed ones' rows, in one
// transaction, and puts the issues it wrote in wrote. Another refresh may
// have written some since stored, the heads this one read: those are
// skipped, as that one's head may be newer than what this one loaded. A
// head left behind is loaded again by the next refresh.
func (ix *Index) write(ctx context.Context, stored map[key]string, gone []key, changed []loaded, wrote map[string]State) error {
	tx, err := ix.db.BeginTx(ctx, nil) // immediate (_txlock): the check and the write are one
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if ix.pg { // one refresh at a time, as sqlite's immediate transaction
		if _, err := tx.ExecContext(ctx, "LOCK TABLE entities IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			return err
		}
	}
	st := &stmts{ix: ix, tx: tx, ctx: ctx}
	exec := st.exec
	moved := func(k key) (bool, error) {
		var head string
		err := st.queryRow("SELECT head FROM entities WHERE kind = ? AND id = ?", k.kind, k.id).Scan(&head)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		return head != stored[k], err
	}
	drop := func(k key) error {
		var stmts []string
		if k.kind == "issue" {
			texts := "DELETE FROM texts WHERE rowid = (SELECT n FROM issues WHERE id = ?1)"
			if ix.pg {
				texts = "DELETE FROM texts WHERE n = (SELECT n FROM issues WHERE id = ?1)"
			}
			stmts = []string{
				texts,
				"DELETE FROM labels WHERE issue = ?1",
				"DELETE FROM issues WHERE id = ?1",
			}
		} else {
			stmts = []string{"DELETE FROM accounts WHERE person = ?1", "DELETE FROM persons WHERE id = ?1"}
		}
		for _, q := range stmts {
			if err := exec(q, k.id); err != nil {
				return err
			}
		}
		return exec("DELETE FROM entities WHERE kind = ? AND id = ?", k.kind, k.id)
	}
	for _, k := range gone {
		if m, err := moved(k); err != nil {
			return err
		} else if m {
			continue
		}
		if err := drop(k); err != nil {
			return err
		}
	}
	for _, l := range changed {
		if m, err := moved(l.key); err != nil {
			return err
		} else if m {
			continue
		}
		if _, ok := stored[l.key]; ok { // a new one has no rows yet
			if err := drop(l.key); err != nil {
				return err
			}
		}
		var msg any // NULL: it loaded
		switch {
		case l.err != nil:
			msg = l.err.Error()
		case l.issue != nil && l.issue.Deleted:
			// a deleted issue is indexed by its head alone: no query finds it
		case l.issue != nil:
			err = insertIssue(st, l.issue)
		default:
			err = insertPerson(st, l.person)
		}
		if err != nil {
			return err
		}
		if err := exec("INSERT INTO entities (kind, id, head, error) VALUES (?, ?, ?, ?)", l.kind, l.id, l.head, msg); err != nil {
			return err
		}
		if msg == nil && l.issue != nil && !l.issue.Deleted {
			wrote[l.id] = State{l.head, l.issue}
		}
	}
	return tx.Commit()
}

// stmts are a write's statements, each prepared once in its transaction:
// a rebuild runs each thousands of times.
type stmts struct {
	ix  *Index
	tx  *sql.Tx
	ctx context.Context
	m   map[string]*sql.Stmt
}

// stmt is query, for the index's server (Index.sql), prepared.
func (st *stmts) stmt(query string) (*sql.Stmt, error) {
	if s, ok := st.m[query]; ok {
		return s, nil
	}
	s, err := st.tx.PrepareContext(st.ctx, st.ix.sql(query))
	if err != nil {
		return nil, err
	}
	if st.m == nil {
		st.m = map[string]*sql.Stmt{}
	}
	st.m[query] = s
	return s, nil
}

func (st *stmts) exec(query string, args ...any) error {
	_, err := st.result(query, args...)
	return err
}

func (st *stmts) result(query string, args ...any) (sql.Result, error) {
	s, err := st.stmt(query)
	if err != nil {
		return nil, err
	}
	return s.ExecContext(st.ctx, args...)
}

// queryRow is QueryRowContext on query, prepared; a failed prepare is
// the row's error.
func (st *stmts) queryRow(query string, args ...any) *sql.Row {
	s, err := st.stmt(query)
	if err != nil {
		return st.tx.QueryRowContext(st.ctx, st.ix.sql(query), args...) // answers the same error
	}
	return s.QueryRowContext(st.ctx, args...)
}

func insertIssue(st *stmts, is *issue.Issue) error {
	ix := st.ix
	// Labels never hold a line feed (issue.Apply refuses one), so it joins them.
	state, err := json.Marshal(is)
	if err != nil {
		return err
	}
	insert := `INSERT INTO issues (id, title, status, author, created, updated, comments, labels, state)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	args := []any{is.ID, is.Title, string(is.Status), is.Author, is.Created, is.Updated, len(is.Comments),
		strings.Join(is.Labels, "\n"), string(state)}
	var n int64
	if ix.pg {
		if err := st.queryRow(insert+" RETURNING n", args...).Scan(&n); err != nil {
			return err
		}
	} else {
		res, err := st.result(insert, args...)
		if err != nil {
			return err
		}
		if n, err = res.LastInsertId(); err != nil {
			return err
		}
	}
	var comments []string
	for _, c := range is.Comments {
		comments = append(comments, c.Body)
	}
	// ponytail: the comments are one column, so a word of several tokens can
	// match across two comments; a row per text if that ever matters.
	texts := "INSERT INTO texts (rowid, title, body, comments) VALUES (?, ?, ?, ?)"
	if ix.pg {
		texts = `INSERT INTO texts (n, doc) VALUES (?, setweight(to_tsvector('simple', ?), 'A')
			|| setweight(to_tsvector('simple', ?), 'B') || setweight(to_tsvector('simple', ?), 'C'))`
	}
	args = []any{n, is.Title, is.Body, strings.Join(comments, "\n")}
	if ix.pg { // one text, tokenized here as sqlite's unicode61 does, each part weighed
		left := maxTokens
		for i, t := range []string{is.Title, is.Body, strings.Join(comments, "\n")} {
			toks := capTokens(tokens(t), left)
			left -= len(toks) + 1
			args[i+1] = toks
		}
	}
	if err := st.exec(texts, args...); err != nil {
		return err
	}
	for _, label := range is.Labels {
		if err := st.exec("INSERT INTO labels (issue, label) VALUES (?, ?)", is.ID, label); err != nil {
			return err
		}
	}
	return nil
}

// maxTokens bounds the text PostgreSQL indexes for one issue. A tsvector
// holds at most 1 MB, and takes at most about 2.5 times its text's bytes
// (each lexeme's entry and position), so one issue's text never fails the
// refresh, and with it every query (queries.md).
const maxTokens = 300_000

// capTokens is toks cut, at a token's end, to n bytes (maxTokens for one
// issue's texts together).
func capTokens(toks string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(toks) <= n {
		return toks
	}
	toks = toks[:n+1]
	if i := strings.LastIndexByte(toks, ' '); i >= 0 {
		return toks[:i]
	}
	return ""
}

func insertPerson(st *stmts, p *people.Person) error {
	if err := st.exec("INSERT INTO persons (id, name) VALUES (?, ?)", p.ID, p.Name); err != nil {
		return err
	}
	for _, a := range p.Accounts {
		if err := st.exec("INSERT INTO accounts (server, id, person, login) VALUES (?, ?, ?, ?)",
			a.Server, a.ID, p.ID, a.Login); err != nil {
			return err
		}
	}
	return nil
}

// Broken lists the entities that failed to load, by kind then id. It
// refreshes first.
func (ix *Index) Broken(ctx context.Context) ([]Broken, error) {
	if err := ix.Refresh(ctx); err != nil {
		return nil, err
	}
	rows, err := ix.db.QueryContext(ctx, "SELECT kind, id, error FROM entities WHERE error IS NOT NULL ORDER BY kind, id")
	if err != nil {
		return nil, fmt.Errorf("index: broken: %w", err)
	}
	defer rows.Close()
	var out []Broken
	for rows.Next() {
		var b Broken
		if err := rows.Scan(&b.Kind, &b.ID, &b.Error); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
