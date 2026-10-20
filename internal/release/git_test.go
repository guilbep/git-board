package release_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/release"
	"github.com/guilbep/git-board/internal/release/releasetest"
)

var ctx = context.Background()

const devOverlay = "images:\n  - name: app\n    newTag: old\n"

func identity(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_SYSTEM": os.DevNull,
		"GIT_AUTHOR_NAME": "bot", "GIT_AUTHOR_EMAIL": "bot@example.com",
		"GIT_COMMITTER_NAME": "bot", "GIT_COMMITTER_EMAIL": "bot@example.com",
	} {
		t.Setenv(k, v)
	}
}

func cloneOf(t *testing.T, bare string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "clone")
	releasetest.Git(t, filepath.Dir(dir), "clone", bare, dir)
	return dir
}

func pushOtherCommit(t *testing.T, bare, file string) {
	t.Helper()
	dir := cloneOf(t, bare)
	if err := os.WriteFile(filepath.Join(dir, file), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	releasetest.Git(t, dir, "commit", "-am", "other app")
	releasetest.Git(t, dir, "push", "origin", "HEAD")
}

func TestPushTagTaken(t *testing.T) {
	identity(t)
	work, bare := releasetest.NewRemote(t)
	other := cloneOf(t, bare)
	c := releasetest.Commit(t, work, "a")
	releasetest.Git(t, work, "push", "origin", "HEAD:dev")
	releasetest.Git(t, other, "fetch", "origin")
	taken, err := release.PushTag(ctx, other, "origin", release.Tag{Name: "v1.0.0-rc.1", Commit: c, Message: "x"})
	if err != nil || taken {
		t.Fatalf("first push: taken=%v err=%v", taken, err)
	}
	taken, err = release.PushTag(ctx, work, "origin", release.Tag{Name: "v1.0.0-rc.1", Commit: c, Message: "y"})
	if err != nil || !taken {
		t.Fatalf("second push: taken=%v err=%v", taken, err)
	}
	if got := releasetest.Git(t, work, "tag", "-l"); got != "" {
		t.Errorf("local tag left: %q", got)
	}
	if got := releasetest.Git(t, bare, "tag", "-l", "-n1"); !strings.Contains(got, "x") {
		t.Errorf("remote tag moved: %q", got)
	}
}

// A clone is renamed in whole: two opening the same repo at once both
// read it.
func TestOpenConfigRepoClonesWhole(t *testing.T) {
	identity(t)
	bare := releasetest.NewConfigRepo(t, map[string]string{"epfl/app/overlays/dev/kustomization.yaml": devOverlay})
	cache := t.TempDir()
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			c, err := release.OpenConfigRepo(ctx, cache, bare)
			if err == nil {
				_, err = c.Read(ctx, "epfl/app/overlays/dev")
			}
			errs <- err
		}()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}

func TestUpdateReplaysOnRejectedPush(t *testing.T) {
	identity(t)
	bare := releasetest.NewConfigRepo(t, map[string]string{
		"epfl/app/overlays/dev/kustomization.yaml": devOverlay,
		"epfl/other/README":                        "x",
	})
	c, err := release.OpenConfigRepo(ctx, t.TempDir(), bare)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	commit, err := c.Update(ctx, "epfl/app/overlays/dev", "msg", func(o *release.Overlay) (bool, error) {
		calls++
		if calls == 1 {
			pushOtherCommit(t, bare, "epfl/other/README")
		}
		return o.Set(release.Deployed{Commit: "abc", Version: "1.0.0"}, "", nil)
	})
	if err != nil || commit == "" || calls != 2 {
		t.Fatalf("commit=%q calls=%d err=%v", commit, calls, err)
	}
	if got := releasetest.Git(t, bare, "log", "--format=%s", "main"); got != "msg\nother app\ninit" {
		t.Errorf("log = %q", got)
	}
	if got := releasetest.Git(t, bare, "rev-parse", "main"); got != commit {
		t.Errorf("main %s != commit %s", got, commit)
	}
}

func TestUpdateNothingToCommit(t *testing.T) {
	identity(t)
	bare := releasetest.NewConfigRepo(t, map[string]string{"a/kustomization.yaml": devOverlay})
	c, _ := release.OpenConfigRepo(ctx, t.TempDir(), bare)
	commit, err := c.Update(ctx, "a", "msg", func(*release.Overlay) (bool, error) { return false, nil })
	if err != nil || commit != "" {
		t.Fatalf("commit=%q err=%v", commit, err)
	}
}

func TestHistory(t *testing.T) {
	identity(t)
	bare := releasetest.NewConfigRepo(t, map[string]string{"a/kustomization.yaml": devOverlay})
	c, _ := release.OpenConfigRepo(ctx, t.TempDir(), bare)
	_, err := c.Update(ctx, "a", "chore(manifest): update app (dev) v1.2.3 [by me]", func(o *release.Overlay) (bool, error) {
		return o.Set(release.Deployed{Commit: "abc", Version: "1.2.3"}, "", nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	h, err := c.History(ctx, "a", 5)
	if err != nil || len(h) != 2 {
		t.Fatalf("history = %+v err=%v", h, err)
	}
	if h[0].Version != "v1.2.3" || h[0].Author != "bot" || h[1].Version != "" || h[0].Date.IsZero() {
		t.Errorf("history = %+v", h)
	}
}

func TestIsAncestorEqual(t *testing.T) {
	work, _ := releasetest.NewRemote(t)
	a := releasetest.Git(t, work, "rev-parse", "HEAD")
	b := releasetest.Commit(t, work, "b")
	for _, c := range []struct {
		x, y string
		want bool
	}{{a, a, true}, {a, b, true}, {b, a, false}} {
		got, err := release.IsAncestor(ctx, work, c.x, c.y)
		if err != nil || got != c.want {
			t.Errorf("IsAncestor(%.7s, %.7s) = %v, %v", c.x, c.y, got, err)
		}
	}
}

func TestReleaseTagsSkipsOthers(t *testing.T) {
	identity(t)
	work, _ := releasetest.NewRemote(t)
	c := releasetest.Git(t, work, "rev-parse", "HEAD")
	for _, n := range []string{"v1.0.0", "v1.1.0-rc.1", "docs"} {
		releasetest.Git(t, work, "tag", "-a", n, "-m", "m "+n)
	}
	tags, err := release.ReleaseTags(ctx, work)
	if err != nil || len(tags) != 2 || tags[0].Name != "v1.1.0-rc.1" || tags[1].Name != "v1.0.0" {
		t.Fatalf("tags = %+v err=%v", tags, err)
	}
	if tags[0].Commit != c || tags[0].Message != "m v1.1.0-rc.1" || tags[0].Date.IsZero() {
		t.Errorf("tag = %+v", tags[0])
	}
	if at := release.TagsAt(tags, c); len(at) != 2 {
		t.Errorf("TagsAt = %+v", at)
	}
}

func TestReadSeesRemoteOnlyAfterFetch(t *testing.T) {
	identity(t)
	bare := releasetest.NewConfigRepo(t, map[string]string{"a/kustomization.yaml": devOverlay})
	c1, _ := release.OpenConfigRepo(ctx, t.TempDir(), bare)
	c2, _ := release.OpenConfigRepo(ctx, t.TempDir(), bare)
	if _, err := c2.Update(ctx, "a", "msg", func(o *release.Overlay) (bool, error) {
		return o.Set(release.Deployed{Commit: "abc", Version: "9.9.9"}, "", nil)
	}); err != nil {
		t.Fatal(err)
	}
	o, err := c1.Read(ctx, "a")
	if err != nil || o.Deployed("").Version != "" {
		t.Fatalf("before Fetch: %+v err=%v", o, err)
	}
	if h, _ := c1.History(ctx, "a", 5); len(h) != 1 {
		t.Errorf("history before Fetch = %+v", h)
	}
	if err := c1.Fetch(ctx); err != nil {
		t.Fatal(err)
	}
	o, err = c1.Read(ctx, "a")
	if err != nil || o.Deployed("").Version != "9.9.9" {
		t.Fatalf("after Fetch: %+v err=%v", o, err)
	}
}

func TestPushTagFailureLeavesNoLocalTag(t *testing.T) {
	identity(t)
	work, _ := releasetest.NewRemote(t)
	c := releasetest.Git(t, work, "rev-parse", "HEAD")
	taken, err := release.PushTag(ctx, work, filepath.Join(t.TempDir(), "nope.git"), release.Tag{Name: "v1.0.0", Commit: c, Message: "x"})
	if err == nil || taken {
		t.Fatalf("taken=%v err=%v", taken, err)
	}
	if got := releasetest.Git(t, work, "tag", "-l"); got != "" {
		t.Errorf("local tag left: %q", got)
	}
}

func TestMergesParsesBothSubjects(t *testing.T) {
	_, _, fx := releasetest.NewRunner(t)
	from := fx.Commit(t, "not a merge")
	a := fx.Merge(t, "fix/1-a", "")
	b := fx.Merge(t, "feat/2-b", "Merge pull request #9 from org/feat/2-b")
	c := fx.Merge(t, "chore/3-c", "Release the hounds")
	want := []release.Merge{
		{Commit: a, Subject: "Merge branch 'fix/1-a' into dev", Branch: "fix/1-a"},
		{Commit: b, Subject: "Merge pull request #9 from org/feat/2-b", Branch: "feat/2-b"},
		{Commit: c, Subject: "Release the hounds"},
	}
	if ms, err := release.Merges(ctx, fx.App, from, "dev"); err != nil || !slices.Equal(ms, want) {
		t.Errorf("Merges since %.7s = %+v, %v\nwant %+v", from, ms, err, want)
	}
	if ms, err := release.Merges(ctx, fx.App, "", "dev"); err != nil || !slices.Equal(ms, want) {
		t.Errorf("all Merges = %+v, %v", ms, err)
	}
	if ms, err := release.Merges(ctx, fx.App, c, "dev"); err != nil || ms != nil {
		t.Errorf("Merges since the head = %+v, %v", ms, err)
	}
}
