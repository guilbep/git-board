package httpapi

import (
	"cmp"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/guilbep/git-board/next/domain/issues"
)

// A cursor names the last item of a page by where it sat in the list's
// order: what the list sorted by, as it was then, and its id. The next
// page starts at the first item that sorts after it now. An item gone
// since (closed, moved to another lane) or one that moved skips nothing,
// and items added before it don't come again: a place, not an index.
type cursorAt struct {
	Sort    string    `json:"s,omitempty"` // the list's sort, which the next page must keep
	Created time.Time `json:"c,omitzero"`
	Updated time.Time `json:"u,omitzero"`
	Title   string    `json:"t,omitempty"` // a title sort's
	Value   string    `json:"v,omitempty"` // a field sort's
	Since   time.Time `json:"w,omitzero"`  // a Work row's
	ID      string    `json:"i"`
}

var errBadCursor = errors.New("not a cursor this server gave, or one of another sort")

func (c cursorAt) String() string {
	data, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(data)
}

// parseCursor reads a cursor; "" is none, the first page.
func parseCursor(s string) (*cursorAt, error) {
	if s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	var c cursorAt
	if err != nil || json.Unmarshal(raw, &c) != nil || c.ID == "" {
		return nil, invalid("cursor", errBadCursor)
	}
	return &c, nil
}

// resume is where the page after c starts in list: the first item after
// it. after must hold for a suffix of list, as an order gives.
func resume[T any](list []T, c *cursorAt, after func(T) bool) int {
	if c == nil {
		return 0
	}
	return sort.Search(len(list), func(i int) bool { return after(list[i]) })
}

func sortKey(spec string) string {
	key, _, _ := strings.Cut(strings.TrimSpace(spec), " ")
	return key
}

// issueCursor is the cursor of is, the last of a page sorted by spec.
func issueCursor(spec string, is issues.Issue) string {
	c := cursorAt{Sort: spec, Created: is.CreatedAt, ID: is.ID}
	switch key := sortKey(spec); key {
	case "", "created":
	case "updated":
		c.Updated = is.UpdatedAt
	case "title":
		c.Title = is.ShownTitle()
	default:
		c.Value = issues.FieldValue(is, key)
	}
	return c.String()
}

// issueAfter says whether an issue sorts after the cursor's in a list
// sorted by spec: by spec's key, as Schema.Sort compares it, then as the
// list comes (by creation, then id), which the sort keeps for a tie.
func issueAfter(sch *issues.Schema, spec string, c *cursorAt) func(issues.Issue) bool {
	if c == nil {
		return nil
	}
	// the cursor's issue, as it was: what the sort reads of it
	const marker = "\x00cursor"
	at := issues.Issue{ID: c.ID, Title: c.Title, Body: marker, CreatedAt: c.Created, UpdatedAt: c.Updated}
	if key := sortKey(spec); c.Value != "" {
		at.Labels = []string{key + ":" + c.Value}
	}
	// moves says whether sorting the pair a, b puts b first: b sorts
	// strictly before a by the key (a key the pair can't sort by: no)
	moves := func(a, b issues.Issue) bool {
		pair := []issues.Issue{a, b}
		return sch.Sort(pair, spec) == nil && pair[0].Body != a.Body
	}
	return func(x issues.Issue) bool {
		x.Body = ""
		switch {
		case moves(at, x): // x before the cursor's
			return false
		case moves(x, at): // the cursor's before x
			return true
		}
		return cmp.Or(x.CreatedAt.Compare(at.CreatedAt), strings.Compare(x.ID, at.ID)) > 0
	}
}
