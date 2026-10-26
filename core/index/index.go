// Package index keeps a local sqlite index of the issues and persons, for
// lists, search and lookups by account. It is never the source of truth:
// every query first brings it up to the refs.
package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
)

// version is the schema's, in PRAGMA user_version.
const version = 1

// schema is the index's tables. entities holds every indexed entity's head;
// error is NULL when it loaded, and its error when it is broken. texts'
// rowid is its issue's n.
const schema = `
CREATE TABLE entities (kind TEXT NOT NULL, id TEXT NOT NULL, head TEXT NOT NULL, error TEXT,
	PRIMARY KEY (kind, id));
CREATE TABLE issues (n INTEGER PRIMARY KEY, id TEXT NOT NULL UNIQUE, title TEXT NOT NULL,
	status TEXT NOT NULL, author TEXT NOT NULL, created INTEGER NOT NULL, updated INTEGER NOT NULL,
	comments INTEGER NOT NULL, labels TEXT NOT NULL);
CREATE TABLE labels (issue TEXT NOT NULL, label TEXT NOT NULL, PRIMARY KEY (label, issue));
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

// Index is the sqlite index of one repository.
type Index struct {
	s  *gitstore.Store
	db *sql.DB
}

// Broken is an entity that failed to load.
type Broken struct{ Kind, ID, Error string }

// Open opens the index at <common dir>/board/index.db. A file that is
// missing, that sqlite can't read, or of another version is built again.
func Open(ctx context.Context, s *gitstore.Store) (*Index, error) {
	dir := filepath.Join(s.CommonDir(), "board")
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

// Refresh brings the index up to the refs. An entity that is new, or whose
// head moved, is loaded again; one whose ref is gone is dropped. One that
// fails to load is kept as broken. The writes are one transaction.
func (ix *Index) Refresh(ctx context.Context) error {
	refs := map[key]string{}
	for _, kind := range kinds {
		prefix := oplog.RefName(kind, "")
		m, err := ix.s.Refs(prefix)
		if err != nil {
			return fmt.Errorf("index: refs: %w", err)
		}
		for name, h := range m {
			if id := strings.TrimPrefix(name, prefix); oplog.IsID(id) {
				refs[key{kind, id}] = h.String()
			}
		}
	}
	stored, err := ix.heads(ctx)
	if err != nil {
		return err
	}
	var gone []key
	for k := range stored {
		if _, ok := refs[k]; !ok {
			gone = append(gone, k)
		}
	}
	// Loading happens before the write lock is taken, so a long rebuild
	// doesn't keep another process waiting.
	var changed []loaded
	for k, head := range refs {
		if stored[k] == head {
			continue
		}
		l := loaded{key: k, head: head}
		if k.kind == "issue" {
			l.issue, l.err = issue.Load(ix.s, k.id)
		} else {
			l.person, l.err = people.Load(ix.s, k.id)
		}
		if errors.Is(l.err, oplog.ErrNotFound) { // its ref went since the listing
			gone = append(gone, k)
			continue
		}
		changed = append(changed, l)
	}
	if len(gone)+len(changed) == 0 {
		return nil
	}
	if err := ix.write(ctx, gone, changed); err != nil {
		return fmt.Errorf("index: refresh: %w", err)
	}
	return nil
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
// transaction. Another refresh may have written some already: each entity's
// rows are deleted before they are written, so the last write wins, and a
// head it left behind is loaded again by the next refresh.
func (ix *Index) write(ctx context.Context, gone []key, changed []loaded) error {
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	exec := func(query string, args ...any) error {
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	}
	drop := func(k key) error {
		var stmts []string
		if k.kind == "issue" {
			stmts = []string{
				"DELETE FROM texts WHERE rowid = (SELECT n FROM issues WHERE id = ?1)",
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
		if err := drop(k); err != nil {
			return err
		}
	}
	for _, l := range changed {
		if err := drop(l.key); err != nil {
			return err
		}
		var msg any // NULL: it loaded
		switch {
		case l.err != nil:
			msg = l.err.Error()
		case l.issue != nil:
			err = insertIssue(ctx, tx, l.issue)
		default:
			err = insertPerson(ctx, tx, l.person)
		}
		if err != nil {
			return err
		}
		if err := exec("INSERT INTO entities (kind, id, head, error) VALUES (?, ?, ?, ?)", l.kind, l.id, l.head, msg); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertIssue(ctx context.Context, tx *sql.Tx, is *issue.Issue) error {
	// Labels never hold a line feed (issue.Apply refuses one), so it joins them.
	res, err := tx.ExecContext(ctx, `INSERT INTO issues (id, title, status, author, created, updated, comments, labels)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		is.ID, is.Title, string(is.Status), is.Author, is.Created, is.Updated, len(is.Comments), strings.Join(is.Labels, "\n"))
	if err != nil {
		return err
	}
	n, err := res.LastInsertId()
	if err != nil {
		return err
	}
	var comments []string
	for _, c := range is.Comments {
		comments = append(comments, c.Body)
	}
	// ponytail: the comments are one column, so a word of several tokens can
	// match across two comments; a row per text if that ever matters.
	if _, err := tx.ExecContext(ctx, "INSERT INTO texts (rowid, title, body, comments) VALUES (?, ?, ?, ?)",
		n, is.Title, is.Body, strings.Join(comments, "\n")); err != nil {
		return err
	}
	for _, label := range is.Labels {
		if _, err := tx.ExecContext(ctx, "INSERT INTO labels (issue, label) VALUES (?, ?)", is.ID, label); err != nil {
			return err
		}
	}
	return nil
}

func insertPerson(ctx context.Context, tx *sql.Tx, p *people.Person) error {
	if _, err := tx.ExecContext(ctx, "INSERT INTO persons (id, name) VALUES (?, ?)", p.ID, p.Name); err != nil {
		return err
	}
	for _, a := range p.Accounts {
		if _, err := tx.ExecContext(ctx, "INSERT INTO accounts (server, id, person, login) VALUES (?, ?, ?, ?)",
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
