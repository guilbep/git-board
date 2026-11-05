package httpapi

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/guilbep/git-board/next/app"
	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/kernel"
)

// The issue routes (docs/rewrite/api.md, Issues).

// issueJSON is an issue as `ls --json` prints it, and its version.
type issueJSON struct {
	issues.Issue
	Version string `json:"version"`
}

// threadJSON is a thread as `show --json` prints it, and its version.
type threadJSON struct {
	issues.Thread
	Version string `json:"version"`
}

// page is one page of a list.
type page[T any] struct {
	Items []T    `json:"items"`
	Total int    `json:"total"`
	Next  string `json:"next,omitempty"`
}

// items is a list that isn't paged.
type items[T any] struct {
	Items []T `json:"items"`
}

// The page sizes.
const (
	defaultLimit = 100
	maxLimit     = 1000
)

type listReq struct {
	Filter *string `query:"filter"`
	Sort   string  `query:"sort"`
	Limit  *int    `query:"limit"`
	Cursor string  `query:"cursor"`
	At     string  `query:"at"`
}

func listIssues(ctx context.Context, r listReq) (page[issueJSON], error) {
	b := board(ctx)
	req := issues.ListReq{Filter: issues.DefaultFilter}
	if r.Filter != nil {
		req.Filter = *r.Filter
	}
	limit := defaultLimit
	if r.Limit != nil {
		limit = *r.Limit
	}
	if limit < 0 || limit > maxLimit {
		return page[issueJSON]{}, invalid("limit", fmt.Errorf("limit %d: from 1 to %d, or 0 for all", limit, maxLimit))
	}
	var found []issues.Issue
	var err error
	if r.At != "" {
		at, perr := parseAt(r.At)
		if perr != nil {
			return page[issueJSON]{}, invalid("at", perr)
		}
		found, err = b.Issues.ListAt(ctx, req, at)
	} else {
		found, err = b.Issues.List(ctx, req)
	}
	if err != nil {
		return page[issueJSON]{}, on("filter", err)
	}
	if err := b.Issues.Schema.Sort(found, r.Sort); err != nil {
		return page[issueJSON]{}, on("sort", err)
	}
	start, err := after(found, r.Cursor)
	if err != nil {
		return page[issueJSON]{}, err
	}
	end := len(found)
	if limit > 0 {
		end = min(end, start+limit)
	}
	p := page[issueJSON]{Items: make([]issueJSON, 0, end-start), Total: len(found)}
	for _, is := range found[start:end] {
		p.Items = append(p.Items, issueJSON{Issue: is, Version: string(is.Version)})
	}
	if end < len(found) {
		p.Next = cursor(end, found[end-1].ID)
	}
	return p, nil
}

// cursor names the last item of a page: its place and its id.
func cursor(n int, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(n) + ":" + id))
}

// after is where the page after a cursor starts: after its item, found
// where the list has it now, else at its place.
func after(list []issues.Issue, c string) (int, error) {
	if c == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(c)
	bad := invalid("cursor", errors.New("not a cursor this server gave"))
	if err != nil {
		return 0, bad
	}
	ns, id, ok := strings.Cut(string(raw), ":")
	n, err := strconv.Atoi(ns)
	if !ok || err != nil || n < 0 {
		return 0, bad
	}
	if n <= len(list) && n > 0 && list[n-1].ID == id {
		return n, nil
	}
	for i, is := range list {
		if is.ID == id {
			return i + 1, nil
		}
	}
	return min(n, len(list)), nil
}

// parseAt is a time an `at` names: RFC 3339, or a date, its end.
func parseAt(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if d, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return d.AddDate(0, 0, 1).Add(-time.Second), nil
	}
	return time.Time{}, fmt.Errorf("at %q: a time (2026-10-01T14:30:00Z) or a date (2026-10-01)", s)
}

// thread is the thread of the issue id, its texts rendered.
func thread(ctx context.Context, id string, status int) (Reply, error) {
	th, err := board(ctx).Issues.Show(ctx, id)
	if err != nil {
		return Reply{}, err
	}
	return threadReply(th, status), nil
}

func threadReply(th issues.Thread, status int) Reply {
	for i, e := range th.Events {
		if e.Body != "" {
			th.Events[i].HTML = render(e.Body)
		}
		for j, v := range e.History { // the description's versions
			th.Events[i].History[j].HTML = render(v.Body)
		}
	}
	return Reply{Status: status, ETag: string(th.Version), Body: threadJSON{Thread: th, Version: string(th.Version)}}
}

type showReq struct {
	ID          string `path:"id"`
	At          string `query:"at"`
	IfNoneMatch string `header:"If-None-Match"`
}

func showIssue(ctx context.Context, r showReq) (Reply, error) {
	if r.At != "" {
		at, err := parseAt(r.At)
		if err != nil {
			return Reply{}, invalid("at", err)
		}
		th, err := board(ctx).Issues.ShowAt(ctx, r.ID, at)
		if err != nil {
			return Reply{}, err
		}
		rep := threadReply(th, http.StatusOK)
		rep.ETag = "" // as it was: no version to write from
		return rep, nil
	}
	rep, err := thread(ctx, r.ID, http.StatusOK)
	if err == nil && r.IfNoneMatch != "" && version(r.IfNoneMatch) == rep.ETag {
		return Reply{Status: http.StatusNotModified, ETag: rep.ETag}, nil
	}
	return rep, err
}

type createReq struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels"`
	Set    []string `json:"set"`
	Force  bool     `json:"force"`
}

func createIssue(ctx context.Context, r createReq) (Reply, error) {
	if strings.TrimSpace(r.Title) == "" {
		return Reply{}, invalid("title", errors.New("an issue needs a title"))
	}
	is, err := board(ctx).Issues.New(ctx, issues.NewReq{Title: r.Title, Body: r.Body, Labels: r.Labels, Assign: r.Set, Force: r.Force})
	if err != nil {
		return Reply{}, on("set", err)
	}
	rep, err := thread(ctx, is.ID, http.StatusCreated)
	rep.Location = "/api/issues/" + is.ID
	return rep, err
}

// expect makes the request's writes on the issue name compare-and-swap
// from the version If-Match names, when it names one; it answers the
// issue's whole id.
func expect(ctx context.Context, name, ifMatch string) (context.Context, string, error) {
	id, err := board(ctx).Issues.Resolve(ctx, name)
	if err != nil {
		return ctx, "", err
	}
	if v := version(ifMatch); v != "" {
		ctx = kernel.Expect(ctx, id, kernel.Version(v))
	}
	return ctx, string(id), nil
}

type labelsChange struct {
	Add    []string `json:"add"`
	Remove []string `json:"remove"`
}

type patchReq struct {
	ID          string        `path:"id"`
	IfMatch     string        `header:"If-Match"`
	Title       *string       `json:"title"`
	Body        *string       `json:"body"`
	Base        *string       `json:"base"`
	Labels      *labelsChange `json:"labels"`
	Set         []string      `json:"set"`
	Force       bool          `json:"force"`
	Status      string        `json:"status"`
	Reason      string        `json:"reason"`
	DuplicateOf string        `json:"duplicate_of"`
	Comment     string        `json:"comment"`
}

func patchIssue(ctx context.Context, r patchReq) (Reply, error) {
	ctx, id, err := expect(ctx, r.ID, r.IfMatch)
	if err != nil {
		return Reply{}, err
	}
	is := board(ctx).Issues
	switch {
	case r.Status != "" && r.Status != "open" && r.Status != "closed":
		return Reply{}, invalid("status", fmt.Errorf("status %q: open or closed", r.Status))
	case r.Status == "" && (r.Comment != "" || r.Reason != "" || r.DuplicateOf != ""):
		return Reply{}, invalid("status", errors.New("a comment, a reason or a duplicate goes with a status"))
	}
	if r.Title != nil || r.Body != nil {
		base, err := is.Show(ctx, id)
		if err != nil {
			return Reply{}, err
		}
		if r.Base != nil && len(base.Events) > 0 {
			base.Events[0].Body = *r.Base
		}
		if _, err := is.Edit(ctx, id, r.Title, r.Body, base); err != nil {
			return Reply{}, on("body", err)
		}
	}
	if r.Labels != nil {
		if _, err := is.ChangeLabels(ctx, id, r.Labels.Add, r.Labels.Remove); err != nil {
			return Reply{}, on("labels", err)
		}
	}
	if len(r.Set) > 0 {
		if _, err := is.Set(ctx, issues.SetReq{Name: id, Assign: r.Set, Force: r.Force}); err != nil {
			return Reply{}, on("set", err)
		}
	}
	switch r.Status {
	case "closed": // no reason is completed, as close's
		if _, err := is.Close(ctx, issues.CloseReq{Name: id, Comment: r.Comment, Reason: cmpString(r.Reason, "completed"), DuplicateOf: r.DuplicateOf}); err != nil {
			return Reply{}, on("status", err)
		}
	case "open":
		if _, err := is.Reopen(ctx, id, r.Comment); err != nil {
			return Reply{}, on("status", err)
		}
	}
	return thread(ctx, id, http.StatusOK)
}

type deleteReq struct {
	ID      string `path:"id"`
	IfMatch string `header:"If-Match"`
}

func deleteIssue(ctx context.Context, r deleteReq) (Reply, error) {
	ctx, id, err := expect(ctx, r.ID, r.IfMatch)
	if err != nil {
		return Reply{}, err
	}
	if _, err := board(ctx).Issues.Delete(ctx, id); err != nil {
		return Reply{}, err
	}
	return Reply{Status: http.StatusNoContent}, nil
}

type logReq struct {
	ID    string `path:"id"`
	Field string `query:"field"`
	Since string `query:"since"`
}

func issueLog(ctx context.Context, r logReq) (items[issues.LogEntry], error) {
	req := issues.LogReq{Name: r.ID, Field: r.Field}
	if r.Since != "" {
		t, err := parseAt(r.Since)
		if err != nil {
			return items[issues.LogEntry]{}, invalid("since", err)
		}
		req.Since = t
	}
	log, _, err := board(ctx).Issues.Log(ctx, req)
	if log == nil {
		log = []issues.LogEntry{}
	}
	return items[issues.LogEntry]{Items: log}, err
}

type commentReq struct {
	ID      string `path:"id"`
	IfMatch string `header:"If-Match"`
	Body    string `json:"body"`
}

func addComment(ctx context.Context, r commentReq) (Reply, error) {
	if strings.TrimSpace(r.Body) == "" {
		return Reply{}, invalid("body", errors.New("a comment needs a text"))
	}
	ctx, id, err := expect(ctx, r.ID, r.IfMatch)
	if err != nil {
		return Reply{}, err
	}
	op, _, err := board(ctx).Issues.Comment(ctx, id, r.Body)
	if err != nil {
		return Reply{}, err
	}
	rep, err := thread(ctx, id, http.StatusCreated)
	rep.Location = "/api/issues/" + id + "/comments/" + string(op)
	return rep, err
}

type editCommentReq struct {
	ID      string  `path:"id"`
	Comment string  `path:"comment"`
	IfMatch string  `header:"If-Match"`
	Body    string  `json:"body"`
	Base    *string `json:"base"`
}

func editComment(ctx context.Context, r editCommentReq) (Reply, error) {
	ctx, id, err := expect(ctx, r.ID, r.IfMatch)
	if err != nil {
		return Reply{}, err
	}
	is := board(ctx).Issues
	base := ""
	if r.Base != nil {
		base = *r.Base
	} else { // no base: the text as it is now
		th, err := is.Show(ctx, id)
		if err != nil {
			return Reply{}, err
		}
		for _, e := range th.Events {
			if e.ID == r.Comment {
				base = e.Body
			}
		}
	}
	if err := is.EditText(ctx, id, kernel.OpID(r.Comment), r.Body, base); err != nil {
		return Reply{}, on("body", err)
	}
	return thread(ctx, id, http.StatusOK)
}

type deleteCommentReq struct {
	ID      string `path:"id"`
	Comment string `path:"comment"`
	IfMatch string `header:"If-Match"`
}

func deleteComment(ctx context.Context, r deleteCommentReq) (Reply, error) {
	ctx, id, err := expect(ctx, r.ID, r.IfMatch)
	if err != nil {
		return Reply{}, err
	}
	if err := board(ctx).Issues.DeleteComment(ctx, id, kernel.OpID(r.Comment)); err != nil {
		return Reply{}, err
	}
	return thread(ctx, id, http.StatusOK)
}

type undoReq struct {
	ID    string `path:"id"`
	Force bool   `json:"force"`
}

func undo(ctx context.Context, r undoReq) (issues.Undone, error) {
	return board(ctx).Issues.Undo(ctx, r.ID, r.Force)
}

type followReq struct {
	ID     string `path:"id"`
	Follow *bool  `json:"follow"`
}

func following(ctx context.Context, r followReq) (issues.FollowState, error) {
	return board(ctx).Issues.Following(ctx, r.ID, "")
}

func follow(ctx context.Context, r followReq) (issues.FollowState, error) {
	_, f, err := board(ctx).Issues.Follow(ctx, r.ID, r.Follow, "")
	return f, err
}

// errUsage is a request the domain refuses as written.
func errUsage(err error) error { return &app.Error{Class: app.Usage, Err: err} }
