package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/git-bug/git-bug/repository/repotest"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
	"github.com/guilbep/git-board/internal/inbox"
)

func getInbox(t *testing.T, ts *httptest.Server) inboxResponse {
	t.Helper()
	var res inboxResponse
	if code := call(t, "GET", ts.URL+"/api/inbox", nil, &res); code != http.StatusOK {
		t.Fatalf("GET /api/inbox: %d", code)
	}
	return res
}

func TestInboxFlow(t *testing.T) {
	ts, s := newIssueServer(t) // writes as the test identity, me: guilbep
	id, _ := s.b.NewIssue("Train legs", "", []string{"assignee:guilbep"})
	ann := gitbugiotest.NewPerson(t, s.b, "Ann", "ann")
	gitbugiotest.CommentAs(t, s.b, id, ann, time.Now().Unix(), "first")

	res := getInbox(t, ts)
	// author: you opened it (the test identity); assignee: the label
	if len(res.Rows) != 1 || !slices.Equal(res.Rows[0].News, []string{"1 comment"}) || !slices.Equal(res.Rows[0].Reasons, []string{"author", "assignee"}) || res.Following != 1 {
		t.Fatalf("inbox: %+v", res)
	}

	if code := call(t, "POST", ts.URL+"/api/inbox/seen", map[string]any{"issues": []string{id}}, &struct{}{}); code != http.StatusOK {
		t.Fatalf("seen: %d", code)
	}
	if res := getInbox(t, ts); len(res.Rows) != 0 {
		t.Fatalf("after seen: %+v", res.Rows)
	}

	var f followResponse
	if code := call(t, "PUT", ts.URL+"/api/inbox/follow/"+id, map[string]any{"follow": false}, &f); code != http.StatusOK || f.Following || f.Set == nil || *f.Set {
		t.Fatalf("unfollow: %d %+v", code, f)
	}
	gitbugiotest.CommentAs(t, s.b, id, ann, time.Now().Unix(), "second")
	if res := getInbox(t, ts); len(res.Rows) != 0 {
		t.Fatalf("unfollowed, yet: %+v", res.Rows)
	}
	if code := call(t, "PUT", ts.URL+"/api/inbox/follow/"+id, map[string]any{"follow": nil}, &f); code != http.StatusOK || !f.Following || f.Set != nil {
		t.Fatalf("back to automatic: %d %+v", code, f)
	}
	if res := getInbox(t, ts); len(res.Rows) != 1 {
		t.Fatalf("followed again: %+v", res.Rows)
	}

	if code := call(t, "PUT", ts.URL+"/api/inbox/settings", map[string]any{"views": []string{"gone"}, "fields": true}, &struct{}{}); code != http.StatusOK {
		t.Fatalf("settings: %d", code)
	}
	if res := getInbox(t, ts); !res.Fields || !slices.Equal(res.ViewsMissing, []string{"gone"}) {
		t.Fatalf("settings: %+v", res)
	}
}

// codeOf is the error code a request answers.
func codeOf(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct{ Code string }
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body.Code
}

func TestInboxNeedsWhoYouAre(t *testing.T) {
	noLogin := &apiServer{cfg: &config.Config{}, b: gitbugiotest.New(t)} // its identity has no login, no --me
	ts := httptest.NewServer(withCORS(newMux(noLogin, http.NotFoundHandler())))
	t.Cleanup(ts.Close)
	if code, c := codeOf(t, ts.URL+"/api/inbox"); code != http.StatusConflict || c != "no-login" {
		t.Errorf("no login: %d %q", code, c)
	}

	b, err := gitbugio.NewForRepo(repotest.NewGoGitRepo(t, false)) // no identity chosen
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	ts2 := httptest.NewServer(withCORS(newMux(&apiServer{cfg: &config.Config{}, b: b, me: "guilbep"}, http.NotFoundHandler())))
	t.Cleanup(ts2.Close)
	if code, c := codeOf(t, ts2.URL+"/api/inbox"); code != http.StatusConflict || c != "no-identity" {
		t.Errorf("no identity: %d %q", code, c)
	}
}

// Review Focus 1: switching who you write as switches inbox.
func TestInboxIsPerIdentity(t *testing.T) {
	ts, s := newIssueServer(t)
	id, _ := s.b.NewIssue("Train legs", "", []string{"assignee:guilbep"})
	first, _, _ := s.b.Me()
	ann := gitbugiotest.NewPerson(t, s.b, "Ann", "ann")

	if code := call(t, "POST", ts.URL+"/api/me", map[string]any{"id": ann.Id().String()}, nil); code != http.StatusOK {
		t.Fatalf("adopt ann: %d", code)
	}
	if code := call(t, "PUT", ts.URL+"/api/inbox/follow/"+id, map[string]any{"follow": false}, nil); code != http.StatusOK {
		t.Fatalf("ann unfollows: %d", code)
	}
	if code := call(t, "POST", ts.URL+"/api/me", map[string]any{"id": first.ID}, nil); code != http.StatusOK {
		t.Fatalf("adopt back: %d", code)
	}
	var f followResponse
	call(t, "GET", ts.URL+"/api/inbox/follow/"+id, nil, &f)
	if f.Set != nil || !f.Following {
		t.Errorf("ann's Unfollow reached the first identity's inbox: %+v", f)
	}
}

// Review Focus 2: two tabs marking at once both stick.
func TestInboxSeenFromTwoTabs(t *testing.T) {
	ts, s := newIssueServer(t)
	a, _ := s.b.NewIssue("A", "", []string{"assignee:guilbep"})
	b, _ := s.b.NewIssue("B", "", []string{"assignee:guilbep"})
	var wg sync.WaitGroup
	for _, id := range []string{a, b} {
		wg.Add(1)
		go func() { // no t.Fatal off the test's goroutine: plain requests
			defer wg.Done()
			req, _ := http.NewRequest("POST", ts.URL+"/api/inbox/seen", strings.NewReader(`{"issues":["`+id+`"]}`))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("seen %s: %v", id, err)
				return
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("seen %s: %d", id, resp.StatusCode)
			}
		}()
	}
	wg.Wait()
	me, _, _ := s.b.Me()
	st, err := inbox.Load(s.b.Storage(), me.ID)
	if err != nil || len(st.Seen[a]) == 0 || len(st.Seen[b]) == 0 {
		t.Errorf("seen after two tabs: %v %v", st.Seen, err)
	}
}

// Review Focus 3 and 4: prefixes are stored whole; an unknown issue
// changes nothing.
func TestInboxTakesPrefixes(t *testing.T) {
	ts, s := newIssueServer(t)
	id, _ := s.b.NewIssue("Train legs", "", nil)
	if code := call(t, "PUT", ts.URL+"/api/inbox/follow/"+id[:7], map[string]any{"follow": true}, nil); code != http.StatusOK {
		t.Fatalf("follow by prefix: %d", code)
	}
	me, _, _ := s.b.Me()
	st, _ := inbox.Load(s.b.Storage(), me.ID)
	if !slices.Equal(st.Follow, []string{id}) {
		t.Errorf("stored %v, want the full id %s", st.Follow, id)
	}
	if code := call(t, "PUT", ts.URL+"/api/inbox/follow/deadbeef", map[string]any{"follow": true}, nil); code != http.StatusNotFound {
		t.Errorf("unknown issue: %d, want 404", code)
	}
	if code := call(t, "POST", ts.URL+"/api/inbox/seen", map[string]any{"issues": []string{"deadbeef"}}, nil); code != http.StatusNotFound {
		t.Errorf("unknown issue seen: %d, want 404", code)
	}
	if after, _ := inbox.Load(s.b.Storage(), me.ID); !slices.Equal(after.Follow, []string{id}) || len(after.Seen) != 0 {
		t.Errorf("an unknown issue changed the file: %+v", after)
	}
}

// A project role (lead-dev of the whole project) brings its new and closed
// issues, not their comments; the inbox says which roles you hold.
func TestInboxProjectRole(t *testing.T) {
	ts, s := newIssueServer(t)
	s.cfg = &config.Config{Roles: map[string]string{"lead-dev": "guilbep"}, Fields: map[string]config.Field{"lead-dev": {Type: config.TypePerson}}}
	ann := gitbugiotest.NewPerson(t, s.b, "Ann", "ann")
	hers, _, err := s.b.TestCache().Bugs().NewRaw(ann, time.Now().Unix(), "Hers", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := hers.Id().String()
	res := getInbox(t, ts)
	if len(res.Rows) != 1 || !slices.Equal(res.Rows[0].News, []string{"new issue"}) || !slices.Equal(res.Rows[0].Reasons, []string{"lead-dev (project)"}) || !slices.Equal(res.Roles, []string{"lead-dev"}) {
		t.Fatalf("a new issue as lead-dev: %+v", res)
	}
	call(t, "POST", ts.URL+"/api/inbox/seen", map[string]any{"issues": []string{id}}, nil)
	gitbugiotest.CommentAs(t, s.b, id, ann, time.Now().Unix(), "a comment")
	if res := getInbox(t, ts); len(res.Rows) != 0 {
		t.Fatalf("a comment reached the project's lead-dev: %+v", res.Rows)
	}
	if _, err := hers.CloseRaw(ann, time.Now().Unix(), nil); err != nil {
		t.Fatal(err)
	}
	if err := hers.Commit(); err != nil {
		t.Fatal(err)
	}
	if res := getInbox(t, ts); len(res.Rows) != 1 || !slices.Equal(res.Rows[0].News, []string{"closed"}) {
		t.Fatalf("a close as lead-dev: %+v", res.Rows)
	}
}
