package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

func serverOn(t *testing.T, b *store.Backend) (*httptest.Server, *apiServer) {
	t.Helper()
	s := &apiServer{cfg: &config.Config{}, b: b, me: "guilbep", cfgPath: filepath.Join(t.TempDir(), config.FileName)}
	ts := httptest.NewServer(withCORS(newMux(s, http.NotFoundHandler())))
	t.Cleanup(ts.Close)
	return ts, s
}

// A fresh clone's server pulls its remote's board as it starts, and
// /api/me says what came (docs/prd/clone-shows-board.md).
func TestFirstPullOnAFreshClone(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	storetest.NewIssue(t, theirs, "made before the clone")
	if _, err := theirs.Push("origin"); err != nil {
		t.Fatal(err)
	}
	ts, s := serverOn(t, mine)
	s.writeMu.Lock()
	s.firstPull()
	var me meResponse
	call(t, "GET", ts.URL+"/api/me", nil, &me)
	if me.FirstPull == nil || me.FirstPull.Issues != 1 || me.FirstPull.Remote != "origin" || me.FirstPull.Error != "" {
		t.Fatalf("first_pull = %+v", me.FirstPull)
	}
	if is, _ := mine.ListIssues(); len(is) != 1 {
		t.Errorf("the board holds %d issues", len(is))
	}
}

// origin unreachable: nothing pulled, nothing created, and /api/me says why.
func TestFirstPullWhenOriginCantBeRead(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	storetest.Git(t, b.GitDir(), "remote", "add", "origin", filepath.Join(t.TempDir(), "gone.git"))
	ts, s := serverOn(t, b)
	s.writeMu.Lock()
	s.firstPull()
	var me meResponse
	call(t, "GET", ts.URL+"/api/me", nil, &me)
	if me.FirstPull == nil || me.FirstPull.Issues != 0 || !strings.Contains(me.FirstPull.Error, "your issues may be there") {
		t.Fatalf("first_pull = %+v", me.FirstPull)
	}
}

// A board with work of its own, origin two ahead: opening pulls nothing,
// the page's fetch on open finds the two to get (Sync's badge) and changes
// nothing local; Pull brings them (docs/prd/clone-shows-board.md, 2).
func TestOpeningBehindOriginWaitsForPull(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	storetest.NewIssue(t, theirs, "theirs, one")
	storetest.NewIssue(t, theirs, "theirs, two")
	if _, err := theirs.Push("origin"); err != nil {
		t.Fatal(err)
	}
	storetest.NewIssue(t, mine, "mine, not pushed")
	ts, s := serverOn(t, mine)
	s.writeMu.Lock()
	s.firstPull()
	issues := func() int {
		t.Helper()
		is, err := mine.ListIssues()
		if err != nil {
			t.Fatal(err)
		}
		return len(is)
	}
	var st remotesStatus
	call(t, "GET", ts.URL+"/api/remotes", nil, &st)
	if len(st.Remotes) != 1 || st.Remotes[0].Checked != nil || issues() != 1 {
		t.Fatalf("opened: %+v, %d issues; want origin unchecked, 1 issue", st.Remotes, issues())
	}
	call(t, "POST", ts.URL+"/api/remotes/origin/fetch", nil, &st)
	if o := st.Remotes[0]; o.Checked == nil || o.Issues.Receive != 2 || o.Issues.Send != 1 {
		t.Errorf("after the fetch on open: %+v; want 2 to get, 1 to send", o)
	}
	if n := issues(); n != 1 {
		t.Errorf("the fetch changed the board: %d issues", n)
	}
	call(t, "POST", ts.URL+"/api/remotes/origin/pull", nil, &st)
	if n := issues(); n != 3 {
		t.Errorf("after Pull: %d issues, want 3", n)
	}
}

// A board with issues, or no remote: nothing to say.
func TestFirstPullSaysNothingOnABoard(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	storetest.NewIssue(t, b, "here already")
	ts, s := serverOn(t, b)
	s.writeMu.Lock()
	s.firstPull()
	var me meResponse
	call(t, "GET", ts.URL+"/api/me", nil, &me)
	if me.FirstPull != nil {
		t.Errorf("first_pull = %+v, want none", me.FirstPull)
	}
}
