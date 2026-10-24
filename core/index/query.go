package index

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode"

	"github.com/guilbep/git-board/core/issue"
)

// Query asks for issues. An empty field filters nothing. Sort is "created"
// or "updated", the default; Desc sorts newest first; a Limit of 0 answers
// all.
type Query struct {
	Status       issue.Status
	Labels       []string
	Author, Text string
	Sort         string
	Desc         bool
	Limit        int
}

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
	if err := ix.Refresh(ctx); err != nil {
		return nil, err
	}
	where, args := []string{"1"}, []any{}
	if q.Status != "" {
		where, args = append(where, "status = ?"), append(args, string(q.Status))
	}
	for _, label := range q.Labels {
		where, args = append(where, "id IN (SELECT issue FROM labels WHERE label = ?)"), append(args, label)
	}
	if q.Author != "" {
		where, args = append(where, "author = ?"), append(args, q.Author)
	}
	if m := match(q.Text); m != "" {
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
	rows, err := ix.db.QueryContext(ctx, query, args...)
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

// match is the FTS5 query for a search text. Each word is quoted, so FTS5's
// syntax is plain text, and marked as a prefix; FTS5 ANDs them. A word with
// no token (unicode61's: letters, numbers, private use) is left out. It is
// "" when no word is left, which matches every issue.
func match(text string) string {
	var words []string
	for _, w := range strings.Fields(text) {
		if strings.IndexFunc(w, isToken) >= 0 {
			words = append(words, `"`+strings.ReplaceAll(w, `"`, `""`)+`"*`)
		}
	}
	return strings.Join(words, " ")
}

func isToken(r rune) bool { return unicode.In(r, unicode.L, unicode.N, unicode.Co) }

// ForAccount answers the person that owns an account: of the persons
// holding it, the one whose id sorts first. It refreshes first.
func (ix *Index) ForAccount(ctx context.Context, server, id string) (string, bool, error) {
	if err := ix.Refresh(ctx); err != nil {
		return "", false, err
	}
	var p sql.NullString
	err := ix.db.QueryRowContext(ctx, "SELECT min(person) FROM accounts WHERE server = ? AND id = ?", server, id).Scan(&p)
	if err != nil {
		return "", false, fmt.Errorf("index: account: %w", err)
	}
	return p.String, p.Valid, nil
}
