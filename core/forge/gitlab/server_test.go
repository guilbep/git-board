package gitlab

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
)

// server serves a fake forge as GitLab's REST API v4, as the project's
// members see it: confidential issues listed, and taking writes.
type server struct {
	f          *fake.Forge
	path       string                                        // the project's path
	perPage    int                                           // the most a page holds, 0 for GitLab's 100
	overlap    bool                                          // each page after the first repeats the last item of the one before
	visibility string                                        // the project's, when the fake's public or private won't do
	extra      map[string][]map[string]any                   // more notes, by issue number
	before     func(http.ResponseWriter, *http.Request) bool // answers instead of the server when true

	mu     sync.Mutex
	tokens map[string]forge.Account
}

// serve gives the adapter over f, served by a new server for the project at
// path, which opts set up first. Its tokens are the fake's.
func serve(t *testing.T, f *fake.Forge, path string, opts ...func(*server)) (*server, *Forge) {
	t.Helper()
	s := &server{f: f, path: path, perPage: 2, tokens: map[string]forge.Account{}, extra: map[string][]map[string]any{}}
	for _, o := range opts {
		o(s)
	}
	srv := httptest.NewServer(s.routes())
	t.Cleanup(srv.Close)
	p, err := newProject(srv.URL+"/", path)
	if err != nil {
		t.Fatal(err)
	}
	p.http, p.pace = srv.Client(), 0
	p.sleep = func(context.Context, time.Duration) error { return nil }
	p.cred = func(ctx context.Context, a forge.Account) (string, error) {
		if _, err := f.As(ctx, a); err != nil {
			return "", err
		}
		return s.grant(a), nil
	}
	me, _ := f.Me(t.Context())
	return s, &Forge{p: p, token: s.grant(me)}
}

func (s *server) grant(a forge.Account) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	token := "tok-" + a.ID
	s.tokens[token] = a
	return token
}

func (s *server) routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		if a, ok := s.account(r); ok {
			reply(w, 200, userJSON(a))
		} else {
			reply(w, 401, map[string]string{"message": "401 Unauthorized"})
		}
	})
	p := "/api/v4/projects/{project}"
	s.handle(m, "GET "+p, s.project)
	s.handle(m, "GET "+p+"/issues", s.list)
	s.handle(m, "POST "+p+"/issues", s.create)
	s.handle(m, "GET "+p+"/issues/{iid}", s.issue)
	s.handle(m, "PUT "+p+"/issues/{iid}", s.update)
	s.handle(m, "GET "+p+"/issues/{iid}/notes", s.notes)
	s.handle(m, "POST "+p+"/issues/{iid}/notes", s.comment)
	s.handle(m, "PUT "+p+"/issues/{iid}/notes/{note}", s.edit)
	s.handle(m, "GET "+p+"/issues/{iid}/resource_state_events", s.states)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.before == nil || !s.before(w, r) {
			m.ServeHTTP(w, r)
		}
	})
}

// handle serves the project at s.path to a known token, through its
// account's view of the fake.
func (s *server) handle(m *http.ServeMux, pattern string, h func(http.ResponseWriter, *http.Request, forge.Forge)) {
	m.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		a, ok := s.account(r)
		if !ok {
			reply(w, 401, map[string]string{"message": "401 Unauthorized"})
			return
		}
		if r.PathValue("project") != s.path {
			reply(w, 404, map[string]string{"message": "404 Project Not Found"})
			return
		}
		view, err := s.f.As(r.Context(), a)
		if err != nil {
			reply(w, 401, map[string]string{"message": "401 Unauthorized"})
			return
		}
		h(w, r, view)
	})
}

func (s *server) account(r *http.Request) (forge.Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.tokens[r.Header.Get("PRIVATE-TOKEN")]
	return a, ok
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// fail answers a fake's error as GitLab would. A crash breaks the
// connection, after the write landed.
func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, fake.ErrCrash):
		if conn, _, herr := w.(http.Hijacker).Hijack(); herr == nil {
			_ = conn.Close()
		}
	case errors.Is(err, forge.ErrNotFound), errors.Is(err, forge.ErrConfidential):
		reply(w, 404, map[string]string{"message": "404 Not found"})
	case errors.Is(err, forge.ErrRefused):
		reply(w, 403, map[string]string{"message": "403 Forbidden"})
	default:
		reply(w, 400, map[string]string{"message": err.Error()})
	}
}

func ts(t int64) string { return time.Unix(t, 0).UTC().Format("2006-01-02T15:04:05.000Z") }

func num(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func userJSON(a forge.Account) map[string]any {
	return map[string]any{"id": num(a.ID), "username": a.Login}
}

func (s *server) issueJSON(r *http.Request, is forge.Issue) map[string]any {
	_, err := s.f.Issue(r.Context(), is.ID)
	state := map[string]string{"open": "opened"}[is.Status]
	if state == "" {
		state = is.Status
	}
	assignees := []any{}
	for _, a := range is.Assignees {
		assignees = append(assignees, userJSON(a))
	}
	return map[string]any{"id": num(is.ID), "iid": num(is.Number), "title": is.Title, "description": is.Body,
		"state": state, "labels": append([]string{}, is.Labels...), "assignees": assignees, "author": userJSON(is.Author),
		"confidential": errors.Is(err, forge.ErrConfidential), "created_at": ts(is.Created), "updated_at": ts(is.Updated),
		"web_url": is.URL}
}

// page answers one page of items, as the request asks, with X-Next-Page.
func (s *server) page(w http.ResponseWriter, r *http.Request, items []any) {
	per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if per <= 0 {
		per = 20
	}
	if s.perPage > 0 {
		per = min(per, s.perPage)
	}
	n, _ := strconv.Atoi(r.URL.Query().Get("page"))
	n = max(n, 1)
	start := min((n-1)*per, len(items))
	end := min(start+per, len(items))
	if end < len(items) {
		w.Header().Set("X-Next-Page", strconv.Itoa(n+1))
	}
	if s.overlap && start > 0 {
		start--
	}
	reply(w, 200, items[start:end])
}

func (s *server) project(w http.ResponseWriter, r *http.Request, view forge.Forge) {
	vis := s.visibility
	if vis == "" {
		v, _ := view.Visibility(r.Context())
		vis = string(v)
	}
	reply(w, 200, map[string]any{"id": 1, "path_with_namespace": s.path, "visibility": vis})
}

func (s *server) list(w http.ResponseWriter, r *http.Request, _ forge.Forge) {
	var after int64
	if v := r.URL.Query().Get("updated_after"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			reply(w, 400, map[string]string{"error": "updated_after is invalid"})
			return
		}
		after = t.Unix()
	}
	var items []any
	for _, is := range s.f.All() {
		if is.Updated >= after {
			items = append(items, s.issueJSON(r, is))
		}
	}
	s.page(w, r, items)
}

func (s *server) create(w http.ResponseWriter, r *http.Request, view forge.Forge) {
	var in struct{ Title, Description string }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		reply(w, 400, map[string]string{"message": err.Error()})
		return
	}
	is, err := view.CreateIssue(r.Context(), in.Title, in.Description)
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, 201, s.issueJSON(r, is))
}

func (s *server) get(w http.ResponseWriter, r *http.Request) (forge.Issue, bool) {
	is, ok := s.f.Get(r.PathValue("iid"))
	if !ok {
		reply(w, 404, map[string]string{"message": "404 Not found"})
	}
	return is, ok
}

func (s *server) issue(w http.ResponseWriter, r *http.Request, _ forge.Forge) {
	if is, ok := s.get(w, r); ok {
		reply(w, 200, s.issueJSON(r, is))
	}
}

func (s *server) update(w http.ResponseWriter, r *http.Request, view forge.Forge) {
	is, ok := s.get(w, r)
	if !ok {
		return
	}
	var in struct {
		Title        *string
		Description  *string
		StateEvent   string  `json:"state_event"`
		AddLabels    string  `json:"add_labels"`
		RemoveLabels string  `json:"remove_labels"`
		AssigneeIDs  []int64 `json:"assignee_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		reply(w, 400, map[string]string{"message": err.Error()})
		return
	}
	ctx, id := r.Context(), is.ID
	var err error
	split := func(s string) []string {
		if s == "" {
			return nil
		}
		return strings.Split(s, ",")
	}
	switch {
	case in.Title != nil:
		err = view.SetTitle(ctx, id, *in.Title)
	case in.Description != nil:
		err = view.SetBody(ctx, id, *in.Description)
	case in.StateEvent != "":
		err = view.SetStatus(ctx, id, map[string]string{"close": "closed", "reopen": "open"}[in.StateEvent])
	case in.AddLabels != "" || in.RemoveLabels != "":
		// a label the fake refuses is left out of the answer
		if err = view.ChangeLabels(ctx, id, split(in.AddLabels), split(in.RemoveLabels)); errors.As(err, new(*forge.Refused)) {
			err = nil
		}
	case in.AssigneeIDs != nil:
		var users []forge.Account
		for _, n := range in.AssigneeIDs {
			if n != 0 {
				users = append(users, s.user(strconv.FormatInt(n, 10)))
			}
		}
		_, err = view.SetAssignees(ctx, id, users)
	}
	if err != nil {
		fail(w, err)
		return
	}
	is, _ = s.f.Get(is.Number)
	reply(w, 200, s.issueJSON(r, is))
}

// user is the account with this id, as the fake's issues and tokens know
// it.
func (s *server) user(id string) forge.Account {
	s.mu.Lock()
	for _, a := range s.tokens {
		if a.ID == id {
			s.mu.Unlock()
			return a
		}
	}
	s.mu.Unlock()
	for _, is := range s.f.All() {
		known := append([]forge.Account{is.Author}, is.Assignees...)
		for _, c := range is.Comments {
			known = append(known, c.Author)
		}
		for _, a := range known {
			if a.ID == id {
				return a
			}
		}
	}
	return forge.Account{ID: id}
}

func noteJSON(c forge.Comment) map[string]any {
	return map[string]any{"id": num(c.ID), "body": c.Body, "author": userJSON(c.Author), "system": false, "internal": false,
		"created_at": ts(c.Time), "updated_at": ts(max(c.Time, c.Edited))}
}

// notes are the issue's comments, then the system notes its change times
// make, and s.extra's, by creation time, newest first unless sort=asc.
func (s *server) notes(w http.ResponseWriter, r *http.Request, _ forge.Forge) {
	is, ok := s.get(w, r)
	if !ok {
		return
	}
	var notes []map[string]any
	for _, c := range is.Comments {
		notes = append(notes, noteJSON(c))
	}
	for field, body := range map[string]string{"title": "changed title from **x** to **" + is.Title + "**", "body": "changed the description"} {
		if t := is.Changed[field]; t != 0 {
			id := 9_000_000 + 10*num(is.Number) + int64(len(field))
			notes = append(notes, map[string]any{"id": id, "body": body, "author": userJSON(is.Author), "system": true,
				"created_at": ts(t), "updated_at": ts(t)})
		}
	}
	notes = append(notes, s.extra[is.Number]...)
	slices.SortStableFunc(notes, func(a, b map[string]any) int {
		return cmp.Or(cmp.Compare(a["created_at"].(string), b["created_at"].(string)), cmp.Compare(noteID(a), noteID(b)))
	})
	if r.URL.Query().Get("sort") != "asc" {
		slices.Reverse(notes)
	}
	items := make([]any, len(notes))
	for i, n := range notes {
		items[i] = n
	}
	s.page(w, r, items)
}

func noteID(n map[string]any) int64 {
	switch v := n["id"].(type) {
	case int:
		return int64(v)
	case int64:
		return v
	}
	return 0
}

func (s *server) comment(w http.ResponseWriter, r *http.Request, view forge.Forge) {
	is, ok := s.get(w, r)
	if !ok {
		return
	}
	var in struct{ Body string }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		reply(w, 400, map[string]string{"message": err.Error()})
		return
	}
	c, err := view.CreateComment(r.Context(), is.ID, in.Body)
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, 201, noteJSON(c))
}

func (s *server) edit(w http.ResponseWriter, r *http.Request, view forge.Forge) {
	is, ok := s.get(w, r)
	if !ok {
		return
	}
	var in struct{ Body string }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		reply(w, 400, map[string]string{"message": err.Error()})
		return
	}
	if err := view.EditComment(r.Context(), is.ID, r.PathValue("note"), in.Body); err != nil {
		fail(w, err)
		return
	}
	is, _ = s.f.Get(is.Number)
	for _, c := range is.Comments {
		if c.ID == r.PathValue("note") {
			reply(w, 200, noteJSON(c))
		}
	}
}

// states is the issue's last state change, as a state event.
func (s *server) states(w http.ResponseWriter, r *http.Request, _ forge.Forge) {
	is, ok := s.get(w, r)
	if !ok {
		return
	}
	items := []any{}
	if t := is.Changed["status"]; t != 0 {
		state := map[string]string{"closed": "closed", "open": "reopened"}[is.Status]
		items = append(items, map[string]any{"id": 8_000_000 + num(is.Number), "state": state, "created_at": ts(t)})
	}
	s.page(w, r, items)
}
