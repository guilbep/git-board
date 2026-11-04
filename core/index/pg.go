package index

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/core/oplog"
)

// pgSchema is the index's tables on PostgreSQL, in the schema the pool's
// search path names. texts holds each issue's words as a tsvector, by its
// issue's n.
const pgSchema = `
CREATE TABLE IF NOT EXISTS meta (key text PRIMARY KEY, value text NOT NULL);
CREATE TABLE IF NOT EXISTS entities (kind text NOT NULL, id text NOT NULL, head text NOT NULL, error text,
	PRIMARY KEY (kind, id));
CREATE TABLE IF NOT EXISTS issues (n bigserial PRIMARY KEY, id text NOT NULL UNIQUE, title text NOT NULL,
	status text NOT NULL, author text NOT NULL, created bigint NOT NULL, updated bigint NOT NULL,
	comments integer NOT NULL, labels text NOT NULL);
CREATE TABLE IF NOT EXISTS labels (issue text NOT NULL, label text NOT NULL, PRIMARY KEY (label, issue));
CREATE TABLE IF NOT EXISTS persons (id text PRIMARY KEY, name text NOT NULL);
CREATE TABLE IF NOT EXISTS accounts (server text NOT NULL, id text NOT NULL, person text NOT NULL, login text NOT NULL,
	PRIMARY KEY (server, id, person));
CREATE INDEX IF NOT EXISTS accounts_person ON accounts (person);
CREATE TABLE IF NOT EXISTS texts (n bigint PRIMARY KEY, doc tsvector NOT NULL);
CREATE INDEX IF NOT EXISTS texts_doc ON texts USING gin (doc);
`

// pgTables are the index's tables, dropped when their version isn't this
// one: the index is never the source of truth, so it is built again.
const pgTables = "meta, entities, issues, labels, persons, accounts, texts"

// openPG opens the index on the datastore's server.
func openPG(ctx context.Context, s oplog.Datastore, q SQL) (*Index, error) {
	db, err := q.IndexDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("index: %w", err)
	}
	if err := setupPG(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("index: %w", err)
	}
	return &Index{s: s, db: db, pg: true}, nil
}

// setupPG makes the tables, or makes them again when they are of another
// version, under a lock another git-board setting up waits for.
func setupPG(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// an arbitrary key, the index's: two setups don't make the tables at once
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(7340177)"); err != nil {
		return err
	}
	var v string
	err = tx.QueryRowContext(ctx, "SELECT value FROM meta WHERE key = 'version'").Scan(&v)
	switch {
	case err == nil && v == strconv.Itoa(version):
		return tx.Commit()
	case err == nil: // another version: build again
		if _, err := tx.ExecContext(ctx, "DROP TABLE IF EXISTS "+pgTables); err != nil {
			return err
		}
	default: // no meta yet; its error aborted the transaction
		tx.Rollback()
		if tx, err = db.BeginTx(ctx, nil); err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(7340177)"); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, pgSchema); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO meta (key, value) VALUES ('version', $1)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, strconv.Itoa(version)); err != nil {
		return err
	}
	return tx.Commit()
}

// sql is a query for the index's server: sqlite's as written, or with
// PostgreSQL's numbered placeholders, ?N as $N and each bare ? the next.
func (ix *Index) sql(query string) string {
	if !ix.pg {
		return query
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(query); i++ {
		c := query[i]
		if c != '?' {
			b.WriteByte(c)
			continue
		}
		j := i + 1
		for j < len(query) && query[j] >= '0' && query[j] <= '9' {
			j++
		}
		if j > i+1 {
			b.WriteString("$" + query[i+1:j])
			i = j - 1
			continue
		}
		n++
		b.WriteString("$" + strconv.Itoa(n))
	}
	return b.String()
}
