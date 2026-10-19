package release_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/release"
	"github.com/guilbep/git-board/internal/release/releasetest"
)

func TestNotesFinalsNewestFirst(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	shipped(t, r, fx)
	rc := cut(t, r, release.Cut{Deploy: true})
	if _, err := r.Promote(ctx, rc.Name); err != nil {
		t.Fatal(err)
	}
	got, err := r.Notes(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "## v1.0.1 (") || strings.Contains(got, "rc.1") || !strings.Contains(got, "#12") {
		t.Errorf("finals:\n%s", got)
	}
	all, err := r.Notes(ctx, true)
	if i, j := strings.Index(all, "## v1.0.1 ("), strings.Index(all, "## v1.0.1-rc.1 ("); err != nil || i < 0 || j < i {
		t.Errorf("all: %v\n%s", err, all)
	}
}

func TestYankPushes(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	shipped(t, r, fx)
	rc := cut(t, r, release.Cut{Deploy: true})
	before := records(t, fx.AppBare)
	if err := r.Yank(ctx, rc.Name, "bad"); err != nil {
		t.Fatal(err)
	}
	rec, _, err := r.B.ReleaseFor(rc.Commit)
	if err != nil || rec.Yanked != "bad" {
		t.Errorf("record = %+v, %v", rec, err)
	}
	if slices.Equal(records(t, fx.AppBare), before) {
		t.Error("the remote's records didn't move")
	}
	if got, _ := r.Notes(ctx, true); !strings.Contains(got, "yanked: bad") {
		t.Errorf("notes lack the yank:\n%s", got)
	}
	if err := r.Yank(ctx, "v9.9.9", "x"); err == nil {
		t.Error("yank of an unknown tag succeeded")
	}
}

func TestSetNotesPushes(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	shipped(t, r, fx)
	rc := cut(t, r, release.Cut{Deploy: true})
	before := records(t, fx.AppBare)
	if err := r.SetNotes(ctx, rc.Name, "### fix\n\n- #12 better"); err != nil {
		t.Fatal(err)
	}
	if rec, _, err := r.B.ReleaseFor(rc.Commit); err != nil || rec.Notes != "### fix\n\n- #12 better" {
		t.Errorf("record = %+v, %v", rec, err)
	}
	if slices.Equal(records(t, fx.AppBare), before) {
		t.Error("the remote's records didn't move")
	}
	if err := r.SetNotes(ctx, "v1.0.0", "x"); err == nil || err.Error() != "v1.0.0 has no release record" {
		t.Errorf("notes of a tag without a record: %v", err)
	}
}

func TestCommentPushes(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	shipped(t, r, fx)
	rc := cut(t, r, release.Cut{Deploy: true})
	before := records(t, fx.AppBare)
	if err := r.Comment(ctx, rc.Name, "looks good in stage"); err != nil {
		t.Fatal(err)
	}
	rec, _, err := r.B.ReleaseFor(rc.Commit)
	if err != nil || len(rec.Comments) != 1 || rec.Comments[0].Message != "looks good in stage" {
		t.Errorf("record = %+v, %v", rec, err)
	}
	if slices.Equal(records(t, fx.AppBare), before) {
		t.Error("the remote's records didn't move")
	}
	r.Offline = true
	if err := r.Comment(ctx, rc.Name, "x"); err == nil || !strings.HasPrefix(err.Error(), "offline") {
		t.Errorf("offline comment: %v", err)
	}
}
