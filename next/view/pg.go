package view

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/guilbep/git-board/next/kernel"
	"github.com/guilbep/git-board/next/kernel/search"
)

// PGSchema is the view's schema in a board's PostgreSQL database, beside
// the board's git_board (datastores.md: disposable, no contract). A view
// of another shape is another schema.
const PGSchema = "git_board_view_v1"

const pgSchema = `
CREATE SCHEMA IF NOT EXISTS git_board_view_v1;
CREATE TABLE IF NOT EXISTS git_board_view_v1.marks (kind text PRIMARY KEY, rev integer NOT NULL, sum bytea NOT NULL, search boolean NOT NULL);
CREATE TABLE IF NOT EXISTS git_board_view_v1.rows (kind text NOT NULL, id text NOT NULL, version text NOT NULL,
	created bigint NOT NULL, updated bigint NOT NULL, hidden boolean NOT NULL, search boolean NOT NULL,
	row bytea NOT NULL, PRIMARY KEY (kind, id));
CREATE INDEX IF NOT EXISTS rows_unsearchable ON git_board_view_v1.rows (kind) WHERE NOT search;
CREATE TABLE IF NOT EXISTS git_board_view_v1.terms (kind text NOT NULL, term text NOT NULL, id text NOT NULL, PRIMARY KEY (kind, term, id));
CREATE INDEX IF NOT EXISTS terms_id ON git_board_view_v1.terms (kind, id);
CREATE TABLE IF NOT EXISTS git_board_view_v1.texts (kind text NOT NULL, id text NOT NULL, field text NOT NULL, toks text NOT NULL,
	PRIMARY KEY (kind, id, field));
CREATE TABLE IF NOT EXISTS git_board_view_v1.fields (kind text NOT NULL, field text NOT NULL, PRIMARY KEY (kind, field));
CREATE TABLE IF NOT EXISTS git_board_view_v1.broken (kind text NOT NULL, id text NOT NULL, version text NOT NULL, error text NOT NULL,
	PRIMARY KEY (kind, id));
`

// PG is a board's view in its PostgreSQL database: every machine reads
// one view, kept up to the board by whoever reads it. Its rows are also
// kept in files in this machine's own folder, read whole by a list, as
// the sqlite view's are.
//
// Free text is matched as the sqlite view's FTS5 matches it
// (queries.md, Text): each text is kept as its tokens (search.Tokens),
// space separated, and a word's tokens are a LIKE pattern on them, in a
// row, the last a prefix.
type PG struct {
	pool  *pgxpool.Pool
	files *rowFiles
}

var _ kernel.View = (*PG)(nil)

// OpenPG opens the view in the database dsn names, making its tables when
// they aren't there; dir is this machine's folder for its rows files.
func OpenPG(ctx context.Context, dsn, dir string) (*PG, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	v := &PG{pool: pool, files: newRowFiles(dir)}
	var there bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('git_board_view_v1.broken') IS NOT NULL").Scan(&there); err != nil {
		pool.Close()
		return nil, err
	}
	if !there {
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7340176)"); err != nil { // the board's setup lock
				return err
			}
			_, err := tx.Exec(ctx, pgSchema)
			return err
		})
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("view: making its tables: %w", err)
		}
	}
	return v, nil
}

// Close closes the connections.
func (v *PG) Close() error { v.pool.Close(); return nil }

type pgQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Mark is what the view holds of a kind; a kind it never held has rev 0.
func (v *PG) Mark(ctx context.Context, kind string) (kernel.Mark, error) { return pgMark(ctx, v.pool, kind) }

func pgMark(ctx context.Context, q pgQuerier, kind string) (kernel.Mark, error) {
	var m kernel.Mark
	var sum []byte
	err := q.QueryRow(ctx, "SELECT rev, sum, search FROM git_board_view_v1.marks WHERE kind = $1", kind).Scan(&m.Rev, &sum, &m.Search)
	if errors.Is(err, pgx.ErrNoRows) {
		return kernel.Mark{}, nil
	}
	copy(m.Sum[:], sum)
	return m, err
}

// Heads is the listing a kind's rows and errors are at, by id.
func (v *PG) Heads(ctx context.Context, kind string) ([]kernel.Head, error) {
	rows, err := v.pool.Query(ctx, `SELECT id, version FROM git_board_view_v1.rows WHERE kind = $1
		UNION ALL SELECT id, version FROM git_board_view_v1.broken WHERE kind = $1`, kind)
	if err != nil {
		return nil, err
	}
	var out []kernel.Head
	var h kernel.Head
	if _, err := pgx.ForEachRow(rows, []any{&h.ID, &h.Version}, func() error { out = append(out, h); return nil }); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b kernel.Head) int { return strings.Compare(string(a.ID), string(b.ID)) })
	return out, nil
}

// pgHead is an entity's head in the view: its row's, or its error's.
type pgHead struct {
	version kernel.Version
	search  bool // its row has terms and texts
	broken  bool
}

// Write changes a kind's rows in one transaction, each where its stored
// version is still its base, as the sqlite view's: writers of a kind take
// a lock of the transaction, one at a time.
func (v *PG) Write(ctx context.Context, kind string, w kernel.Write) error {
	var before, after kernel.Mark
	changed := map[kernel.ID]*row{} // what the rows files take: nil a row gone
	wrote := false
	err := pgx.BeginFunc(ctx, v.pool, func(tx pgx.Tx) error {
		h := fnv.New64a()
		h.Write([]byte(PGSchema + "/" + kind))
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(h.Sum64())); err != nil {
			return err
		}
		m, err := pgMark(ctx, tx, kind)
		if err != nil {
			return err
		}
		emptied := false
		if w.Reset && m.Rev != w.Rev { // another process may have started over meanwhile: then its rows stand
			for _, t := range []string{"terms", "texts", "rows", "broken", "fields"} {
				if _, err := tx.Exec(ctx, "DELETE FROM git_board_view_v1."+t+" WHERE kind = $1", kind); err != nil {
					return err
				}
			}
			m, emptied = kernel.Mark{Rev: w.Rev}, true
		} else if m.Rev != w.Rev {
			return nil // a stale process's: left out
		}
		before = m
		cur := map[kernel.ID]*pgHead{}
		if !emptied {
			ids := make([]string, 0, len(w.Put)+len(w.Drop))
			for _, p := range w.Put {
				ids = append(ids, string(p.ID))
			}
			for _, d := range w.Drop {
				ids = append(ids, string(d.ID))
			}
			rows, err := tx.Query(ctx, `SELECT id, version, search, false FROM git_board_view_v1.rows WHERE kind = $1 AND id = ANY($2)
				UNION ALL SELECT id, version, false, true FROM git_board_view_v1.broken WHERE kind = $1 AND id = ANY($2)`, kind, ids)
			if err != nil {
				return err
			}
			var id kernel.ID
			var h pgHead
			if _, err := pgx.ForEachRow(rows, []any{&id, &h.version, &h.search, &h.broken}, func() error {
				hh := h
				cur[id] = &hh
				return nil
			}); err != nil {
				return err
			}
		}
		b := &pgx.Batch{}
		sum := m.Sum
		fields := map[string]bool{}
		unsearch := func(id kernel.ID) {
			b.Queue("DELETE FROM git_board_view_v1.terms WHERE kind = $1 AND id = $2", kind, id)
			b.Queue("DELETE FROM git_board_view_v1.texts WHERE kind = $1 AND id = $2", kind, id)
		}
		remove := func(id kernel.ID, h *pgHead) {
			if h.broken {
				b.Queue("DELETE FROM git_board_view_v1.broken WHERE kind = $1 AND id = $2", kind, id)
				return
			}
			if h.search {
				unsearch(id)
			}
			b.Queue("DELETE FROM git_board_view_v1.rows WHERE kind = $1 AND id = $2", kind, id)
		}
		for _, p := range w.Put {
			h := cur[p.ID]
			there := h != nil
			var at kernel.Version
			if there {
				at = h.version
			}
			if at != p.Base || there == (p.Base == "") {
				continue // another process moved it meanwhile
			}
			switch {
			case p.Err != "":
				if there {
					remove(p.ID, h)
				}
				b.Queue("INSERT INTO git_board_view_v1.broken (kind, id, version, error) VALUES ($1, $2, $3, $4)", kind, p.ID, p.Version, p.Err)
				changed[p.ID] = nil
			default:
				if there && h.broken {
					remove(p.ID, h)
				} else if there && h.search {
					unsearch(p.ID)
				}
				d := p.Doc
				b.Queue(`INSERT INTO git_board_view_v1.rows (kind, id, version, created, updated, hidden, search, row)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (kind, id) DO UPDATE SET version = excluded.version,
					created = excluded.created, updated = excluded.updated, hidden = excluded.hidden, search = excluded.search,
					row = excluded.row`, kind, p.ID, p.Version, d.Created, d.Updated, d.Hidden, p.Search, d.Row)
				if p.Search {
					if len(d.Terms) > 0 {
						b.Queue(`INSERT INTO git_board_view_v1.terms (kind, term, id) SELECT $1, t, $3 FROM unnest($2::text[]) t
							ON CONFLICT DO NOTHING`, kind, d.Terms, p.ID)
					}
					for _, t := range d.Texts {
						b.Queue(`INSERT INTO git_board_view_v1.texts (kind, id, field, toks) VALUES ($1, $2, $3, $4)
							ON CONFLICT (kind, id, field) DO UPDATE SET toks = excluded.toks`, kind, p.ID, t.Field, toks(t.Body))
						fields[t.Field] = true
					}
				}
				changed[p.ID] = &row{id: p.ID, version: p.Version, created: d.Created, updated: d.Updated, hidden: d.Hidden, row: d.Row}
			}
			if there {
				sum = kernel.SubSum(sum, kernel.HeadSum(kernel.Head{ID: p.ID, Version: at}))
			}
			sum = kernel.AddSum(sum, kernel.HeadSum(kernel.Head{ID: p.ID, Version: p.Version}))
			cur[p.ID] = &pgHead{version: p.Version, search: p.Err == "" && p.Search, broken: p.Err != ""}
		}
		for _, d := range w.Drop {
			h := cur[d.ID]
			if h == nil || h.version != d.Version {
				continue
			}
			remove(d.ID, h)
			if !h.broken {
				changed[d.ID] = nil
			}
			sum = kernel.SubSum(sum, kernel.HeadSum(kernel.Head{ID: d.ID, Version: h.version}))
			delete(cur, d.ID)
		}
		for f := range fields {
			b.Queue("INSERT INTO git_board_view_v1.fields (kind, field) VALUES ($1, $2) ON CONFLICT DO NOTHING", kind, f)
		}
		after = kernel.Mark{Rev: m.Rev, Sum: sum, Search: m.Search || w.Search}
		b.Queue(`INSERT INTO git_board_view_v1.marks (kind, rev, sum, search) VALUES ($1, $2, $3, $4)
			ON CONFLICT (kind) DO UPDATE SET rev = excluded.rev, sum = excluded.sum, search = excluded.search`,
			kind, after.Rev, after.Sum[:], after.Search)
		wrote = true
		return tx.SendBatch(ctx, b).Close()
	})
	if err != nil {
		return fmt.Errorf("view: %w", err)
	}
	if wrote && !w.Reset {
		v.files.patchRows(kind, before, after, changed)
	}
	return nil
}

// toks is a text as the view keeps it for a search: its tokens, each
// between spaces.
func toks(body string) string {
	t := search.Tokens(body)
	if len(t) == 0 {
		return " "
	}
	return " " + strings.Join(t, " ") + " "
}

// Rows answers the visible rows matching q: the kind's rows read whole,
// narrowed by the ids the terms and the text match.
func (v *PG) Rows(ctx context.Context, kind string, q kernel.Query) ([]kernel.Row, error) {
	switch q.Sort {
	case "", "created", "updated":
	default:
		return nil, fmt.Errorf("view: sort by created or updated, not %q", q.Sort)
	}
	keep, err := v.matching(ctx, kind, q)
	if err != nil {
		return nil, err
	}
	m, err := v.Mark(ctx, kind)
	if err != nil {
		return nil, err
	}
	all, err := v.files.all(ctx, kind, m, v.readRows)
	if err != nil {
		return nil, err
	}
	out := make([]kernel.Row, 0, len(all))
	for _, r := range sorted(all, q) {
		if r.hidden || keep != nil && !keep[r.id] {
			continue
		}
		out = append(out, kernel.Row{ID: r.id, Version: r.version, Row: r.row})
		if q.Limit > 0 && len(out) == q.Limit {
			break
		}
	}
	return out, nil
}

// matching are the ids q's terms and text match; nil when q has none.
func (v *PG) matching(ctx context.Context, kind string, q kernel.Query) (map[kernel.ID]bool, error) {
	args := []any{kind}
	arg := func(a any) string {
		args = append(args, a)
		return fmt.Sprintf("$%d", len(args))
	}
	var where []string
	for _, t := range q.All {
		where = append(where, "r.id IN (SELECT id FROM git_board_view_v1.terms WHERE kind = $1 AND term = "+arg(t)+")")
	}
	for _, group := range q.Any {
		if len(group) > 0 {
			where = append(where, "r.id IN (SELECT id FROM git_board_view_v1.terms WHERE kind = $1 AND term = ANY("+arg(group)+"))")
		}
	}
	if len(q.Not) > 0 {
		where = append(where, "r.id NOT IN (SELECT id FROM git_board_view_v1.terms WHERE kind = $1 AND term = ANY("+arg(q.Not)+"))")
	}
	if q.Text != "" {
		in := ""
		if len(q.In) > 0 {
			var known bool
			if err := v.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM git_board_view_v1.fields WHERE kind = $1 AND field = ANY($2))",
				kind, q.In).Scan(&known); err != nil {
				return nil, fmt.Errorf("view: %w", err)
			}
			if !known {
				return map[kernel.ID]bool{}, nil // looked for nowhere
			}
			in = " AND field = ANY(" + arg(q.In) + ")"
		}
		for _, w := range strings.Fields(q.Text) {
			t := search.Tokens(w)
			if len(t) == 0 {
				continue
			}
			// tokens hold letters, numbers and private use characters:
			// none of LIKE's own
			where = append(where, "r.id IN (SELECT id FROM git_board_view_v1.texts WHERE kind = $1"+in+
				" AND toks LIKE "+arg("% "+strings.Join(t, " ")+"%")+")")
		}
	}
	if len(where) == 0 {
		return nil, nil
	}
	rows, err := v.pool.Query(ctx, "SELECT r.id FROM git_board_view_v1.rows r WHERE r.kind = $1 AND "+strings.Join(where, " AND "), args...)
	if err != nil {
		return nil, fmt.Errorf("view: %w", err)
	}
	keep := map[kernel.ID]bool{}
	var id kernel.ID
	if _, err := pgx.ForEachRow(rows, []any{&id}, func() error { keep[id] = true; return nil }); err != nil {
		return nil, fmt.Errorf("view: %w", err)
	}
	return keep, nil
}

// readRows reads a kind's mark and rows from the tables, in one snapshot,
// by (created, id) as Go compares ids.
func (v *PG) readRows(ctx context.Context, kind string) (kernel.Mark, []row, error) {
	var m kernel.Mark
	var out []row
	err := pgx.BeginTxFunc(ctx, v.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var err error
		if m, err = pgMark(ctx, tx, kind); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id, version, created, updated, hidden, row FROM git_board_view_v1.rows
			WHERE kind = $1 ORDER BY created, id COLLATE "C"`, kind)
		if err != nil {
			return err
		}
		var r row
		_, err = pgx.ForEachRow(rows, []any{&r.id, &r.version, &r.created, &r.updated, &r.hidden, &r.row}, func() error {
			out = append(out, r)
			return nil
		})
		return err
	})
	return m, out, err
}

// Resolve answers up to two visible ids starting with prefix.
func (v *PG) Resolve(ctx context.Context, kind, prefix string) ([]kernel.ID, error) {
	rows, err := v.pool.Query(ctx, `SELECT id FROM git_board_view_v1.rows WHERE kind = $1 AND NOT hidden AND id LIKE $2
		ORDER BY id COLLATE "C" LIMIT 2`, kind, prefix+"%")
	if err != nil {
		return nil, err
	}
	var out []kernel.ID
	var id kernel.ID
	_, err = pgx.ForEachRow(rows, []any{&id}, func() error { out = append(out, id); return nil })
	return out, err
}

// Unsearchable are the rows whose terms and texts aren't kept yet,
// hidden ones too.
func (v *PG) Unsearchable(ctx context.Context, kind string) ([]kernel.Head, error) {
	rows, err := v.pool.Query(ctx, "SELECT id, version FROM git_board_view_v1.rows WHERE kind = $1 AND NOT search", kind)
	if err != nil {
		return nil, err
	}
	var out []kernel.Head
	var h kernel.Head
	_, err = pgx.ForEachRow(rows, []any{&h.ID, &h.Version}, func() error { out = append(out, h); return nil })
	return out, err
}

// Broken are the entities that failed to load, by kind then id.
func (v *PG) Broken(ctx context.Context) ([]kernel.Broken, error) {
	rows, err := v.pool.Query(ctx, `SELECT kind, id, error FROM git_board_view_v1.broken ORDER BY kind COLLATE "C", id COLLATE "C"`)
	if err != nil {
		return nil, err
	}
	var out []kernel.Broken
	var b kernel.Broken
	var msg string
	_, err = pgx.ForEachRow(rows, []any{&b.Kind, &b.ID, &msg}, func() error {
		b.Err = errors.New(msg)
		out = append(out, b)
		return nil
	})
	return out, err
}
