package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// fakeGitHub is GitHub's API as the gate uses it, answering with
// internal/ghpr's fixtures (co2's PR #3082 as o/r), and what it was sent.
type fakeGitHub struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string          // "METHOD path"
	bodies   map[string]string // by "METHOD path"
	refuse   bool              // the merge: GitHub's 409
	down     bool              // every request: GitHub's 500
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{bodies: map[string]string{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-dev" {
			t.Errorf("Authorization %q", r.Header.Get("Authorization"))
		}
		key := r.Method + " " + r.URL.Path
		data, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.requests, f.bodies[key] = append(f.requests, key), string(data)
		refuse, down := f.refuse, f.down
		f.mu.Unlock()
		if down {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch key {
		case "POST /graphql":
			serveFixture(t, w, "pr.json")
		case "GET /repos/o/r/rules/branches/dev":
			serveFixture(t, w, "rules.json")
		case "POST /repos/o/r/pulls":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"number": 9, "html_url": "https://github.com/o/r/pull/9"}`)
		case "PATCH /repos/o/r/pulls/3082":
			_, _ = io.WriteString(w, `{"number":3082}`)
		case "PUT /repos/o/r/pulls/3082/merge":
			if refuse {
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `{"message":"Head branch was modified. Review and try the merge again."}`)
				return
			}
			_, _ = io.WriteString(w, `{"merged":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func serveFixture(t *testing.T, w http.ResponseWriter, name string) {
	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "ghpr", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write(data)
}

func (f *fakeGitHub) body(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[key]
}

func (f *fakeGitHub) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

// gateRepo is a repository whose origin is url, with dev (origin's
// default branch), fix/2518-x on origin and 2518-local only here, the
// identity dev (GitHub login dev), and the issue #2518 of o/r. withToken
// stores dev's token, in a keyring of its own: git-bug's keyring is one
// file for every repository, in the user's config directory. Its HOME
// keeps its tests serial.
func gateRepo(t *testing.T, url string, withToken bool) (*apiServer, *fakeGitHub, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	if ucd, err := os.UserConfigDir(); err != nil || !strings.HasPrefix(ucd, home) {
		t.Fatalf("the user's config directory %q is not the test's", ucd)
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "dev")
	git(t, dir, "config", "user.name", "dev")
	git(t, dir, "config", "user.email", "dev@example.com")
	git(t, dir, "remote", "add", "origin", url)
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "start")
	git(t, dir, "branch", "fix/2518-x")
	git(t, dir, "branch", "2518-local")
	git(t, dir, "update-ref", "refs/remotes/origin/dev", "dev")
	git(t, dir, "update-ref", "refs/remotes/origin/fix/2518-x", "fix/2518-x")
	git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/dev")

	b, err := gitbugio.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if _, err := b.NewIdentity("dev", "dev@example.com"); err != nil {
		t.Fatal(err)
	}
	gitbugiotest.SetMyLogin(t, b, "dev")
	if withToken {
		t.Skip("M5 phase 2: tokens")
	}
	id := gitbugiotest.NewImportedIssue(t, b, "[BUG](Equipment) Crash on save (#2518)", "https://github.com/o/r/issues/2518")
	gh := newFakeGitHub(t)
	return &apiServer{cfg: &config.Config{}, b: b, me: "guilbep", githubAPI: gh.URL, gitDir: filepath.Join(dir, ".git"), root: dir}, gh, id
}

func serve(t *testing.T, s *apiServer) *httptest.Server {
	ts := httptest.NewServer(withCORS(newMux(s, http.NotFoundHandler())))
	t.Cleanup(ts.Close)
	return ts
}

type gateAnswer struct {
	Repo  string `json:"repo"`
	Base  string `json:"base"`
	Login string `json:"login"`
	Pulls []struct {
		Branch string `json:"branch"`
		PR     *struct {
			Number   int      `json:"number"`
			Head     string   `json:"head"`
			CanMerge bool     `json:"can_merge"`
			Waits    []string `json:"waits"`
		} `json:"pr"`
	} `json:"pulls"`
}

func TestPulls(t *testing.T) {
	s, gh, id := gateRepo(t, "https://github.com/o/r.git", true)
	ts := serve(t, s)
	var res gateAnswer
	if resp := getJSON(t, ts.URL+"/api/issues/"+id[:8]+"/pulls", &res); resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if res.Repo != "o/r" || res.Base != "dev" || res.Login != "dev" || len(res.Pulls) != 1 {
		t.Fatalf("pulls %+v", res)
	}
	p := res.Pulls[0]
	want := []string{"Frontent Lint/Type Check", "Frontend Tests", "an approval"}
	if p.Branch != "fix/2518-x" || p.PR == nil || p.PR.Number != 3082 || p.PR.CanMerge || strings.Join(p.PR.Waits, ", ") != strings.Join(want, ", ") {
		t.Errorf("fix/2518-x %+v", p.PR)
	}
	if got := strings.Join(gh.sent(), ", "); got != "POST /graphql, GET /repos/o/r/rules/branches/dev" {
		t.Errorf("GitHub was asked %s", got)
	}
}

// Without GitHub, or without a branch on origin, the gate asks GitHub
// nothing: Development works offline.
func TestPullsWithoutGitHub(t *testing.T) {
	s, gh, id := gateRepo(t, "https://gitlab.com/o/r.git", true)
	var res gateAnswer
	if resp := getJSON(t, serve(t, s).URL+"/api/issues/"+id[:8]+"/pulls", &res); resp.StatusCode != http.StatusOK || res.Repo != "" || len(res.Pulls) != 0 {
		t.Errorf("a GitLab origin: %d %+v", resp.StatusCode, res)
	}
	s, gh2, _ := gateRepo(t, "https://github.com/o/r.git", false)
	other := gitbugiotest.NewIssue(t, s.b, "No branch yet")
	if resp := getJSON(t, serve(t, s).URL+"/api/issues/"+other[:8]+"/pulls", &res); resp.StatusCode != http.StatusOK || res.Repo != "o/r" || len(res.Pulls) != 0 {
		t.Errorf("no branch on origin: %d %+v", resp.StatusCode, res)
	}
	if len(gh.sent())+len(gh2.sent()) != 0 {
		t.Errorf("GitHub was asked %v %v", gh.sent(), gh2.sent())
	}
}

func TestPullsNoToken(t *testing.T) {
	t.Skip("M5 phase 2: tokens")
}

func TestOpenPull(t *testing.T) {
	s, gh, id := gateRepo(t, "https://github.com/o/r.git", true)
	ts := serve(t, s)
	var pr struct {
		Number int    `json:"number"`
		URL    string `json:"url"`
	}
	if code := call(t, "POST", ts.URL+"/api/issues/"+id[:8]+"/pulls", map[string]string{"branch": "fix/2518-x"}, &pr); code != http.StatusCreated || pr.Number != 9 {
		t.Fatalf("open: %d %+v", code, pr)
	}
	var sent map[string]string
	if err := json.Unmarshal([]byte(gh.body("POST /repos/o/r/pulls")), &sent); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"head": "fix/2518-x", "base": "dev", "title": "[BUG](Equipment) Crash on save", "body": id[:7] + "\n\nCloses #2518"}
	for k, v := range want {
		if sent[k] != v {
			t.Errorf("%s = %q, want %q", k, sent[k], v)
		}
	}
	if code := call(t, "POST", ts.URL+"/api/issues/"+id[:8]+"/pulls", map[string]string{"branch": "2518-local"}, nil); code != http.StatusBadRequest {
		t.Errorf("a branch only here: %d, want 400", code)
	}
	resp, err := http.Post(ts.URL+"/api/issues/"+id[:8]+"/pulls", "text/plain", strings.NewReader(`{"branch":"fix/2518-x"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("not JSON: %d, want refused", resp.StatusCode)
	}
}

func TestMergePull(t *testing.T) {
	s, gh, id := gateRepo(t, "https://github.com/o/r.git", true)
	ts := serve(t, s)
	url := ts.URL + "/api/issues/" + id[:8] + "/pulls/3082/merge"
	if code := call(t, "POST", url, map[string]string{"sha": "c1ee2d98598eb5a0dadeb8a67a2fee1e3593eae3"}, nil); code != http.StatusOK {
		t.Fatalf("merge: %d", code)
	}
	if got := gh.body("PUT /repos/o/r/pulls/3082/merge"); got != `{"merge_method":"merge","sha":"c1ee2d98598eb5a0dadeb8a67a2fee1e3593eae3"}` {
		t.Errorf("GitHub was sent %s", got)
	}
	gh.mu.Lock()
	gh.refuse = true
	gh.mu.Unlock()
	data, _ := json.Marshal(map[string]string{"sha": "c1ee2d9"})
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadGateway || !strings.Contains(string(body), "Head branch was modified") {
		t.Errorf("a refused merge: %d %s", resp.StatusCode, body)
	}
}

func commentReview(t *testing.T, s *apiServer, issue, id string) *gitbugio.Review {
	t.Helper()
	r, err := s.b.CommentReview(issue, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// An edit writes the title and description of the issue's own PR, and no
// other: the number is found again on GitHub first.
func TestEditPull(t *testing.T) {
	s, gh, id := gateRepo(t, "https://github.com/o/r.git", true)
	ts := serve(t, s)
	var pr struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	edit := map[string]string{"title": "Fix the crash on save", "body": "Closes #2518"}
	if code := call(t, "PATCH", ts.URL+"/api/issues/"+id[:8]+"/pulls/3082", edit, &pr); code != http.StatusOK || pr.Title != edit["title"] || pr.Body != edit["body"] {
		t.Fatalf("edit: %d %+v", code, pr)
	}
	if got := gh.body("PATCH /repos/o/r/pulls/3082"); got != `{"title":"Fix the crash on save","body":"Closes #2518"}` {
		t.Errorf("GitHub was sent %s", got)
	}
	if cached, _ := s.pulls.PR("fix/2518-x"); cached == nil || cached.Title != edit["title"] {
		t.Errorf("the cycle's copy: %+v", cached)
	}
	if code := call(t, "PATCH", ts.URL+"/api/issues/"+id[:8]+"/pulls/77", edit, nil); code != http.StatusBadRequest {
		t.Errorf("another PR: %d, want 400", code)
	}
	if code := call(t, "PATCH", ts.URL+"/api/issues/"+id[:8]+"/pulls/3082", map[string]string{"title": " "}, nil); code != http.StatusBadRequest {
		t.Errorf("no title: %d, want 400", code)
	}
	if slices.Contains(gh.sent(), "PATCH /repos/o/r/pulls/77") {
		t.Error("a PR of no branch of the issue was written")
	}
}
