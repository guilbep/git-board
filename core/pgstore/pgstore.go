// Package pgstore is the PostgreSQL datastore (ADR 0002): one shared
// server keeps the board's packs, its counter and its index. Every writer
// writes there at once, so there is no replica to fetch, push or merge, and
// no merge pack: a version is a count of the entity's writes.
//
// Its tables are in the schema git_board:
//
//	entities (kind, id, version)       an entity and its version
//	packs    (kind, id, hash, data)    its packs, byte for byte
//	counter  (value, version)          the project numbers, one row
//
// and the index's (core/index) in git_board_index_v<its version>.
package pgstore

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/oplog"
)

// Schema is where the datastore's tables are. The index's are in a schema
// of their own, one per version (core/index.PGSchema).
const Schema = "git_board"

// indexSchema is a schema IndexDB makes: a lowercase SQL identifier, as
// it is written into the statement.
var indexSchema = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// version is the schema's, in git_board.meta.
const version = 1

const schema = `
CREATE SCHEMA IF NOT EXISTS git_board;
CREATE TABLE IF NOT EXISTS git_board.meta (key text PRIMARY KEY, value text NOT NULL);
CREATE TABLE IF NOT EXISTS git_board.entities (
	kind text NOT NULL, id text NOT NULL, version bigint NOT NULL,
	PRIMARY KEY (kind, id));
CREATE TABLE IF NOT EXISTS git_board.packs (
	kind text NOT NULL, id text NOT NULL, hash text NOT NULL, data bytea NOT NULL,
	PRIMARY KEY (kind, id, hash),
	FOREIGN KEY (kind, id) REFERENCES git_board.entities);
CREATE TABLE IF NOT EXISTS git_board.counter (
	one boolean PRIMARY KEY DEFAULT true CHECK (one), value bytea NOT NULL, version bigint NOT NULL);
`

// Store is a board kept in PostgreSQL.
type Store struct {
	db     *sql.DB
	cfg    *pgx.ConnConfig
	folder string
}

var (
	_ datastore.Datastore = (*Store)(nil)
	_ datastore.Counter   = (*Store)(nil)
)

// Open connects to the database dsn names (a postgres:// URL or key=value
// pairs, as libpq takes them) and makes its tables when they aren't there.
// folder is this machine's own folder for the board.
func Open(ctx context.Context, dsn, folder string) (*Store, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("pgstore: %w", err)
	}
	s := &Store{db: stdlib.OpenDB(*cfg), cfg: cfg, folder: folder}
	if err := s.setup(ctx); err != nil {
		s.db.Close()
		return nil, err
	}
	return s, nil
}

// setup makes the tables and checks their version, under a lock another
// git-board setting up waits for.
func (s *Store) setup(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("pgstore: connecting to %s: %w", s.Where(), err)
	}
	defer tx.Rollback()
	// an arbitrary key, git-board's: two setups don't make the tables at once
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(7340176)"); err != nil {
		return fmt.Errorf("pgstore: %w", err)
	}
	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("pgstore: making the tables: %w", err)
	}
	var v string
	err = tx.QueryRowContext(ctx, "SELECT value FROM git_board.meta WHERE key = 'version'").Scan(&v)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, "INSERT INTO git_board.meta (key, value) VALUES ('version', $1)", strconv.Itoa(version)); err != nil {
			return fmt.Errorf("pgstore: %w", err)
		}
	case err != nil:
		return fmt.Errorf("pgstore: %w", err)
	case v != strconv.Itoa(version):
		return fmt.Errorf("pgstore: the tables are version %s, this git-board reads %d", v, version)
	}
	return tx.Commit()
}

// Probe is what a connection test finds, writing nothing.
type Probe struct {
	Server   string `json:"server"`   // the server's version
	Where    string `json:"where"`    // user@host:port/database
	Tables   bool   `json:"tables"`   // git-board's tables are there
	Entities int    `json:"entities"` // how many entities they keep
}

// Try connects to dsn and says what is there, making nothing.
func Try(ctx context.Context, dsn string) (Probe, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return Probe{}, fmt.Errorf("pgstore: %w", err)
	}
	p := Probe{Where: fmt.Sprintf("%s@%s:%d/%s", cfg.User, cfg.Host, cfg.Port, cfg.Database)}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return p, fmt.Errorf("connecting to %s: %w", p.Where, err)
	}
	defer conn.Close(ctx)
	if err := conn.QueryRow(ctx, "SHOW server_version").Scan(&p.Server); err != nil {
		return p, err
	}
	var reg *string
	if err := conn.QueryRow(ctx, "SELECT to_regclass('git_board.entities')::text").Scan(&reg); err != nil {
		return p, err
	}
	if p.Tables = reg != nil; p.Tables {
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM git_board.entities").Scan(&p.Entities); err != nil {
			return p, err
		}
	}
	return p, nil
}

// Close closes the connections.
func (s *Store) Close() error { return s.db.Close() }

// Name is where the board is kept, for a user to read.
func (s *Store) Name() string { return "PostgreSQL" }

// Where is the server and database, without the password.
func (s *Store) Where() string {
	return fmt.Sprintf("%s@%s:%d/%s", s.cfg.User, s.cfg.Host, s.cfg.Port, s.cfg.Database)
}

// IndexDB is a connection pool whose tables are the index's: the same
// database, with schema first on its search path, made when it isn't
// there. The index (core/index) keeps its tables in it, so every machine
// reads one index, kept up to the datastore by whoever reads it.
func (s *Store) IndexDB(ctx context.Context, schema string) (*sql.DB, error) {
	if !indexSchema.MatchString(schema) || schema == Schema {
		return nil, fmt.Errorf("pgstore: %q can't be the index's schema", schema)
	}
	// under setup's lock: two CREATE SCHEMA IF NOT EXISTS at once fail one
	if err := s.locked(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema); err != nil {
		return nil, fmt.Errorf("pgstore: %w", err)
	}
	cfg := s.cfg.Copy()
	if cfg.RuntimeParams == nil {
		cfg.RuntimeParams = map[string]string{}
	}
	cfg.RuntimeParams["search_path"] = schema
	return stdlib.OpenDB(*cfg), nil
}

// locked runs statement in a transaction holding setup's lock.
func (s *Store) locked(ctx context.Context, statement string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(7340176)"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, statement); err != nil {
		return err
	}
	return tx.Commit()
}

// Heads are a kind's entities and their versions.
func (s *Store) Heads(ctx context.Context, kind string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, version FROM git_board.entities WHERE kind = $1", kind)
	if err != nil {
		return nil, fmt.Errorf("pgstore: heads: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id string
		var v int64
		if err := rows.Scan(&id, &v); err != nil {
			return nil, err
		}
		out[id] = strconv.FormatInt(v, 10)
	}
	return out, rows.Err()
}

// Packs are an entity's packs and its version, read in one statement: one
// snapshot.
func (s *Store) Packs(ctx context.Context, kind, id string) ([][]byte, string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.version, p.data FROM git_board.entities e
		JOIN git_board.packs p USING (kind, id) WHERE e.kind = $1 AND e.id = $2`, kind, id)
	if err != nil {
		return nil, "", fmt.Errorf("pgstore: packs: %w", err)
	}
	defer rows.Close()
	var packs [][]byte
	var v int64
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&v, &data); err != nil {
			return nil, "", err
		}
		packs = append(packs, data)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(packs) == 0 {
		return nil, "", fmt.Errorf("%w: %s %s", datastore.ErrNotFound, kind, id)
	}
	return packs, strconv.FormatInt(v, 10), nil
}

// Add stores a pack, in one transaction that moves the version only from
// prev. Two packs with the same bytes are one.
func (s *Store) Add(ctx context.Context, kind, id string, pack []byte, prev string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("pgstore: %w", err)
	}
	defer tx.Rollback()
	var v int64
	if prev == "" {
		err = tx.QueryRowContext(ctx, `INSERT INTO git_board.entities (kind, id, version) VALUES ($1, $2, 1)
			ON CONFLICT DO NOTHING RETURNING version`, kind, id).Scan(&v)
	} else {
		p, perr := strconv.ParseInt(prev, 10, 64)
		if perr != nil {
			return "", fmt.Errorf("%w: %s %s: version %q", datastore.ErrMoved, kind, id, prev)
		}
		err = tx.QueryRowContext(ctx, `UPDATE git_board.entities SET version = version + 1
			WHERE kind = $1 AND id = $2 AND version = $3 RETURNING version`, kind, id, p).Scan(&v)
	}
	if errors.Is(err, sql.ErrNoRows) || serialization(err) {
		return "", fmt.Errorf("%w: %s %s", datastore.ErrMoved, kind, id)
	}
	if err != nil {
		return "", fmt.Errorf("pgstore: add: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO git_board.packs (kind, id, hash, data) VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING`, kind, id, oplog.Hash(pack), pack); err != nil {
		return "", fmt.Errorf("pgstore: add: %w", err)
	}
	if err := tx.Commit(); err != nil {
		if serialization(err) {
			return "", fmt.Errorf("%w: %s %s", datastore.ErrMoved, kind, id)
		}
		return "", fmt.Errorf("pgstore: add: %w", err)
	}
	return strconv.FormatInt(v, 10), nil
}

// serialization says the server refused a write that raced another.
func serialization(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && (pe.Code == "40001" || pe.Code == "40P01")
}

// Folder is this machine's folder.
func (s *Store) Folder() string { return s.folder }

// Count reads the counter: nil when none was started.
func (s *Store) Count(ctx context.Context) ([]byte, string, error) {
	var value []byte
	var v int64
	err := s.db.QueryRowContext(ctx, "SELECT value, version FROM git_board.counter").Scan(&value, &v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("pgstore: counter: %w", err)
	}
	return value, strconv.FormatInt(v, 10), nil
}

// SetCount writes the counter if it is still at prev.
func (s *Store) SetCount(ctx context.Context, value []byte, prev string) (string, error) {
	var v int64
	var err error
	if prev == "" {
		err = s.db.QueryRowContext(ctx, `INSERT INTO git_board.counter (value, version) VALUES ($1, 1)
			ON CONFLICT DO NOTHING RETURNING version`, value).Scan(&v)
	} else {
		p, perr := strconv.ParseInt(prev, 10, 64)
		if perr != nil {
			return "", fmt.Errorf("%w: the counter: version %q", datastore.ErrMoved, prev)
		}
		err = s.db.QueryRowContext(ctx, `UPDATE git_board.counter SET value = $1, version = version + 1
			WHERE version = $2 RETURNING version`, value, p).Scan(&v)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: the counter", datastore.ErrMoved)
	}
	if err != nil {
		return "", fmt.Errorf("pgstore: counter: %w", err)
	}
	return strconv.FormatInt(v, 10), nil
}

// Copy copies every entity of from into this, as datastore.Copy does: the
// packs this hasn't, a whole entity or the ones written in from since a
// copy, each entity in one transaction. It copies from's counter when it
// keeps one and this has none: a board leaving git keeps its ids, as its
// packs copy as they are. It answers how many entities it wrote to.
func (s *Store) Copy(ctx context.Context, from datastore.Datastore) (int, error) {
	n := 0
	for _, kind := range datastore.Kinds {
		theirs, err := from.Heads(ctx, kind)
		if err != nil {
			return n, err
		}
		ours, err := s.Heads(ctx, kind)
		if err != nil {
			return n, err
		}
		for id := range theirs {
			packs, _, err := from.Packs(ctx, kind, id)
			if err != nil {
				return n, fmt.Errorf("pgstore: copying %s %s: %w", kind, id, err)
			}
			prev := ""
			if _, ok := ours[id]; ok {
				mine, v, err := s.Packs(ctx, kind, id)
				if err != nil {
					return n, fmt.Errorf("pgstore: copying %s %s: %w", kind, id, err)
				}
				have := map[string]bool{}
				for _, p := range mine {
					have[string(p)] = true
				}
				packs = slices.DeleteFunc(packs, func(p []byte) bool { return have[string(p)] })
				if len(packs) == 0 {
					continue
				}
				prev = v
			}
			if err := s.put(ctx, kind, id, packs, prev); err != nil {
				return n, err
			}
			n++
		}
	}
	if c, ok := from.(datastore.Counter); ok {
		value, _, err := c.Count(ctx)
		if err != nil {
			return n, err
		}
		if mine, _, err := s.Count(ctx); err != nil {
			return n, err
		} else if value != nil && mine == nil {
			if _, err := s.SetCount(ctx, value, ""); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

// put writes packs on an entity whose version is still prev ("" for a new
// one), in one transaction.
func (s *Store) put(ctx context.Context, kind, id string, packs [][]byte, prev string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if prev == "" {
		if _, err := tx.ExecContext(ctx, "INSERT INTO git_board.entities (kind, id, version) VALUES ($1, $2, 1)", kind, id); err != nil {
			return fmt.Errorf("pgstore: copying %s %s: %w", kind, id, err)
		}
	} else {
		res, err := tx.ExecContext(ctx, `UPDATE git_board.entities SET version = version + 1
			WHERE kind = $1 AND id = $2 AND version::text = $3`, kind, id, prev)
		if err != nil {
			return fmt.Errorf("pgstore: copying %s %s: %w", kind, id, err)
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			return fmt.Errorf("pgstore: copying %s %s: %w", kind, id, cmp.Or(err, datastore.ErrMoved))
		}
	}
	for _, p := range packs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO git_board.packs (kind, id, hash, data) VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING`, kind, id, oplog.Hash(p), p); err != nil {
			return fmt.Errorf("pgstore: copying %s %s: %w", kind, id, err)
		}
	}
	return tx.Commit()
}
