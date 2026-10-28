package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghbridge"
	"github.com/guilbep/git-board/internal/store/storetest"
)

type startOverAnswer struct {
	Deleted int    `json:"deleted"`
	Backup  string `json:"backup"`
	Me      bool   `json:"me"`
}

// Start over's actions, as the page runs them: empty this clone's
// board, make origin's this one's, take origin's.
func TestStartOverActions(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	me, them := serverOf(t, mine), serverOf(t, theirs)
	storetest.NewIssue(t, mine, "mine")
	if _, err := mine.PushStep("origin"); err != nil {
		t.Fatal(err)
	}

	var got startOverAnswer
	if code := call(t, "POST", me.URL+"/api/start-over", map[string]any{"action": "reset"}, &got); code != http.StatusOK || got.Backup == "" {
		t.Fatalf("reset: %d %+v", code, got)
	}
	if is, _ := mine.ListIssues(); len(is) != 0 {
		t.Errorf("issues left: %v", is)
	}
	if code := call(t, "POST", me.URL+"/api/start-over", map[string]any{"action": "push-prune"}, &got); code != http.StatusOK {
		t.Fatalf("push-prune: %d", code)
	}
	origin := strings.TrimSpace(storetest.Git(t, mine.GitDir(), "remote", "get-url", "origin"))
	if left := storetest.Git(t, origin, "for-each-ref", "refs/board/issue"); left != "" {
		t.Errorf("origin's issues left: %q", left)
	}

	storetest.NewIssue(t, theirs, "theirs")
	if code := call(t, "POST", them.URL+"/api/start-over", map[string]any{"action": "take"}, &got); code != http.StatusOK {
		t.Fatalf("take: %d", code)
	}
	if is, _ := theirs.ListIssues(); len(is) != 0 {
		t.Errorf("theirs kept %v", is)
	}
	if code := call(t, "POST", me.URL+"/api/start-over", map[string]any{"action": "nope"}, nil); code != http.StatusBadRequest {
		t.Errorf("an unknown action: %d, want 400", code)
	}
}

// Start over waits for a sync job: it would empty the board under it.
func TestStartOverWaitsForTheSync(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	storetest.NewIssue(t, b, "stays")
	s := &apiServer{cfg: &config.Config{}, b: b}
	s.syncing.Store(true)
	ts := httptest.NewServer(withCORS(newMux(s, http.NotFoundHandler())))
	t.Cleanup(ts.Close)
	if code := call(t, "POST", ts.URL+"/api/start-over", map[string]any{"action": "reset"}, nil); code != http.StatusConflict {
		t.Errorf("reset while syncing: %d, want 409", code)
	}
	if is, _ := b.ListIssues(); len(is) != 1 {
		t.Errorf("issues %v", is)
	}
}

// What Start over finds: git-bug's data, and whether the repository
// switched, without which a prune is refused.
func TestStartOverStatus(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	dir := b.GitDir()
	storetest.Git(t, dir, "update-ref", "refs/bugs/"+strings.Repeat("a", 64), strings.TrimSpace(storetest.Git(t, dir, "hash-object", "-w", "--stdin")))
	ts := serverOf(t, b)
	var st struct {
		Switched bool `json:"switched"`
		GitBug   struct {
			Refs int `json:"refs"`
		} `json:"git_bug"`
	}
	if call(t, "GET", ts.URL+"/api/start-over", nil, &st); st.Switched || st.GitBug.Refs != 1 {
		t.Errorf("unswitched: %+v", st)
	}
	tree := strings.TrimSpace(storetest.Git(t, dir, "mktree"))
	storetest.Git(t, dir, "update-ref", "refs/board/switched", strings.TrimSpace(storetest.Git(t, dir, "commit-tree", tree, "-m", "mark")))
	if call(t, "GET", ts.URL+"/api/start-over", nil, &st); !st.Switched || st.GitBug.Refs != 1 {
		t.Errorf("switched: %+v", st)
	}
}

// The Project's Pull and Push: one side of the plan each.
func TestSyncApplyOneSide(t *testing.T) {
	t.Parallel()
	ts, b, id, rec := syncFixture(t, true)
	// agreed at qa, changed here since: a push
	if err := b.SetProjectBases(id, ghbridge.BasePrefix(config.GithubProject{Owner: "o", Number: 1, Write: true}), map[string]string{"status": "qa"}); err != nil {
		t.Fatal(err)
	}
	if resp := postJSON(t, ts.URL+"/api/sync/apply", map[string]string{"side": "labels"}, nil); resp.StatusCode != http.StatusOK || len(rec.edits) != 0 {
		t.Fatalf("labels: %d, edits %+v", resp.StatusCode, rec.edits)
	}
	if resp := postJSON(t, ts.URL+"/api/sync/apply", map[string]string{"side": "project"}, nil); resp.StatusCode != http.StatusOK || len(rec.edits) != 1 {
		t.Fatalf("project: %d, edits %+v", resp.StatusCode, rec.edits)
	}
}

// A start-over changes the issues every tab shows: it is an issues event,
// which the pages reload on, not a settings one.
func TestStartOverTellsEveryTab(t *testing.T) {
	t.Parallel()
	if e := eventFor(httptest.NewRequest("POST", "/api/start-over", nil)); e.Kind != "issues" {
		t.Errorf("kind %q, want issues", e.Kind)
	}
}
