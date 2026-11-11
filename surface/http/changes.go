package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/domain/issues"
)

// The board as a page keeps it (ADR 0023): every issue at once
// (GET /api/snapshot), then what changed since (GET /api/changes), the
// change events saying when; a filter the page can't answer itself (free
// text, involves:, follows:) as the ids it matches (GET /api/issues/ids).
// A reply built from the board says its stamp (ETag), and a page that
// holds it gets a 304, no list built.

// ringSize is how many issue changes the server remembers: a page that
// fell further behind takes the snapshot again.
const ringSize = 4096

// changeRing is the issues' changes, numbered: a cursor is the server's
// boot, then the number of the last change a page has.
type changeRing struct {
	mu   sync.Mutex
	boot string
	seq  uint64
	ids  []string // ids[i] is change seq-len(ids)+1+i's issue
}

func newChangeRing() *changeRing {
	b := make([]byte, 6)
	rand.Read(b)
	return &changeRing{boot: hex.EncodeToString(b)}
}

// add records a change of issue id, and answers the cursor after it.
func (c *changeRing) add(id string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	c.ids = append(c.ids, id)
	if len(c.ids) > ringSize {
		c.ids = c.ids[len(c.ids)-ringSize:]
	}
	return c.boot + ":" + strconv.FormatUint(c.seq, 10)
}

// cursor is where the ring is now.
func (c *changeRing) cursor() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.boot + ":" + strconv.FormatUint(c.seq, 10)
}

// since is the issues changed after cursor, each once, and the cursor
// now; ok false when the cursor is another boot's, or older than the
// ring holds: the page takes the snapshot again.
func (c *changeRing) since(cursor string) (ids []string, now string, ok bool) {
	boot, n, _ := strings.Cut(cursor, ":")
	after, err := strconv.ParseUint(n, 10, 64)
	c.mu.Lock()
	defer c.mu.Unlock()
	now = c.boot + ":" + strconv.FormatUint(c.seq, 10)
	first := c.seq - uint64(len(c.ids)) // the change before the oldest held
	if err != nil || boot != c.boot || after > c.seq || after < first {
		return nil, now, false
	}
	seen := map[string]bool{}
	for _, id := range c.ids[after-first:] {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, now, true
}

// stamp is the board's version for a reply: its issues', people's,
// records' and counter's (issues.Service.Version), and which open of the
// board it is (a config changed opens it again); "" when it can't be told.
func stamp(ctx context.Context) string {
	b := board(ctx)
	v := b.Issues.Version(ctx)
	if v == "" {
		return ""
	}
	h := sha256.Sum256(fmt.Appendf(nil, "%p\n%s", b, v))
	return hex.EncodeToString(h[:12])
}

type snapshotRes struct {
	Stamp  string      `json:"stamp"`
	Cursor string      `json:"cursor"`
	Issues []issueJSON `json:"issues"`
}

// snapshot is GET /api/snapshot: every issue, open and closed, as a list
// shows it; the cursor to ask the changes from; its stamp, as its ETag.
func (s *Server) snapshot(ctx context.Context, r conditional) (Reply, error) {
	st := stamp(ctx)
	if st != "" && version(r.IfNoneMatch) == st {
		return Reply{Status: http.StatusNotModified, ETag: st}, nil
	}
	cursor := s.changes.cursor() // before the read: a change after it is told again, harmlessly
	all, err := board(ctx).Issues.List(ctx, issues.ListReq{Filter: ""})
	if err != nil {
		return Reply{}, err
	}
	res := snapshotRes{Stamp: st, Cursor: cursor, Issues: make([]issueJSON, len(all))}
	for i, is := range all {
		res.Issues[i] = issueJSON{Issue: is, Version: string(is.Version)}
	}
	return Reply{ETag: st, Body: res}, nil
}

type conditional struct {
	IfNoneMatch string `header:"If-None-Match"`
}

type changesReq struct {
	Since string `query:"since"`
}

type changesRes struct {
	Cursor  string      `json:"cursor"`
	Put     []issueJSON `json:"put"`
	Dropped []string    `json:"dropped"`
}

// changes is GET /api/changes?since=: the issues changed since a cursor,
// each as it is now, those gone by id; 410 gone when the cursor is older
// than the server holds (a restart, a page asleep for long).
func (s *Server) changesSince(ctx context.Context, r changesReq) (changesRes, error) {
	ids, now, ok := s.changes.since(r.Since)
	if !ok {
		return changesRes{}, &coded{code: "gone", status: http.StatusGone, err: fmt.Errorf("cursor %q: take the snapshot again", r.Since)}
	}
	res := changesRes{Cursor: now, Put: []issueJSON{}, Dropped: []string{}}
	if len(ids) == 0 {
		return res, nil
	}
	all, err := board(ctx).Issues.List(ctx, issues.ListReq{Filter: ""}) // the kept build: no issue read
	if err != nil {
		return changesRes{}, err
	}
	byID := make(map[string]issues.Issue, len(all))
	for _, is := range all {
		byID[is.ID] = is
	}
	for _, id := range ids {
		if is, ok := byID[id]; ok {
			res.Put = append(res.Put, issueJSON{Issue: is, Version: string(is.Version)})
		} else {
			res.Dropped = append(res.Dropped, id)
		}
	}
	return res, nil
}

type idsReq struct {
	Filters []string `query:"f"`
	Sort    string   `query:"sort"`
}

type idsRes struct {
	Stamp string     `json:"stamp"`
	Lists [][]string `json:"lists"`
}

// issueIDs is GET /api/issues/ids?f=…&f=…&sort=: the ids each filter
// matches, in the sort, in the order asked, from one snapshot of the
// board: a page that keeps the board asks its lanes and counts so, the
// filter grammar the server's alone, and shows its own copies.
func issueIDs(ctx context.Context, r idsReq) (idsRes, error) {
	if n := len(r.Filters); n == 0 || n > maxBatch {
		return idsRes{}, invalid("f", fmt.Errorf("%d lists: from 1 to %d", n, maxBatch))
	}
	b := board(ctx)
	ctx = issues.ReadOnce(ctx)
	res := idsRes{Stamp: stamp(ctx), Lists: make([][]string, 0, len(r.Filters))}
	for _, f := range r.Filters {
		found, err := b.Issues.List(ctx, issues.ListReq{Filter: f})
		if err != nil {
			return idsRes{}, on("f", err)
		}
		if err := b.Issues.Schema.Sort(found, r.Sort); err != nil {
			return idsRes{}, on("sort", err)
		}
		ids := make([]string, len(found))
		for i, is := range found {
			ids[i] = is.ID
		}
		res.Lists = append(res.Lists, ids)
	}
	return res, nil
}

// conditionalGets are the lists a page asks again on each change: built
// from the board alone, so the board's stamp, the query, who @me is and
// the day (@today, a sprint) name their reply.
var conditionalGets = map[string]bool{"/api/issues": true, "/api/issues/batch": true, "/api/labels": true, "/api/issues/ids": true}

// withConditional answers a page that holds a list's reply 304, its ETag
// the reply's name, before any list is built.
func withConditional(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !conditionalGets[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		st := stamp(r.Context())
		if st == "" {
			next.ServeHTTP(w, r)
			return
		}
		b := board(r.Context())
		h := sha256.Sum256(fmt.Appendf(nil, "%s\n%s\n%s\n%s\n%s", st, r.URL.Path, r.URL.RawQuery, b.Issues.MeName, time.Now().Format(time.DateOnly)))
		etag := hex.EncodeToString(h[:12])
		w.Header().Set("ETag", strconv.Quote(etag))
		if version(r.Header.Get("If-None-Match")) == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		next.ServeHTTP(w, r)
	})
}
