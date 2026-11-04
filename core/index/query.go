package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/search"
)

// Query asks for issues. An empty field filters nothing. In names the
// texts Text is looked for in (search.Texts: title, body, comments); none: all
// of them. Sort is "created" or "updated", the default; Desc sorts newest
// first; a Limit of 0 answers all.
type Query struct {
	Status       issue.Status
	Labels       []string
	Author, Text string
	In           []string
	Sort         string
	Desc         bool
	Limit        int
}

// weights are each text's weight in PostgreSQL's tsvector (search.Texts,
// the sqlite index's columns in order).
var weights = map[string]string{"title": "A", "body": "B", "comments": "C"}

// Summary is what the index holds for an issue.
type Summary struct {
	ID, Title        string
	Status           issue.Status
	Labels           []string
	Author           string
	Created, Updated int64
	Comments         int
}

// sorts are the columns a query sorts by.
var sorts = map[string]string{"": "updated", "created": "created", "updated": "updated"}

// Issues answers the issues matching q. It refreshes first.
func (ix *Index) Issues(ctx context.Context, q Query) ([]Summary, error) {
	if q.Status != "" && q.Status != issue.Open && q.Status != issue.Closed {
		return nil, fmt.Errorf("index: unknown status %q", q.Status)
	}
	col, ok := sorts[q.Sort]
	if !ok {
		return nil, fmt.Errorf("index: unknown sort %q", q.Sort)
	}
	if q.Limit < 0 {
		return nil, fmt.Errorf("index: negative limit %d", q.Limit)
	}
	for _, in := range q.In {
		if !slices.Contains(search.Texts, in) {
			return nil, fmt.Errorf("index: unknown text %q", in)
		}
	}
	if err := ix.Refresh(ctx); err != nil {
		return nil, err
	}
	where, args := []string{"1 = 1"}, []any{}
	if q.Status != "" {
		where, args = append(where, "status = ?"), append(args, string(q.Status))
	}
	for _, label := range q.Labels {
		where, args = append(where, "id IN (SELECT issue FROM labels WHERE label = ?)"), append(args, label)
	}
	if q.Author != "" {
		where, args = append(where, "author = ?"), append(args, q.Author)
	}
	switch m := match(q.Text, q.In); {
	case ix.pg:
		if m := tsquery(q.Text, q.In); m != "" {
			where, args = append(where, "n IN (SELECT n FROM texts WHERE doc @@ to_tsquery('simple', ?))"), append(args, m)
		}
	case m != "":
		where, args = append(where, "n IN (SELECT rowid FROM texts WHERE texts MATCH ?)"), append(args, m)
	}
	dir := "ASC"
	if q.Desc {
		dir = "DESC"
	}
	query := "SELECT id, title, status, labels, author, created, updated, comments FROM issues WHERE " +
		strings.Join(where, " AND ") + " ORDER BY " + col + " " + dir + ", id " + dir
	if q.Limit > 0 {
		query, args = query+" LIMIT ?", append(args, q.Limit)
	}
	rows, err := ix.db.QueryContext(ctx, ix.sql(query), args...)
	if err != nil {
		return nil, fmt.Errorf("index: issues: %w", err)
	}
	defer rows.Close()
	out := []Summary{}
	for rows.Next() {
		var s Summary
		var labels string
		if err := rows.Scan(&s.ID, &s.Title, &s.Status, &labels, &s.Author, &s.Created, &s.Updated, &s.Comments); err != nil {
			return nil, err
		}
		s.Labels = []string{}
		if labels != "" {
			s.Labels = strings.Split(labels, "\n")
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// State is an issue as the index keeps it: issue.Apply's fold at a head.
type State struct {
	Head  string
	Issue *issue.Issue
}

// States answers every issue the index holds, by id, folded at the head it
// read, and the broken issues it keeps out: deleted ones are neither. For
// each, cached gives the issue already folded at that head, or nil: the
// index then reads the state it keeps, so a caller that has most of them
// reads few. It refreshes first.
func (ix *Index) States(ctx context.Context, cached func(id, head string) *issue.Issue) (map[string]State, []Broken, error) {
	fresh, err := ix.refresh(ctx) // what it folds, it needn't read back
	if err != nil {
		return nil, nil, err
	}
	const join = " FROM issues i JOIN entities e ON e.kind = 'issue' AND e.id = i.id"
	rows, err := ix.db.QueryContext(ctx, `SELECT id, head, error FROM entities
		WHERE kind = 'issue' AND (error IS NOT NULL OR id IN (SELECT id FROM issues))`)
	if err != nil {
		return nil, nil, fmt.Errorf("index: states: %w", err)
	}
	out := map[string]State{}
	var missing []string
	var broken []Broken
	for rows.Next() {
		var id, head string
		var msg sql.NullString
		if err := rows.Scan(&id, &head, &msg); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if msg.Valid {
			broken = append(broken, Broken{"issue", id, msg.String})
			continue
		}
		if f, ok := fresh[id]; ok && f.Head == head {
			out[id] = f
		} else if is := cached(id, head); is != nil {
			out[id] = State{head, is}
		} else {
			missing = append(missing, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	// A state comes with its head, in one statement: an issue that moved
	// since the heads were read comes as it is now, and one that went,
	// not at all.
	type row struct {
		id, head string
		state    []byte
	}
	var read []row
	scan := func(where string, args ...any) error {
		rows, err := ix.db.QueryContext(ctx, ix.sql("SELECT i.id, e.head, i.state"+join+where), args...)
		if err != nil {
			return fmt.Errorf("index: states: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.head, &r.state); err != nil {
				return err
			}
			if _, ok := out[r.id]; !ok {
				read = append(read, r)
			}
		}
		return rows.Err()
	}
	if len(missing) > len(out) { // most of them: one read of all
		err = scan("")
	} else {
		for _, id := range missing {
			if err = scan(" WHERE i.id = ?", id); err != nil {
				break
			}
		}
	}
	if err != nil {
		return nil, nil, err
	}
	// decoding is most of a cold read: on every CPU
	issues := make([]issue.Issue, len(read))
	errs := make([]error, len(read))
	var wg sync.WaitGroup
	workers := min(runtime.GOMAXPROCS(0), len(read))
	for w := range workers {
		wg.Go(func() {
			for i := w; i < len(read); i += workers {
				errs[i] = json.Unmarshal(read[i].state, &issues[i])
			}
		})
	}
	wg.Wait()
	for i, r := range read {
		if errs[i] != nil {
			return nil, nil, fmt.Errorf("index: issue %s: %w", r.id, errs[i])
		}
		out[r.id] = State{r.head, &issues[i]}
	}
	slices.SortFunc(broken, func(a, b Broken) int { return strings.Compare(a.ID, b.ID) })
	return out, broken, nil
}

// match is the FTS5 query for a search text. Each word is quoted, so FTS5's
// syntax is plain text, and marked as a prefix; FTS5 ANDs them. A word with
// no token (unicode61's: letters, numbers, private use) is left out. It is
// "" when no word is left, which matches every issue. in, when given,
// are the columns each word is looked for in.
func match(text string, in []string) string {
	cols := ""
	if len(in) > 0 {
		cols = "{" + strings.Join(in, " ") + "} : "
	}
	var words []string
	for _, w := range strings.Fields(text) {
		if strings.IndexFunc(w, search.IsToken) >= 0 {
			words = append(words, cols+`"`+strings.ReplaceAll(w, `"`, `""`)+`"*`)
		}
	}
	return strings.Join(words, " ")
}

// tsquery is match for PostgreSQL, as FTS5 reads match's: each word's
// tokens in a row (<->), the last a prefix, the words ANDed; in
// to_tsquery's syntax with the 'simple' configuration, over the tokens
// that tokens gives. in, when given, are the texts each token is looked
// for in, by their weights. It is "" when no token is left, which matches
// every issue.
func tsquery(text string, in []string) string {
	w8 := ""
	for _, t := range in {
		w8 += weights[t]
	}
	var words []string
	for _, w := range strings.Fields(text) {
		toks := search.Tokens(w)
		if len(toks) == 0 {
			continue
		}
		for i, tok := range toks {
			toks[i] = "'" + tok + "'"
			if i < len(toks)-1 && w8 != "" {
				toks[i] += ":" + w8
			}
		}
		words = append(words, strings.Join(toks, " <-> ")+":*"+w8)
	}
	return strings.Join(words, " & ")
}

// tokens is text as unicode61 tokenizes it (search.Tokens), joined by
// spaces, for PostgreSQL to index and search alike.
func tokens(text string) string { return strings.Join(search.Tokens(text), " ") }

// ForAccount answers the person that owns an account: of the persons
// holding it, the one whose id sorts first. It refreshes first.
func (ix *Index) ForAccount(ctx context.Context, server, id string) (string, bool, error) {
	if err := ix.Refresh(ctx); err != nil {
		return "", false, err
	}
	var p sql.NullString
	err := ix.db.QueryRowContext(ctx, ix.sql("SELECT min(person) FROM accounts WHERE server = ? AND id = ?"), server, id).Scan(&p)
	if err != nil {
		return "", false, fmt.Errorf("index: account: %w", err)
	}
	return p.String, p.Valid, nil
}

// Labels answers every label in use, sorted, each once. It refreshes first.
func (ix *Index) Labels(ctx context.Context) ([]string, error) {
	if err := ix.Refresh(ctx); err != nil {
		return nil, err
	}
	query := "SELECT DISTINCT label FROM labels ORDER BY label"
	if ix.pg { // byte for byte, whatever the database's collation
		query = `SELECT DISTINCT label COLLATE "C" FROM labels ORDER BY 1`
	}
	rows, err := ix.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("index: labels: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

var (
	// ErrNoMatch: no issue or person has an id with that prefix.
	ErrNoMatch = errors.New("index: no such id")
	// ErrAmbiguous: several have.
	ErrAmbiguous = errors.New("index: ambiguous id prefix")
)

var prefixPattern = regexp.MustCompile(`^[0-9a-f]{1,40}$`)

// Resolve answers the id of the issue or person ("issue" or "person")
// whose id starts with prefix: ErrNoMatch for none, ErrAmbiguous for
// several. A prefix is 1 to 40 lowercase hex digits. It refreshes first.
func (ix *Index) Resolve(ctx context.Context, kind, prefix string) (string, error) {
	table := map[string]string{"issue": "issues", "person": "persons"}[kind]
	if table == "" {
		return "", fmt.Errorf("index: unknown kind %q", kind)
	}
	if !prefixPattern.MatchString(prefix) {
		return "", fmt.Errorf("index: bad id prefix %q", prefix)
	}
	if err := ix.Refresh(ctx); err != nil {
		return "", err
	}
	// the prefix is hex only, so the bounds compare as plain text
	query := "SELECT id FROM " + table + " WHERE id >= ? AND id < ? ORDER BY id LIMIT 2"
	if ix.pg {
		query = "SELECT id FROM " + table + ` WHERE id COLLATE "C" >= ? AND id COLLATE "C" < ? ORDER BY id COLLATE "C" LIMIT 2`
	}
	rows, err := ix.db.QueryContext(ctx, ix.sql(query), prefix, prefix+"g")
	if err != nil {
		return "", fmt.Errorf("index: resolve: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	switch {
	case len(ids) == 0:
		return "", fmt.Errorf("%w: %s %s", ErrNoMatch, kind, prefix)
	case len(ids) > 1:
		return "", fmt.Errorf("%w: %s %s", ErrAmbiguous, kind, prefix)
	}
	return ids[0], nil
}
