package gitlab

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// The suite, over the fake served as GitLab, two items a page.
func TestConformance(t *testing.T) {
	t.Parallel()
	conformance.Run(t, func(t *testing.T, f *fake.Forge) forge.Forge {
		_, a := serve(t, f, "p")
		return a
	})
}

var ann = forge.Account{ID: "7", Login: "ann"}

func titlesOf(list []forge.Issue) []string {
	var out []string
	for _, is := range list {
		out = append(out, is.Title)
	}
	return out
}

// Pages are read to the end, and an issue or a note a page repeats counts
// once. Notes come oldest first, as asked: the server's default is newest.
func TestPagingAndRepeats(t *testing.T) {
	t.Parallel()
	f := fake.New("example.com/p")
	var cs []forge.Comment
	for i := range 5 {
		cs = append(cs, forge.Comment{Author: ann, Body: fmt.Sprint("c", i), Time: int64(100 + i)})
	}
	for _, title := range []string{"A", "B", "C"} {
		f.Add(forge.Issue{Author: ann, Title: title, Comments: cs})
	}
	_, a := serve(t, f, "p", func(s *server) { s.overlap = true })
	list, err := a.Issues(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := titlesOf(list); !slices.Equal(got, []string{"A", "B", "C"}) {
		t.Fatalf("issues = %v", got)
	}
	c, err := a.Complete(t.Context(), list[2])
	if err != nil {
		t.Fatal(err)
	}
	var bodies []string
	for _, c := range c.Comments {
		bodies = append(bodies, c.Body)
	}
	if !slices.Equal(bodies, []string{"c0", "c1", "c2", "c3", "c4"}) {
		t.Fatalf("comments = %v", bodies)
	}
}

// The listing starts 2 minutes before since: GitLab moves updated_at late.
func TestListingMargin(t *testing.T) {
	t.Parallel()
	f := fake.New("example.com/p")
	since := time.Now().Unix()
	f.Add(forge.Issue{Author: ann, Title: "old", Created: since - 200})
	f.Add(forge.Issue{Author: ann, Title: "late", Created: since - 60})
	_, a := serve(t, f, "p")
	list, err := a.Issues(t.Context(), since)
	if err != nil {
		t.Fatal(err)
	}
	if got := titlesOf(list); !slices.Equal(got, []string{"late"}) {
		t.Fatalf("issues = %v", got)
	}
}

// A web edit's \r\n comes back as \n.
func TestLineEndings(t *testing.T) {
	t.Parallel()
	f := fake.New("example.com/p")
	is := f.Add(forge.Issue{Author: ann, Title: "T", Body: "a\r\nb\r\n", Comments: []forge.Comment{{Author: ann, Body: "c\r\n\r\nd"}}})
	_, a := serve(t, f, "p")
	got, err := a.Issue(t.Context(), is.Ref())
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "a\nb\n" || got.Comments[0].Body != "c\n\nd" {
		t.Fatalf("body %q, comment %q", got.Body, got.Comments[0].Body)
	}
}

// System notes are no comments, but tell when the title, body and status
// changed. Internal notes are never read.
func TestSystemAndInternalNotes(t *testing.T) {
	t.Parallel()
	f := fake.New("example.com/p")
	is := f.Add(forge.Issue{Author: ann, Title: "T", Comments: []forge.Comment{{Author: ann, Body: "hi", Time: 500}}})
	note := func(id, at int64, body string, flag string) map[string]any {
		n := map[string]any{"id": id, "body": body, "author": userJSON(ann), "created_at": ts(at), "updated_at": ts(at)}
		n[flag] = true
		return n
	}
	_, a := serve(t, f, "p", func(s *server) {
		s.extra[is.Number] = []map[string]any{
			note(50, 1000, "changed title from **a** to **b**", "system"),
			note(51, 2000, "changed the description", "system"),
			note(52, 3000, "closed", "system"),
			note(53, 3500, "added ~bug label", "system"),
			note(54, 4000, "between us", "internal"),
			note(55, 4500, "between us, before 15.0", "confidential"),
		}
	})
	got, err := a.Issue(t.Context(), is.Ref())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Comments) != 1 || got.Comments[0].Body != "hi" {
		t.Fatalf("comments = %+v", got.Comments)
	}
	if want := map[string]int64{"title": 1000, "body": 2000, "status": 3000}; !maps.Equal(got.Changed, want) {
		t.Fatalf("changed = %v, want %v", got.Changed, want)
	}
	// A state event tells the status's change too.
	f.Edit(is.Number, func(is *forge.Issue) { is.Status = "closed" })
	if got, _ = a.Issue(t.Context(), is.Ref()); got.Changed["status"] < time.Now().Unix()-60 {
		t.Fatalf("changed = %v", got.Changed)
	}
}

// Confidential issues aren't listed, can't be read, and take no writes,
// even once mirrored. A fresh adapter finds one by its id too.
func TestConfidential(t *testing.T) {
	t.Parallel()
	f := fake.New("example.com/p")
	hidden := f.Add(forge.Issue{Author: ann, Title: "hidden"})
	f.SetConfidential(hidden.Number, true)
	shown := f.Add(forge.Issue{Author: ann, Title: "shown"})
	_, a := serve(t, f, "p")
	if _, err := a.Issue(t.Context(), hidden.Ref()); !errors.Is(err, forge.ErrConfidential) {
		t.Fatalf("reading it unlisted: %v", err)
	}
	list, err := a.Issues(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := titlesOf(list); !slices.Equal(got, []string{"shown"}) {
		t.Fatalf("issues = %v", got)
	}
	f.SetConfidential(shown.Number, true)
	ctx := t.Context()
	for name, err := range map[string]error{
		"title":   a.SetTitle(ctx, shown.Ref(), "x"),
		"labels":  a.ChangeLabels(ctx, shown.Ref(), []string{"bug"}, nil),
		"comment": func() error { _, err := a.CreateComment(ctx, shown.Ref(), "x"); return err }(),
	} {
		if !errors.Is(err, forge.ErrConfidential) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if w := f.Writes(); len(w) != 0 {
		t.Fatalf("writes = %+v", w)
	}
}

// The project's visibility, as GitLab says it; another value is an error.
func TestVisibility(t *testing.T) {
	t.Parallel()
	for _, vis := range []string{"public", "internal", "private", "secret"} {
		_, a := serve(t, fake.New("example.com/p"), "p", func(s *server) { s.visibility = vis })
		got, err := a.Visibility(t.Context())
		if (vis == "secret") != (err != nil) || err == nil && string(got) != vis {
			t.Errorf("%s: %v, %v", vis, got, err)
		}
	}
}

// A project named by its path goes to GitLab escaped, and is its key.
func TestNamespacedPath(t *testing.T) {
	t.Parallel()
	f := fake.New("example.com/group/proj")
	_, a := serve(t, f, "group/sub/proj")
	if !strings.HasSuffix(a.Key(), "/group/sub/proj") || a.Key() != a.Server()+"/group/sub/proj" {
		t.Fatalf("key = %q, server = %q", a.Key(), a.Server())
	}
	is, err := a.CreateIssue(t.Context(), "T", "b")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetTitle(t.Context(), is.Ref(), "T2"); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.Get(is.Number); got.Title != "T2" {
		t.Fatalf("title = %q", got.Title)
	}
}

// GitLab's answers map to the forge's errors.
func TestErrors(t *testing.T) {
	t.Parallel()
	f := fake.New("example.com/p")
	is := f.Add(forge.Issue{Author: ann, Title: "T"})
	_, a := serve(t, f, "p")
	ctx := t.Context()
	if _, err := a.Issue(ctx, forge.Ref{ID: "424242", Number: "424242"}); !errors.Is(err, forge.ErrNotFound) {
		t.Errorf("an unknown issue: %v", err)
	}
	if err := a.EditComment(ctx, is.Ref(), "424242", "x"); !errors.Is(err, forge.ErrNotFound) {
		t.Errorf("an unknown comment: %v", err)
	}
	f.Before(func(fake.Write) error { return forge.ErrRefused })
	if err := a.SetTitle(ctx, is.Ref(), "x"); !errors.Is(err, forge.ErrRefused) {
		t.Errorf("a refused write: %v", err)
	}
	f.Before(nil)
	// a label with a comma, or one left out, is refused alone: the rest go
	f.RefuseLabels("bad")
	err := a.ChangeLabels(ctx, is.Ref(), []string{"a,b", "bad", "ui"}, []string{"c,d"})
	var some *forge.Refused
	if !errors.As(err, &some) || !slices.Equal(some.Values, []string{"a,b", "c,d", "bad"}) || !errors.Is(err, forge.ErrRefused) {
		t.Errorf("refused labels: %v", err)
	}
	if got, _ := f.Get(is.Number); !slices.Equal(got.Labels, []string{"ui"}) {
		t.Errorf("labels = %v, want ui", got.Labels)
	}
	f.Delete(is.Number)
	if err := a.SetStatus(ctx, is.Ref(), "closed"); !errors.Is(err, forge.ErrNotFound) {
		t.Errorf("a deleted issue: %v", err)
	}
}

// A 429 is retried after Retry-After, a read's 5xx after 1, 2 and 4 s,
// and a spent rate limit waits for its reset. A create's 5xx isn't retried:
// it may have landed.
func TestRetries(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var failing string // method and path
	var code, n int
	var spend bool
	_, a := serve(t, fake.New("example.com/p"), "p", func(s *server) {
		s.before = func(w http.ResponseWriter, r *http.Request) bool {
			mu.Lock()
			defer mu.Unlock()
			if r.Method+" "+r.URL.Path != failing || n == 0 {
				if spend {
					w.Header().Set("RateLimit-Remaining", "0")
					w.Header().Set("RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
					spend = false
				}
				return false
			}
			n--
			if code == 429 {
				w.Header().Set("Retry-After", "7")
			}
			w.WriteHeader(code)
			return true
		}
	})
	var slept []time.Duration
	fail := func(k string, c, times int) {
		mu.Lock()
		defer mu.Unlock()
		failing, code, n, slept = k, c, times, nil
	}
	ctx, s := t.Context(), time.Second
	check := func(what string, err error, ok bool, want ...time.Duration) {
		t.Helper()
		if (err == nil) != ok || !slices.Equal(slept, want) {
			t.Fatalf("%s: %v, slept %v, want %v", what, err, slept, want)
		}
	}
	a.p.sleep = func(_ context.Context, d time.Duration) error {
		if d > 0 {
			slept = append(slept, d.Round(s))
		}
		return nil
	}

	fail("GET /api/v4/user", 429, 2)
	_, err := a.Me(ctx)
	check("429 twice", err, true, 7*s, 7*s)
	fail("GET /api/v4/projects/p", 503, 3)
	_, err = a.Visibility(ctx)
	check("503 three times", err, true, s, 2*s, 4*s)
	fail("GET /api/v4/projects/p", 500, 4)
	_, err = a.Visibility(ctx)
	check("500 four times", err, false, s, 2*s, 4*s)
	fail("POST /api/v4/projects/p/issues", 502, 1)
	_, err = a.CreateIssue(ctx, "T", "")
	check("502 on a create", err, false)
	fail("POST /api/v4/projects/p/issues", 429, 1)
	_, err = a.CreateIssue(ctx, "T", "")
	check("429 on a create", err, true, 7*s)

	fail("", 0, 0)
	mu.Lock()
	spend = true
	mu.Unlock()
	_, err = a.Visibility(ctx) // answered as spent
	check("spending it", err, true)
	_, err = a.Visibility(ctx)
	if err != nil || len(slept) != 1 || slept[0] < 59*time.Minute {
		t.Fatalf("after it was spent: %v, slept %v", err, slept)
	}
	slept = nil
	_, err = a.Visibility(ctx)
	check("once waited", err, true)
}

// The posting account's token is GITLAB_TOKEN when set, else the
// credential helper's for the host and login; another account's is the
// helper's, once GitLab says it is theirs. Not parallel: it sets the
// environment, and runs git in a home of its own.
func TestTokenLookup(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "helper")
	script := "#!/bin/sh\ntest \"$1\" = get || exit 0\n" +
		"while read -r l; do case $l in username=*) u=${l#username=};; esac; done\n" +
		"test -n \"$u\" && echo password=helper-$u\nexit 0\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gitconfig"), []byte("[credential]\n\thelper = "+helper+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", dir)
	t.Chdir(dir)

	f := fake.New("example.com/p")
	me, _ := f.Me(t.Context())
	f.Grant(ann)
	s := &server{f: f, path: "p", tokens: map[string]forge.Account{
		"env": me, "helper-board": me, "helper-ann": ann, "helper-bob": ann, // bob's helper holds ann's token
	}}
	srv := httptest.NewServer(s.routes())
	defer srv.Close()
	ctx := t.Context()

	u, _ := url.Parse(srv.URL)
	t.Setenv("GITLAB_HOST", "other.example.test")
	t.Setenv(tokenEnv, "env")
	if a, err := New(ctx, srv.URL, "p", "board"); err != nil || a.token != "helper-board" {
		t.Fatalf("GITLAB_TOKEN for another host: %v", err) // falls through to the helper
	}
	if _, err := New(ctx, "http://gitlab.example.test", "p", "board"); err == nil || errors.Is(err, forge.ErrNoToken) {
		t.Fatalf("http to a non-loopback host: %v", err)
	}
	t.Setenv("GITLAB_HOST", u.Host)
	a, err := New(ctx, srv.URL, "p", "board")
	if err != nil || a.token != "env" {
		t.Fatalf("with GITLAB_TOKEN: %v", err)
	}
	t.Setenv(tokenEnv, "")
	if a, err = New(ctx, srv.URL, "p", "board"); err != nil || a.token != "helper-board" {
		t.Fatalf("from the helper: %v", err)
	}
	if _, err := New(ctx, srv.URL, "p", ""); !errors.Is(err, forge.ErrNoToken) {
		t.Fatalf("no username, which the helper doesn't know: %v", err)
	}
	if got, err := a.As(ctx, ann); err != nil || got.(*Forge).token != "helper-ann" {
		t.Fatalf("as ann: %v", err)
	}
	if got, err := a.As(ctx, me); err != nil || got != forge.Forge(a) {
		t.Fatalf("as the posting account: %v", err)
	}
	for _, who := range []forge.Account{{ID: "8", Login: "bob"}, {ID: "9", Login: "carl"}, {ID: "10", Login: "x\nusername=ann"}} {
		if _, err := a.As(ctx, who); !errors.Is(err, forge.ErrNoToken) {
			t.Errorf("as %s: %v", who.Login, err)
		}
	}
}

// A redirect, to another host here, isn't followed: Go would send
// PRIVATE-TOKEN along. It reads as not found.
func TestRedirectKeepsTheToken(t *testing.T) {
	t.Parallel()
	got := make(chan string, 1)
	away := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("PRIVATE-TOKEN")
		w.Write([]byte(`{"id":1,"username":"x"}`))
	}))
	defer away.Close()
	_, port, _ := strings.Cut(away.Listener.Addr().String(), ":")
	home := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://localhost:"+port+"/steal", http.StatusFound) // localhost, not 127.0.0.1
	}))
	defer home.Close()
	p, err := newProject(home.URL, "g/p")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&Forge{p: p, token: "SECRET-TOKEN"}).Me(t.Context()); !errors.Is(err, forge.ErrNotFound) {
		t.Errorf("err = %v, want not found", err)
	}
	select {
	case tok := <-got:
		t.Errorf("redirect followed, token %q", tok)
	default:
	}
}

// Path is the project's path, as GitLab names it.
func TestPath(t *testing.T) {
	t.Parallel()
	_, a := serve(t, fake.New("example.com/p"), "grp/r")
	if got, err := a.Path(t.Context()); got != "grp/r" || err != nil {
		t.Fatalf("%q, %v", got, err)
	}
}

// A reply goes into the discussion of the note it answers, found among the
// issue's discussions, pages read; a note gone is ErrNotFound.
func TestReply(t *testing.T) {
	t.Parallel()
	f := fake.New("example.com/p")
	var cs []forge.Comment
	for i := range 3 {
		cs = append(cs, forge.Comment{Author: ann, Body: fmt.Sprint("c", i), Time: int64(100 + i)})
	}
	is := f.Add(forge.Issue{Author: ann, Title: "A", Comments: cs})
	s, a := serve(t, f, "p")
	root := is.Comments[2].ID
	c, err := a.Reply(t.Context(), is.Ref(), root, "an answer")
	if err != nil {
		t.Fatal(err)
	}
	if s.threads[c.ID] != root || c.Body != "an answer" {
		t.Errorf("reply %+v in %q, want in %s's", c, s.threads[c.ID], root)
	}
	if _, err := a.Reply(t.Context(), is.Ref(), "999", "lost"); !errors.Is(err, forge.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
