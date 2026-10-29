package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/conformance"
	"github.com/guilbep/git-board/core/forge/fake"
)

var (
	ann = forge.Account{ID: "7", Login: "ann"}
	bob = forge.Account{ID: "8", Login: "bob"}
)

func TestConformance(t *testing.T) {
	t.Parallel()
	conformance.Run(t, func(t *testing.T, f *fake.Forge) forge.Forge { return newHub(t, f).adapter(t) })
}

// TestListing reads past a page, each issue and comment once though pages
// repeat one, without pull requests, and with web edits' \r\n undone.
func TestListing(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	f := fake.New("example.com/p")
	for i := range 250 {
		f.Add(forge.Issue{Author: ann, Title: "i" + strconv.Itoa(i)})
	}
	var cs []forge.Comment
	for i := range 150 {
		cs = append(cs, forge.Comment{Author: bob, Body: "c" + strconv.Itoa(i)})
	}
	cs[0].Body = "one\r\ntwo"
	many := f.Add(forge.Issue{Author: ann, Title: "many", Body: "a\r\nb", Comments: cs})
	pr := f.Add(forge.Issue{Author: ann, Title: "a pull request"})
	f.SetConfidential(pr.Number, true) // served as a pull request
	h := newHub(t, f)
	h.repeat = true

	a := h.adapter(t)
	list, err := a.Issues(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 251 {
		t.Fatalf("%d issues, want 251", len(list))
	}
	if !list[250].Partial || list[250].Comments != nil || list[0].Partial {
		t.Fatalf("the listing reads comments: %+v", list[250])
	}
	got, err := a.Complete(ctx, list[250])
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "many" || got.Body != "a\nb" || len(got.Comments) != 150 || got.Comments[0].Body != "one\ntwo" {
		t.Fatalf("many = %q %q, %d comments, first %q", got.Title, got.Body, len(got.Comments), got.Comments[0].Body)
	}
	seen := map[string]bool{}
	for _, c := range got.Comments {
		if seen[c.ID] {
			t.Fatalf("comment %s twice", c.ID)
		}
		seen[c.ID] = true
	}

	// A fresh adapter finds an id of an earlier run by listing every issue.
	b := h.adapter(t)
	if is, err := b.Issue(ctx, many.Ref()); err != nil || is.Number != many.Number || len(is.Comments) != 150 {
		t.Fatalf("Issue(many) = %s, %d comments, %v", is.Number, len(is.Comments), err)
	}
	if _, err := b.Issue(ctx, pr.Ref()); !errors.Is(err, forge.ErrConfidential) {
		t.Fatalf("a pull request: %v, want ErrConfidential", err)
	}
	if _, err := b.Issue(ctx, forge.Ref{ID: "424242", Number: "424242"}); !errors.Is(err, forge.ErrNotFound) {
		t.Fatalf("no such id: %v, want ErrNotFound", err)
	}
}

// TestSince lists from a minute before since, as GitHub's clock isn't ours.
func TestSince(t *testing.T) {
	t.Parallel()
	f := fake.New("example.com/p")
	f.Add(forge.Issue{Author: ann, Title: "old", Created: 1000})
	f.Add(forge.Issue{Author: ann, Title: "just before", Created: 1950})
	f.Add(forge.Issue{Author: ann, Title: "after", Created: 2100})
	list, err := newHub(t, f).adapter(t).Issues(t.Context(), 2000)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, is := range list {
		titles = append(titles, is.Title)
	}
	if !slices.Equal(titles, []string{"just before", "after"}) {
		t.Fatalf("listed %q", titles)
	}
}

func answer(code int, header map[string]string, msg string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range header {
			w.Header().Set(k, v)
		}
		write(w, code, map[string]string{"message": msg})
	}
}

// TestRetries: a rate limit is waited out, three times at most; a read is
// tried again after a 5xx, a write never is; a refusal is never retried.
func TestRetries(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	f := fake.New("example.com/p")
	is := f.Add(forge.Issue{Author: ann, Title: "T"})
	h := newHub(t, f)
	a := h.adapter(t)
	clock := time.Unix(1_000_000, 0)
	var slept []time.Duration
	a.s.now = func() time.Time { return clock }
	a.s.sleep = func(_ context.Context, d time.Duration) error {
		if d > 0 {
			slept = append(slept, d)
			clock = clock.Add(d)
		}
		return nil
	}
	if _, err := a.Issues(ctx, 0); err != nil { // the number of is
		t.Fatal(err)
	}
	path := "/repos/o/p/issues/" + is.Number
	sleeps := func() []time.Duration {
		s := slept
		slept = nil
		return s
	}
	hits := func(key string) int {
		h.mu.Lock()
		defer h.mu.Unlock()
		n := h.hits[key]
		h.hits[key] = 0
		return n
	}
	sleeps()

	h.fault("GET "+path, answer(502, nil, "Bad Gateway"),
		answer(403, nil, "You have exceeded a secondary rate limit"),
		answer(429, map[string]string{"Retry-After": "7"}, ""))
	if _, err := a.Issue(ctx, is.Ref()); err != nil {
		t.Fatal(err)
	}
	if got, want := sleeps(), []time.Duration{time.Second, time.Minute, 7 * time.Second}; !slices.Equal(got, want) || hits("GET "+path) != 4 {
		t.Fatalf("slept %v, want %v", got, want)
	}

	reset := strconv.FormatInt(clock.Unix()+30, 10)
	h.fault("PATCH "+path, answer(403, map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": reset}, "API rate limit exceeded"))
	if err := a.SetTitle(ctx, is.Ref(), "T2"); err != nil {
		t.Fatal(err)
	}
	if got := sleeps(); !slices.Equal(got, []time.Duration{31 * time.Second}) || hits("PATCH "+path) != 2 {
		t.Fatalf("slept %v, want 31s, until the reset", got)
	}
	if got, _ := f.Get(is.Number); got.Title != "T2" {
		t.Fatalf("title = %q", got.Title)
	}

	h.fault("POST "+path+"/comments", answer(502, nil, "Bad Gateway"))
	if _, err := a.CreateComment(ctx, is.Ref(), "hi"); err == nil || errors.Is(err, forge.ErrRefused) || hits("POST "+path+"/comments") != 1 {
		t.Fatalf("a write on a 502: %v, want one try and an error", err)
	}

	for _, code := range []int{403, 422} {
		h.fault("PATCH "+path, answer(code, nil, "Must have admin rights"))
		if err := a.SetTitle(ctx, is.Ref(), "T3"); !errors.Is(err, forge.ErrRefused) || hits("PATCH "+path) != 1 {
			t.Fatalf("%d: %v, want one try, refused", code, err)
		}
	}
	h.fault("PATCH "+path, answer(410, nil, "This issue was deleted"))
	if err := a.SetTitle(ctx, is.Ref(), "T3"); !errors.Is(err, forge.ErrNotFound) {
		t.Fatalf("410: %v, want not found", err)
	}

	limited := answer(429, map[string]string{"Retry-After": "1"}, "")
	h.fault("GET "+path, limited, limited, limited, limited)
	if _, err := a.Issue(ctx, is.Ref()); err == nil || errors.Is(err, forge.ErrRefused) || hits("GET "+path) != 4 {
		t.Fatalf("still limited: %v, want an error after three retries", err)
	}
}

// hangUp drops the connection mid-answer, as a Wi-Fi gone or a laptop
// asleep does: after the status line, where Go's client doesn't try again
// by itself.
func hangUp(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Length", "100")
	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()
	panic(http.ErrAbortHandler)
}

// A read waits out an outage, a dropped connection or a 5xx, in waits
// doubling to 30 s, each told, for 5 minutes at most; then it gives up.
func TestAReadWaitsOutAnOutage(t *testing.T) {
	t.Parallel()
	f := fake.New("example.com/p")
	is := f.Add(forge.Issue{Author: ann, Title: "T"})
	h := newHub(t, f)
	a := h.adapter(t)
	var slept []time.Duration
	a.s.sleep = func(_ context.Context, d time.Duration) error {
		if d > 0 {
			slept = append(slept, d)
		}
		return nil
	}
	var heard []string
	ctx := forge.WithWaits(t.Context(), func(w forge.Wait) { heard = append(heard, fmt.Sprint(w.Forge, " ", w.Why, " ", w.For)) })
	path := "GET /repos/o/p/issues/" + is.Number
	h.fault(path, hangUp, answer(502, nil, "Bad Gateway"), hangUp, hangUp, hangUp, hangUp, hangUp)
	if _, err := a.Issue(ctx, is.Ref()); err != nil {
		t.Fatal(err)
	}
	s := time.Second
	if want := []time.Duration{s, 2 * s, 4 * s, 8 * s, 16 * s, 30 * s, 30 * s}; !slices.Equal(slept, want) {
		t.Fatalf("slept %v, want %v", slept, want)
	}
	if len(heard) != 7 || heard[0] != a.Key()+" GitHub doesn't answer 1s" || heard[1] != a.Key()+" GitHub answers 502 2s" {
		t.Fatalf("heard %q", heard)
	}

	slept = nil
	for range 20 {
		h.fault(path, hangUp)
	}
	if _, err := a.Issue(ctx, is.Ref()); err == nil {
		t.Fatal("a read still down after 5 minutes didn't give up")
	}
	var total time.Duration
	for _, d := range slept {
		total += d
	}
	if total > 5*time.Minute || total < 4*time.Minute {
		t.Fatalf("waited %v in all (%v), want about 5 minutes", total, slept)
	}
}

// A write isn't tried again after a dropped connection: GitHub may have
// made it.
func TestAWriteIsntRetried(t *testing.T) {
	t.Parallel()
	f := fake.New("example.com/p")
	is := f.Add(forge.Issue{Author: ann, Title: "T"})
	h := newHub(t, f)
	a := h.adapter(t)
	key := "POST /repos/o/p/issues/" + is.Number + "/comments"
	h.fault(key, hangUp)
	if _, err := a.CreateComment(t.Context(), is.Ref(), "hi"); err == nil {
		t.Fatal("no error")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hits[key] != 1 {
		t.Fatalf("%d tries", h.hits[key])
	}
}

// A label GitHub leaves out is refused alone: the other adds and the
// removes still go.
func TestLabelRefusedAlone(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	f := fake.New("example.com/p")
	is := f.Add(forge.Issue{Author: ann, Title: "T", Labels: []string{"old"}})
	a := newHub(t, f).adapter(t)
	f.RefuseLabels("bad")
	err := a.ChangeLabels(ctx, is.Ref(), []string{"bad", "ui"}, []string{"old"})
	var some *forge.Refused
	if !errors.As(err, &some) || !slices.Equal(some.Values, []string{"bad"}) || !errors.Is(err, forge.ErrRefused) {
		t.Fatalf("err = %v, want bad refused", err)
	}
	if got, _ := f.Get(is.Number); !slices.Equal(got.Labels, []string{"ui"}) {
		t.Fatalf("labels = %v, want ui", got.Labels)
	}
}

// The repository's labels: listed past a page, created, edited by name,
// deleted, and counted with GraphQL.
func TestLabels(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path+" "+strings.TrimSpace(string(body)))
		mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/labels":
			var page []map[string]any
			if r.URL.Query().Get("page") == "1" {
				for i := range perPage {
					page = append(page, map[string]any{"id": i, "name": fmt.Sprint("l", i), "color": "ededed"})
				}
			} else {
				page = []map[string]any{{"id": 1000, "name": "bug", "color": "d73a4a", "description": "Something isn't working"}}
			}
			_ = json.NewEncoder(w).Encode(page)
		case r.URL.Path == "/graphql":
			_, _ = io.WriteString(w, `{"data":{"repository":{"label":{"issues":{"totalCount":2},"pullRequests":{"totalCount":1}}}}}`)
		}
	}))
	t.Cleanup(srv.Close)
	a, err := New(ctx, Options{Project: "o/r", API: srv.URL, Token: func(context.Context, forge.Account) (string, error) { return "t", nil }})
	if err != nil {
		t.Fatal(err)
	}
	ls, err := a.Labels(ctx)
	if err != nil || len(ls) != perPage+1 || ls[perPage] != (Label{Name: "bug", Color: "d73a4a", Description: "Something isn't working"}) {
		t.Fatalf("labels %d, last %+v, %v", len(ls), ls[len(ls)-1], err)
	}
	if err := a.SetLabel(ctx, Label{Name: "status:ready", Color: "00897f"}, false); err != nil {
		t.Fatal(err)
	}
	if err := a.SetLabel(ctx, Label{Name: "kind:feat", Color: "0e8a16", Description: "New"}, true); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteLabel(ctx, "wontfix"); err != nil {
		t.Fatal(err)
	}
	if n, err := a.LabelCount(ctx, "wontfix"); n != 3 || err != nil {
		t.Fatalf("count %d, %v", n, err)
	}
	want := []string{
		`POST /repos/o/r/labels {"color":"00897f","description":"","name":"status:ready"}`,
		`PATCH /repos/o/r/labels/kind:feat {"color":"0e8a16","description":"New"}`,
		`DELETE /repos/o/r/labels/wontfix `,
	}
	mu.Lock()
	defer mu.Unlock()
	if got := calls[2:5]; !slices.Equal(got, want) {
		t.Errorf("writes\n%q\nwant\n%q", got, want)
	}
}

// TestPace spaces writes a second apart, and keeps under 500 an hour.
func TestPace(t *testing.T) {
	t.Parallel()
	clock := time.Unix(1_000_000, 0)
	var slept []time.Duration
	s := &shared{now: func() time.Time { return clock }, sleep: func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		clock = clock.Add(d)
		return nil
	}}
	for range 3 {
		if err := s.pace(t.Context(), ""); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(slept, []time.Duration{0, time.Second, time.Second}) {
		t.Fatalf("slept %v", slept)
	}
	s.writes, slept = nil, nil
	for i := range perHour {
		s.writes = append(s.writes, clock.Add(-30*time.Minute+time.Duration(i)*time.Millisecond))
	}
	if err := s.pace(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(slept, []time.Duration{30 * time.Minute}) {
		t.Fatalf("the 501st write in an hour slept %v, want 30m", slept)
	}
}

// TestAs takes a token only when GitHub says it is the account's.
func TestAs(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	f := fake.New("example.com/p")
	f.Grant(bob)
	a := newHub(t, f).adapter(t)
	me, err := a.Me(ctx)
	if err != nil || me != (forge.Account{ID: "1", Login: "board"}) {
		t.Fatalf("me = %+v, %v", me, err)
	}
	if self, err := a.As(ctx, me); err != nil || self != forge.Forge(a) {
		t.Fatalf("As(me) = %v, %v", self, err)
	}
	if _, err := a.As(ctx, ann); !errors.Is(err, forge.ErrNoToken) {
		t.Fatalf("As(ann), not granted: %v", err)
	}
	b, err := a.As(ctx, bob)
	if err != nil {
		t.Fatal(err)
	}
	if who, _ := b.Me(ctx); who != bob {
		t.Fatalf("bob's view is %+v's", who)
	}
	a.s.tokenOf = func(context.Context, forge.Account) (string, error) { return "t-8-bob", nil }
	if _, err := a.As(ctx, ann); !errors.Is(err, forge.ErrNoToken) {
		t.Fatalf("As(ann) with bob's token: %v", err)
	}
}

// TestTokenLookup: the environment variable when set, else git's
// credential helper, asked for the login. It sets environment variables,
// so it stays serial.
func TestTokenLookup(t *testing.T) {
	ctx := t.Context()
	helper := filepath.Join(t.TempDir(), "helper")
	script := "#!/bin/sh\n[ \"$1\" = get ] || exit 0\ncase \"$(cat)\" in *username=ann*) echo password=from-helper;; esac\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// An empty helper first drops the machine's own, keychains included.
	for k, v := range map[string]string{"GIT_CONFIG_COUNT": "2",
		"GIT_CONFIG_KEY_0": "credential.helper", "GIT_CONFIG_VALUE_0": "",
		"GIT_CONFIG_KEY_1": "credential.helper", "GIT_CONFIG_VALUE_1": helper} {
		t.Setenv(k, v)
	}
	const ghe = "ghe.example.test"
	t.Setenv("GITHUB_TOKEN", "from-env")
	t.Setenv("GITHUB_ENTERPRISE_TOKEN", "")
	for _, c := range []struct{ host, login, want string }{
		{"github.com", "ann", "from-env"},
		{ghe, "ann", "from-helper"}, // github.com's token never goes to another host
		{ghe, "bob", ""},
	} {
		got, err := lookup(ctx, c.host, c.login)
		if got != c.want || (c.want == "") != errors.Is(err, forge.ErrNoToken) {
			t.Errorf("lookup(%s, %s) = %q, %v; want %q", c.host, c.login, got, err, c.want)
		}
	}
	t.Setenv("GITHUB_ENTERPRISE_TOKEN", "from-ghe-env")
	t.Setenv("GH_TOKEN", "from-gh")
	if got, _ := lookup(ctx, ghe, "ann"); got != "from-helper" { // no GITHUB_ENTERPRISE_HOST
		t.Errorf("without GITHUB_ENTERPRISE_HOST: %q", got)
	}
	t.Setenv("GITHUB_ENTERPRISE_HOST", ghe)
	if got, _ := lookup(ctx, ghe, "ann"); got != "from-ghe-env" {
		t.Errorf("with GITHUB_ENTERPRISE_TOKEN: %q", got)
	}
	if got, _ := lookup(ctx, "other.example.test", "ann"); got != "from-helper" {
		t.Errorf("another host: %q", got)
	}
}

// TestNewRefusesClearText: http is for loopback only, and an API of
// another host is refused.
func TestNewRefusesClearText(t *testing.T) {
	for _, api := range []string{"http://ghe.example.test/api/v3", "https://evil.example.test/api/v3"} {
		_, err := New(t.Context(), Options{Host: "ghe.example.test", Project: "o/r", API: api,
			Token: func(context.Context, forge.Account) (string, error) { return "t", nil }})
		if err == nil || errors.Is(err, forge.ErrNoToken) {
			t.Errorf("New with API %s: %v", api, err)
		}
	}
}

// Printing a Forge shows its key, never its token.
func TestStringHidesTheToken(t *testing.T) {
	t.Parallel()
	a, err := New(t.Context(), Options{Project: "o/r", Token: func(context.Context, forge.Account) (string, error) { return "SECRET-TOKEN", nil }})
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%v %+v", a, a); strings.Contains(got, "SECRET") {
		t.Errorf("printed %q", got)
	}
}
