package github

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

// hub serves a fake forge as GitHub's API: the REST endpoints the adapter
// uses, and its one GraphQL query. A request acts as its token's account,
// so the fake records who wrote. A confidential issue of the fake is served
// as a pull request, the one thing on GitHub that sync never reads.
type hub struct {
	f   *fake.Forge
	url string

	mu     sync.Mutex
	labels map[string]bool          // labels made through the API, in lower case
	users  map[string]forge.Account // the tokens' accounts, by login
	repeat bool                     // a listing's later pages start with the last item of the page before
	faults map[string][]http.HandlerFunc
	hits   map[string]int // requests, by "METHOD path"
}

func newHub(t *testing.T, f *fake.Forge) *hub {
	h := &hub{f: f, labels: map[string]bool{}, users: map[string]forge.Account{},
		faults: map[string][]http.HandlerFunc{}, hits: map[string]int{}}
	mux := http.NewServeMux()
	for pattern, fn := range map[string]func(http.ResponseWriter, *http.Request, forge.Forge){
		"GET /user":                                      h.user,
		"GET /repos/{o}/{r}":                             h.repo,
		"GET /repos/{o}/{r}/issues":                      h.list,
		"POST /repos/{o}/{r}/issues":                     h.create,
		"GET /repos/{o}/{r}/issues/{n}":                  h.get,
		"PATCH /repos/{o}/{r}/issues/{n}":                h.patch,
		"GET /repos/{o}/{r}/issues/{n}/comments":         h.comments,
		"POST /repos/{o}/{r}/issues/{n}/comments":        h.comment,
		"PATCH /repos/{o}/{r}/issues/comments/{id}":      h.editComment,
		"GET /repos/{o}/{r}/labels/{name}":               h.getLabel,
		"POST /repos/{o}/{r}/labels":                     h.makeLabel,
		"POST /repos/{o}/{r}/issues/{n}/labels":          h.addLabels,
		"DELETE /repos/{o}/{r}/issues/{n}/labels/{name}": h.removeLabel,
		"POST /graphql":                                  h.graphql,
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			ff, ok := h.auth(r)
			if !ok {
				write(w, http.StatusUnauthorized, map[string]string{"message": "Bad credentials"})
				return
			}
			fn(w, r, ff)
		})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		h.mu.Lock()
		h.hits[key]++
		var fault http.HandlerFunc
		if q := h.faults[key]; len(q) > 0 {
			fault, h.faults[key] = q[0], q[1:]
		}
		h.mu.Unlock()
		if fault != nil {
			fault(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	h.url = srv.URL
	return h
}

// adapter is the adapter over h. An account's token names it; the hub takes
// it when the fake granted it.
func (h *hub) adapter(t *testing.T) *Forge {
	t.Helper()
	host, rest, _ := strings.Cut(h.f.Key(), "/")
	a, err := New(t.Context(), Options{Host: host, Project: "o/" + rest, API: h.url,
		Token: func(_ context.Context, a forge.Account) (string, error) {
			if a.ID == "" {
				a = forge.Account{ID: "1", Login: "board"} // the fake's posting account
			}
			return "t-" + a.ID + "-" + a.Login, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	a.s.sleep = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	return a
}

// fault answers the next requests to "METHOD path" with these, in order.
func (h *hub) fault(key string, answers ...http.HandlerFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.faults[key] = append(h.faults[key], answers...)
}

func (h *hub) auth(r *http.Request) (forge.Forge, bool) {
	tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer t-")
	id, login, _ := strings.Cut(tok, "-")
	a := forge.Account{ID: id, Login: login}
	ff, err := h.f.As(r.Context(), a)
	if err != nil {
		return nil, false
	}
	h.mu.Lock()
	h.users[strings.ToLower(login)] = a
	h.mu.Unlock()
	return ff, true
}

func (h *hub) user(w http.ResponseWriter, r *http.Request, ff forge.Forge) {
	me, _ := ff.Me(r.Context())
	write(w, http.StatusOK, userJSON(me))
}

func (h *hub) repo(w http.ResponseWriter, r *http.Request, ff forge.Forge) {
	vis, _ := ff.Visibility(r.Context())
	write(w, http.StatusOK, map[string]any{"private": vis != forge.Public, "visibility": vis})
}

func (h *hub) list(w http.ResponseWriter, r *http.Request, _ forge.Forge) {
	var since int64
	if s := r.URL.Query().Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			write(w, http.StatusUnprocessableEntity, map[string]string{"message": "bad since"})
			return
		}
		since = t.Unix()
	}
	items := []any{}
	for _, is := range h.f.All() {
		if is.Updated >= since {
			items = append(items, h.issueJSON(r.Context(), is))
		}
	}
	h.page(w, r, items)
}

func (h *hub) create(w http.ResponseWriter, r *http.Request, ff forge.Forge) {
	var in struct{ Title, Body string }
	if !read(w, r, &in) {
		return
	}
	is, err := ff.CreateIssue(r.Context(), in.Title, in.Body)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusCreated, h.issueJSON(r.Context(), is))
}

// issue is the issue the path's number names.
func (h *hub) issue(w http.ResponseWriter, r *http.Request) (forge.Issue, bool) {
	is, ok := h.f.Get(r.PathValue("n"))
	if !ok {
		write(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
	}
	return is, ok
}

func (h *hub) get(w http.ResponseWriter, r *http.Request, _ forge.Forge) {
	if is, ok := h.issue(w, r); ok {
		write(w, http.StatusOK, h.issueJSON(r.Context(), is))
	}
}

func (h *hub) patch(w http.ResponseWriter, r *http.Request, ff forge.Forge) {
	is, ok := h.issue(w, r)
	var in map[string]json.RawMessage
	if !ok || !read(w, r, &in) {
		return
	}
	ctx := r.Context()
	for k, v := range in {
		var s string
		var logins []string
		var err error
		switch k {
		case "title":
			_ = json.Unmarshal(v, &s)
			err = ff.SetTitle(ctx, is.Ref(), s)
		case "body":
			_ = json.Unmarshal(v, &s)
			err = ff.SetBody(ctx, is.Ref(), s)
		case "state":
			_ = json.Unmarshal(v, &s)
			err = ff.SetStatus(ctx, is.Ref(), s)
		case "assignees":
			_ = json.Unmarshal(v, &logins)
			_, err = ff.SetAssignees(ctx, is.Ref(), h.accounts(logins))
		}
		if err != nil {
			fail(w, err)
			return
		}
	}
	h.get(w, r, ff)
}

// accounts are the users with these logins, as the hub knows them: GitHub
// leaves out a login it can't assign.
func (h *hub) accounts(logins []string) []forge.Account {
	known := map[string]forge.Account{}
	for _, is := range h.f.All() {
		known[strings.ToLower(is.Author.Login)] = is.Author
		for _, a := range is.Assignees {
			known[strings.ToLower(a.Login)] = a
		}
		for _, c := range is.Comments {
			known[strings.ToLower(c.Author.Login)] = c.Author
		}
	}
	h.mu.Lock()
	for l, a := range h.users {
		known[l] = a
	}
	h.mu.Unlock()
	var out []forge.Account
	for _, l := range logins {
		if a, ok := known[strings.ToLower(l)]; ok {
			out = append(out, a)
		}
	}
	return out
}

func (h *hub) comments(w http.ResponseWriter, r *http.Request, _ forge.Forge) {
	is, ok := h.issue(w, r)
	if !ok {
		return
	}
	items := []any{}
	for _, c := range is.Comments {
		items = append(items, commentJSON(c))
	}
	h.page(w, r, items)
}

func (h *hub) comment(w http.ResponseWriter, r *http.Request, ff forge.Forge) {
	is, ok := h.issue(w, r)
	var in struct{ Body string }
	if !ok || !read(w, r, &in) {
		return
	}
	c, err := ff.CreateComment(r.Context(), is.Ref(), in.Body)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusCreated, commentJSON(c))
}

func (h *hub) editComment(w http.ResponseWriter, r *http.Request, ff forge.Forge) {
	var in struct{ Body string }
	if !read(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	for _, is := range h.f.All() {
		if slices.ContainsFunc(is.Comments, func(c forge.Comment) bool { return c.ID == id }) {
			if err := ff.EditComment(r.Context(), is.Ref(), id, in.Body); err != nil {
				fail(w, err)
				return
			}
			got, _ := h.f.Get(is.Number)
			write(w, http.StatusOK, commentJSON(got.Comments[slices.IndexFunc(got.Comments, func(c forge.Comment) bool { return c.ID == id })]))
			return
		}
	}
	write(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
}

// known says whether the project has the label: on an issue, or made.
func (h *hub) known(name string) bool {
	for _, is := range h.f.All() {
		if slices.ContainsFunc(is.Labels, func(l string) bool { return strings.EqualFold(l, name) }) {
			return true
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.labels[strings.ToLower(name)]
}

func (h *hub) getLabel(w http.ResponseWriter, r *http.Request, _ forge.Forge) {
	if name := r.PathValue("name"); h.known(name) {
		write(w, http.StatusOK, map[string]string{"name": name})
		return
	}
	write(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
}

func (h *hub) makeLabel(w http.ResponseWriter, r *http.Request, _ forge.Forge) {
	var in struct{ Name string }
	if !read(w, r, &in) {
		return
	}
	if h.known(in.Name) {
		write(w, http.StatusUnprocessableEntity, map[string]string{"message": "Validation Failed"})
		return
	}
	h.mu.Lock()
	h.labels[strings.ToLower(in.Name)] = true
	h.mu.Unlock()
	write(w, http.StatusCreated, map[string]string{"name": in.Name})
}

// addLabels takes only labels the project has, so the adapter must make
// them.
func (h *hub) addLabels(w http.ResponseWriter, r *http.Request, ff forge.Forge) {
	is, ok := h.issue(w, r)
	var in struct{ Labels []string }
	if !ok || !read(w, r, &in) {
		return
	}
	for _, l := range in.Labels {
		if !h.known(l) {
			write(w, http.StatusUnprocessableEntity, map[string]string{"message": "Label does not exist"})
			return
		}
	}
	// a label the fake refuses is left out of the answer, as GitHub does
	if err := ff.ChangeLabels(r.Context(), is.Ref(), in.Labels, nil); err != nil && !errors.As(err, new(*forge.Refused)) {
		fail(w, err)
		return
	}
	h.labelsOf(w, is.Number)
}

func (h *hub) removeLabel(w http.ResponseWriter, r *http.Request, ff forge.Forge) {
	is, ok := h.issue(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !slices.Contains(is.Labels, name) {
		write(w, http.StatusNotFound, map[string]string{"message": "Label does not exist"})
		return
	}
	if err := ff.ChangeLabels(r.Context(), is.Ref(), nil, []string{name}); err != nil {
		fail(w, err)
		return
	}
	h.labelsOf(w, is.Number)
}

func (h *hub) labelsOf(w http.ResponseWriter, number string) {
	is, _ := h.f.Get(number)
	write(w, http.StatusOK, labelsJSON(is.Labels))
}

// graphql answers the adapter's one query, whatever its text, from its ids.
func (h *hub) graphql(w http.ResponseWriter, r *http.Request, _ forge.Forge) {
	var in struct {
		Variables struct {
			IDs []string `json:"ids"`
		} `json:"variables"`
	}
	if !read(w, r, &in) {
		return
	}
	all := h.f.All()
	nodes := []any{}
	for _, nid := range in.Variables.IDs {
		i := slices.IndexFunc(all, func(is forge.Issue) bool { return "I_"+is.ID == nid })
		if i < 0 {
			nodes = append(nodes, nil)
			continue
		}
		is := all[i]
		n := map[string]any{"id": nid, "lastEditedAt": nil, "renamed": eventsJSON(is.Changed["title"]), "reopened": eventsJSON(0)}
		if t := is.Changed["body"]; t != 0 {
			n["lastEditedAt"] = ts(t)
		}
		if is.Status == "open" {
			n["reopened"] = eventsJSON(is.Changed["status"])
		}
		nodes = append(nodes, n)
	}
	write(w, http.StatusOK, map[string]any{"data": map[string]any{"nodes": nodes}})
}

func eventsJSON(t int64) map[string]any {
	nodes := []any{}
	if t != 0 {
		nodes = append(nodes, map[string]any{"createdAt": ts(t)})
	}
	return map[string]any{"nodes": nodes}
}

func (h *hub) issueJSON(ctx context.Context, is forge.Issue) map[string]any {
	assignees := []any{}
	for _, a := range is.Assignees {
		assignees = append(assignees, userJSON(a))
	}
	m := map[string]any{
		"id": json.Number(is.ID), "node_id": "I_" + is.ID, "number": json.Number(is.Number), "html_url": is.URL,
		"title": is.Title, "body": is.Body, "state": is.Status, "user": userJSON(is.Author),
		"labels": labelsJSON(is.Labels), "assignees": assignees, "comments": len(is.Comments),
		"created_at": ts(is.Created), "updated_at": ts(is.Updated), "closed_at": nil,
	}
	if is.Status == "closed" {
		m["closed_at"] = ts(cmp.Or(is.Changed["status"], is.Created))
	}
	if _, err := h.f.Issue(ctx, is.Ref()); errors.Is(err, forge.ErrConfidential) {
		m["pull_request"] = map[string]any{"url": is.URL}
	}
	return m
}

func commentJSON(c forge.Comment) map[string]any {
	return map[string]any{"id": json.Number(c.ID), "body": c.Body, "user": userJSON(c.Author),
		"created_at": ts(c.Time), "updated_at": ts(max(c.Edited, c.Time))}
}

func userJSON(a forge.Account) map[string]any {
	return map[string]any{"id": json.Number(a.ID), "login": a.Login}
}

func labelsJSON(ls []string) []any {
	out := []any{}
	for _, l := range ls {
		out = append(out, map[string]string{"name": l})
	}
	return out
}

func ts(t int64) string { return time.Unix(t, 0).UTC().Format(time.RFC3339) }

// page serves one page of a listing, by per_page and page.
func (h *hub) page(w http.ResponseWriter, r *http.Request, items []any) {
	per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	per = cmp.Or(per, 30)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	lo := (max(page, 1) - 1) * per
	h.mu.Lock()
	if h.repeat && lo > 0 {
		lo--
	}
	h.mu.Unlock()
	lo = min(lo, len(items))
	write(w, http.StatusOK, items[lo:min(lo+per, len(items))])
}

func read(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		write(w, http.StatusBadRequest, map[string]string{"message": "Problems parsing JSON"})
		return false
	}
	return true
}

// fail answers a fake's error as GitHub would. A write that landed, then
// broke, is a 502.
func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, forge.ErrNotFound), errors.Is(err, forge.ErrConfidential):
		write(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
	case errors.Is(err, forge.ErrRefused):
		write(w, http.StatusForbidden, map[string]string{"message": "Must have push access to repository"})
	case errors.Is(err, fake.ErrCrash):
		write(w, http.StatusBadGateway, map[string]string{"message": "Bad Gateway"})
	default:
		write(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
	}
}

func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// A redirect, to another host here, isn't followed, so the token stays
// home. It reads as not found.
func TestRedirectKeepsTheToken(t *testing.T) {
	t.Parallel()
	got := make(chan string, 1)
	away := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("Authorization")
		w.Write([]byte(`{"id":1,"login":"x"}`))
	}))
	defer away.Close()
	_, port, _ := strings.Cut(away.Listener.Addr().String(), ":")
	home := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://localhost:"+port+"/steal", http.StatusFound) // localhost, not 127.0.0.1
	}))
	defer home.Close()
	a, err := New(t.Context(), Options{Project: "o/r", API: home.URL,
		Token: func(context.Context, forge.Account) (string, error) { return "SECRET-TOKEN", nil }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Me(t.Context()); !errors.Is(err, forge.ErrNotFound) {
		t.Errorf("err = %v, want not found", err)
	}
	select {
	case tok := <-got:
		t.Errorf("redirect followed, Authorization %q", tok)
	default:
	}
}

// The repository's visibility; internal is an Enterprise's.
func TestVisibility(t *testing.T) {
	t.Parallel()
	for _, v := range []forge.Visibility{forge.Public, forge.Internal, forge.Private} {
		f := fake.New("github.com/r")
		f.SetVisibility(v)
		if got, err := newHub(t, f).adapter(t).Visibility(t.Context()); got != v || err != nil {
			t.Errorf("%s: %s, %v", v, got, err)
		}
	}
}
