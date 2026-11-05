package httpapi

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// A fresh clone of a board opens on origin's: the server's first pull
// brings it, /api/board says how many came, and a second server, the
// board now here, pulls nothing (docs/prd/clone-shows-board.md).
func TestFirstPull(t *testing.T) {
	t.Parallel()
	origin, _ := newBoard(t)
	gitRun(t, origin, "commit", "-q", "--allow-empty", "-m", "init")
	clone := filepath.Join(t.TempDir(), "clone")
	gitRun(t, "", "clone", "-q", origin, clone)
	gitRun(t, clone, "config", "board.me", strings.TrimSpace(gitRun(t, origin, "config", "board.me")))

	a := newAPI(t, clone)
	if got := want[boardRes](t, a.do("GET", "/api/board", nil), http.StatusOK); got.FirstPull != nil {
		t.Fatalf("before the pull: %+v", got.FirstPull)
	}
	a.s.FirstPull(ctx)
	got := want[boardRes](t, a.do("GET", "/api/board", nil), http.StatusOK)
	if got.FirstPull == nil || got.FirstPull.Remote != "origin" || got.FirstPull.Issues != 2 || got.FirstPull.Error != "" {
		t.Fatalf("first_pull: %+v", got.FirstPull)
	}
	list := want[struct{ Items []struct{ ID string } }](t, a.do("GET", "/api/issues?filter=is:open,closed", nil), http.StatusOK)
	if len(list.Items) != 2 {
		t.Errorf("issues after the pull: %+v", list.Items)
	}

	again := newAPI(t, clone)
	again.s.FirstPull(ctx)
	if got := want[boardRes](t, again.do("GET", "/api/board", nil), http.StatusOK); got.FirstPull != nil {
		t.Errorf("a second open: %+v", got.FirstPull)
	}
}

// An origin that can't be read is said, and nothing is created.
func TestFirstPullUnreadable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	gitRun(t, "", "init", "-q", dir)
	gitRun(t, dir, "remote", "add", "origin", filepath.Join(dir, "nowhere"))
	a := newAPI(t, dir)
	a.s.FirstPull(ctx)
	got := want[boardRes](t, a.do("GET", "/api/board", nil), http.StatusOK)
	if got.FirstPull == nil || got.FirstPull.Issues != 0 || got.FirstPull.Error == "" {
		t.Fatalf("first_pull: %+v", got.FirstPull)
	}
}
