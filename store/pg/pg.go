// Package pg is the PostgreSQL datastore (docs/spec/datastores.md,
// PostgreSQL): one database is one board, which every machine reads and
// writes at once. There is no replica to fetch, push or merge, and no
// merge pack: a version is the count of an entity's writes.
//
// Its tables are in the schema git_board, the contract v1 wrote:
//
//	meta     (key, value)               ('version', '1')
//	entities (kind, id, version)        an entity and its version
//	packs    (kind, id, hash, data)     its packs, byte for byte
//	counter  (one, value, version)      the project numbers, one row
//
// The view, disposable, is in a schema of its own (view).
package pg

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/guilbep/git-board/kernel/pack"
	"github.com/guilbep/git-board/store"
)

// Schema is the board's tables' schema.
const Schema = "git_board"

// version is the tables', in git_board.meta.
const version = 1

// setupLock is the advisory lock two first uses take, so they don't make
// the tables at once: v1's key.
const setupLock = 7340176

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
	pool   *pgxpool.Pool
	dsn    string
	where  string
	folder string
}

var (
	_ store.Datastore = (*Store)(nil)
	_ store.Counter   = (*Store)(nil)
	_ store.Locker    = (*Store)(nil)
)

// IsURL says a board.datastore value names PostgreSQL.
func IsURL(v string) bool {
	return strings.HasPrefix(v, "postgres://") || strings.HasPrefix(v, "postgresql://")
}

// Open connects to the database dsn names (a postgres:// URL or libpq's
// key=value pairs) and makes its tables when they aren't there. folder is
// this machine's own folder for the board.
func Open(ctx context.Context, dsn, folder string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("pg: %s", Redact(err.Error()))
	}
	c := cfg.ConnConfig
	s := &Store{dsn: dsn, folder: folder, where: fmt.Sprintf("%s@%s:%d/%s", c.User, c.Host, c.Port, c.Database)}
	if s.pool, err = pgxpool.NewWithConfig(ctx, cfg); err != nil {
		return nil, fmt.Errorf("pg: %w", err)
	}
	if err := s.setup(ctx); err != nil {
		s.pool.Close()
		return nil, err
	}
	return s, nil
}

// setup makes the tables and checks their version. A board that has them
// reads the version alone; else, under the lock another first use waits
// for, they are made.
func (s *Store) setup(ctx context.Context) error {
	var v string
	err := s.pool.QueryRow(ctx, "SELECT value FROM git_board.meta WHERE key = 'version'").Scan(&v)
	if err == nil {
		return checkVersion(v)
	}
	if pe := (*pgconn.PgError)(nil); !errors.As(err, &pe) && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("pg: connecting to %s: %w", s.where, err)
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", setupLock); err != nil {
			return fmt.Errorf("pg: %w", err)
		}
		if _, err := tx.Exec(ctx, schema); err != nil {
			return fmt.Errorf("pg: making the tables: %w", err)
		}
		err := tx.QueryRow(ctx, "SELECT value FROM git_board.meta WHERE key = 'version'").Scan(&v)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			_, err = tx.Exec(ctx, "INSERT INTO git_board.meta (key, value) VALUES ('version', $1)", strconv.Itoa(version))
			return err
		case err != nil:
			return fmt.Errorf("pg: %w", err)
		}
		return checkVersion(v)
	})
}

func checkVersion(v string) error {
	if v != strconv.Itoa(version) {
		return fmt.Errorf("pg: the tables are version %s, this git-board reads %d", v, version)
	}
	return nil
}

// Caps: shared by every machine; the counter, the lock and the view's
// database are the server's.
func (s *Store) Caps() store.Caps {
	return store.Caps{Shared: true, Counter: s, Locker: s, SQL: s, Put: s}
}

// DSN is the database's connection string, for the view.
func (s *Store) DSN() string { return s.dsn }

// Where is the server and the database, without the password.
func (s *Store) Where() string { return s.where }

// Folder is this machine's own folder.
func (s *Store) Folder() string { return s.folder }

// Close closes the connections.
func (s *Store) Close() error { s.pool.Close(); return nil }

// Heads lists the kinds' entities, in one statement, sorted by kind then
// id as Go compares them, whatever the database's collation.
func (s *Store) Heads(ctx context.Context, kinds ...string) ([]store.Head, error) {
	rows, err := s.pool.Query(ctx, "SELECT kind, id, version FROM git_board.entities WHERE kind = ANY($1)", kinds)
	if err != nil {
		return nil, fmt.Errorf("pg: heads: %w", err)
	}
	var out []store.Head
	var h store.Head
	var v int64
	_, err = pgx.ForEachRow(rows, []any{&h.Kind, &h.ID, &v}, func() error {
		if store.IsID(h.ID) {
			out = append(out, store.Head{Kind: h.Kind, ID: h.ID, Version: strconv.FormatInt(v, 10)})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("pg: heads: %w", err)
	}
	slices.SortFunc(out, func(a, b store.Head) int { return cmp.Or(strings.Compare(a.Kind, b.Kind), strings.Compare(a.ID, b.ID)) })
	return out, nil
}

// wholeKind is how many refs of a kind make a load read the kind whole:
// one scan of its packs, not an index lookup a ref.
const wholeKind = 512

// Load reads the refs' packs, each entity's with its version in one
// statement, so one snapshot: the current ones, whatever version a ref
// names, as a count of writes names no packs for good. Many refs of a
// kind read the kind whole.
func (s *Store) Load(ctx context.Context, refs []store.Ref, each func(store.Ref, [][]byte, error) error) error {
	byKind := map[string][]string{}
	var kinds []string
	for _, r := range refs {
		if _, ok := byKind[r.Kind]; !ok {
			kinds = append(kinds, r.Kind)
		}
		byKind[r.Kind] = append(byKind[r.Kind], r.ID)
	}
	for _, kind := range kinds {
		ids := byKind[kind]
		want := make(map[string]bool, len(ids))
		for _, id := range ids {
			want[id] = true
		}
		q, args := `SELECT e.id, e.version, p.data FROM git_board.entities e JOIN git_board.packs p USING (kind, id)
			WHERE e.kind = $1 AND e.id = ANY($2) ORDER BY e.id`, []any{kind, ids}
		if len(ids) >= wholeKind {
			q, args = `SELECT e.id, e.version, p.data FROM git_board.entities e JOIN git_board.packs p USING (kind, id)
				WHERE e.kind = $1 ORDER BY e.id`, []any{kind}
		}
		rows, err := s.pool.Query(ctx, q, args...)
		if err != nil {
			return fmt.Errorf("pg: load: %w", err)
		}
		var cur string
		var v int64
		var packs [][]byte
		flush := func() error {
			if cur == "" || !want[cur] {
				return nil
			}
			delete(want, cur)
			return each(store.Ref{Kind: kind, ID: cur, Version: strconv.FormatInt(v, 10)}, packs, nil)
		}
		var id string
		var ver int64
		var data []byte
		_, err = pgx.ForEachRow(rows, []any{&id, &ver, &data}, func() error {
			if id != cur {
				if err := flush(); err != nil {
					return err
				}
				cur, v, packs = id, ver, nil
			}
			packs = append(packs, data) // pgx gives each row its own bytes
			return nil
		})
		if err == nil {
			err = flush()
		}
		if err != nil {
			return fmt.Errorf("pg: load: %w", err)
		}
		for _, id := range ids { // in the order asked: the ones no row answered
			if want[id] {
				delete(want, id)
				if err := each(store.Ref{Kind: kind, ID: id}, nil, fmt.Errorf("%w: %s %s", store.ErrNotFound, kind, id)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Add stores a pack in one transaction that moves the version only from
// prev ("" for a new entity). Its clock must be one more than the highest
// of the entity's packs.
func (s *Store) Add(ctx context.Context, kind, id string, data []byte, prev string) (string, error) {
	if !store.IsID(id) || kind == "" || strings.ContainsAny(kind, "/.") {
		return "", fmt.Errorf("pg: bad name %s %s", kind, id)
	}
	clock, empty, err := pack.Header(data)
	if err != nil {
		return "", fmt.Errorf("pg: %s %s: pack: %w", kind, id, err)
	}
	if empty {
		return "", fmt.Errorf("pg: %s %s: %w", kind, id, pack.ErrEmpty)
	}
	return s.put(ctx, kind, id, [][]byte{data}, prev, func(tx pgx.Tx) error {
		want := uint64(1)
		if prev != "" {
			h, err := s.highest(ctx, tx, kind, id)
			if err != nil {
				return err
			}
			want = h + 1
		}
		if clock != want {
			return fmt.Errorf("pg: %s %s: a pack of clock %d on one of clock %d", kind, id, clock, want-1)
		}
		return nil
	})
}

// highest is the highest clock of an entity's packs, read in tx.
func (s *Store) highest(ctx context.Context, tx pgx.Tx, kind, id string) (uint64, error) {
	rows, err := tx.Query(ctx, "SELECT data FROM git_board.packs WHERE kind = $1 AND id = $2", kind, id)
	if err != nil {
		return 0, err
	}
	var h uint64
	var data []byte
	_, err = pgx.ForEachRow(rows, []any{&data}, func() error {
		if c, _, err := pack.Header(data); err == nil {
			h = max(h, c)
		}
		return nil
	})
	return h, err
}

// Put stores packs as they are, in one transaction that moves the version
// by one from prev: datastore copy's. It answers prev when the entity
// holds them all already.
func (s *Store) Put(ctx context.Context, kind, id string, packs [][]byte, prev string) (string, error) {
	if !store.IsID(id) || kind == "" || strings.ContainsAny(kind, "/.") {
		return "", fmt.Errorf("pg: bad name %s %s", kind, id)
	}
	if len(packs) == 0 {
		return prev, nil
	}
	return s.put(ctx, kind, id, packs, prev, nil)
}

// put moves an entity's version from prev, checks, and writes the packs,
// in one transaction (datastores.md, Writing).
func (s *Store) put(ctx context.Context, kind, id string, packs [][]byte, prev string, check func(pgx.Tx) error) (string, error) {
	var v int64
	moved := fmt.Errorf("%w: %s %s", store.ErrMoved, kind, id)
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		if prev == "" {
			err = tx.QueryRow(ctx, `INSERT INTO git_board.entities (kind, id, version) VALUES ($1, $2, 1)
				ON CONFLICT DO NOTHING RETURNING version`, kind, id).Scan(&v)
		} else {
			p, perr := strconv.ParseInt(prev, 10, 64)
			if perr != nil {
				return moved
			}
			err = tx.QueryRow(ctx, `UPDATE git_board.entities SET version = version + 1
				WHERE kind = $1 AND id = $2 AND version = $3 RETURNING version`, kind, id, p).Scan(&v)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return moved
		}
		if err != nil {
			return err
		}
		if check != nil {
			if err := check(tx); err != nil {
				return err
			}
		}
		b := &pgx.Batch{}
		for _, p := range packs {
			b.Queue(`INSERT INTO git_board.packs (kind, id, hash, data) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
				kind, id, pack.Hash(p), p)
		}
		return tx.SendBatch(ctx, b).Close()
	})
	switch {
	case errors.Is(err, store.ErrMoved) || serialization(err):
		return "", moved
	case err != nil && strings.HasPrefix(err.Error(), "pg: "):
		return "", err
	case err != nil:
		return "", fmt.Errorf("pg: add %s %s: %w", kind, id, err)
	}
	return strconv.FormatInt(v, 10), nil
}

// serialization says the server refused a write that raced another.
func serialization(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && (pe.Code == "40001" || pe.Code == "40P01")
}

// Lock takes the board's lock named name: a session advisory lock, on a
// connection held until unlock. Its key is the FNV-1a hash of
// git-board/<name>. The server lets it go when the session ends, so a
// process killed holding it keeps no one out. A lock held is refused at
// once: ErrHeld.
func (s *Store) Lock(ctx context.Context, name string) (func(), error) {
	h := fnv.New64a()
	h.Write([]byte("git-board/" + name))
	key := int64(h.Sum64())
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("pg: lock %s: %w", name, err)
	}
	var got bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&got); err != nil {
		conn.Release()
		return nil, fmt.Errorf("pg: lock %s: %w", name, err)
	}
	if !got {
		conn.Release()
		return nil, fmt.Errorf("%w: %s, on %s", store.ErrHeld, name, s.where)
	}
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var freed bool
		if err := conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", key).Scan(&freed); err != nil || !freed {
			// the session may still hold it: closed, not given back
			conn.Hijack().Close(ctx)
			return
		}
		conn.Release()
	}, nil
}

// Count reads the counter: nil when none was started.
func (s *Store) Count(ctx context.Context) ([]byte, string, error) {
	var value []byte
	var v int64
	err := s.pool.QueryRow(ctx, "SELECT value, version FROM git_board.counter").Scan(&value, &v)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("pg: counter: %w", err)
	}
	return value, strconv.FormatInt(v, 10), nil
}

// SetCount writes the counter if it is still at prev.
func (s *Store) SetCount(ctx context.Context, value []byte, prev string) (string, error) {
	var v int64
	var err error
	if prev == "" {
		err = s.pool.QueryRow(ctx, `INSERT INTO git_board.counter (value, version) VALUES ($1, 1)
			ON CONFLICT DO NOTHING RETURNING version`, value).Scan(&v)
	} else {
		p, perr := strconv.ParseInt(prev, 10, 64)
		if perr != nil {
			return "", fmt.Errorf("%w: the counter: version %q", store.ErrMoved, prev)
		}
		err = s.pool.QueryRow(ctx, `UPDATE git_board.counter SET value = $1, version = version + 1
			WHERE version = $2 RETURNING version`, value, p).Scan(&v)
	}
	if errors.Is(err, pgx.ErrNoRows) || serialization(err) {
		return "", fmt.Errorf("%w: the counter", store.ErrMoved)
	}
	if err != nil {
		return "", fmt.Errorf("pg: counter: %w", err)
	}
	return strconv.FormatInt(v, 10), nil
}

// Redact is s with a postgres:// URL's password, or a password= pair's,
// replaced by xxxxx: what an error or a page may show.
func Redact(s string) string {
	for _, scheme := range []string{"postgres://", "postgresql://"} {
		for i := 0; ; {
			j := strings.Index(s[i:], scheme)
			if j < 0 {
				break
			}
			start := i + j + len(scheme)
			end := start + strings.IndexAny(s[start:]+" ", " /?#")
			if at := strings.LastIndexByte(s[start:end], '@'); at >= 0 {
				if colon := strings.IndexByte(s[start:start+at], ':'); colon >= 0 {
					s = s[:start+colon+1] + "xxxxx" + s[start+at:]
				}
			}
			i = start
		}
	}
	if i := strings.Index(s, "password="); i >= 0 {
		end := i + len("password=") + strings.IndexAny(s[i+len("password="):]+" ", " ")
		s = s[:i] + "password=xxxxx" + s[end:]
	}
	return s
}
