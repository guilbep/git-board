package release_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/release"
	"github.com/guilbep/git-board/internal/release/releasetest"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// issue makes the forge's issue #n, with labels.
func issue(t *testing.T, r *release.Runner, n int, title string, labels ...string) {
	t.Helper()
	id := storetest.NewImportedIssue(t, r.B, title, fmt.Sprintf("https://github.com/org/app/issues/%d", n))
	if len(labels) == 0 {
		return
	}
	if _, err := r.B.ApplyLabelChange(id, labels, nil); err != nil {
		t.Fatal(err)
	}
}

// released tags the app's first commit v1.0.0, makes stage and prod run it,
// and returns it.
func released(t *testing.T, fx *releasetest.Fixture) string {
	t.Helper()
	first := releasetest.Git(t, fx.App, "rev-list", "--max-parents=0", "HEAD")
	fx.Tag(t, first, "v1.0.0")
	both(t, fx, "stage", release.Deployed{Commit: first, Version: "v1.0.0"})
	both(t, fx, "prod", release.Deployed{Commit: first, Version: "v1.0.0"})
	return first
}

// onDev makes dev run c.
func onDev(t *testing.T, fx *releasetest.Fixture, c string) {
	t.Helper()
	both(t, fx, "dev", release.Deployed{Commit: c, Version: "1.0.1-dev.1.g" + c[:7]})
}

func draft(t *testing.T, r *release.Runner, final bool) release.Draft {
	t.Helper()
	d, err := r.Draft(ctx, final)
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	return d
}

func TestDraftProposesPatchAndRC(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	first := released(t, fx)
	issue(t, r, 12, "a", "kind:fix")
	issue(t, r, 13, "b (#13)", "kind:chore")
	issue(t, r, 14, "c", "kind:feat")
	m1 := fx.Merge(t, "fix/12-a", "")
	m2 := fx.Merge(t, "chore/13-b", "Merge pull request #7 from org/chore/13-b")
	onDev(t, fx, m2)
	fx.Merge(t, "feat/14-c", "") // on the trunk, not in dev yet
	fx.Commit(t, "no branch")

	d := draft(t, r, false)
	if d.From != m2 || d.FromVersion != "1.0.1-dev.1.g"+m2[:7] {
		t.Errorf("from %.7s %s, want dev's", d.From, d.FromVersion)
	}
	if d.Previous == nil || d.Previous.Name != "v1.0.0" {
		t.Errorf("previous = %+v", d.Previous)
	}
	if d.Proposed != (release.Version{Major: 1, Minor: 0, Patch: 1}) || d.Tag != "v1.0.1-rc.1" {
		t.Errorf("proposed %v, tag %s; want 1.0.1, v1.0.1-rc.1", d.Proposed, d.Tag)
	}
	want := []release.DraftIssue{{Number: "#12", Title: "a", Kind: "fix", Merge: m1}, {Number: "#13", Title: "b", Kind: "chore", Merge: m2}}
	if !slices.Equal(d.Issues, want) || !strings.Contains(d.Notes, "#12") {
		t.Errorf("issues %+v, notes %q", d.Issues, d.Notes)
	}
	if notes := "### fix\n\n- #12 a\n\n### chore\n\n- #13 b"; d.Notes != notes {
		t.Errorf("notes = %q, want %q", d.Notes, notes)
	}
	if got := d.Undeployed; !slices.Equal(got, []string{"Merge branch 'feat/14-c' into dev"}) {
		t.Errorf("undeployed = %q", got)
	}
	checks := []release.Check{
		{OK: true, Text: "It contains what runs in prod, v1.0.0 (" + first[:7] + ")"},
		{OK: false, Text: "Images for " + m2[:7] + " exist on ghcr.test/app, quay.test/app: frontend, backend"},
		{OK: true, Text: "stage moves forward"},
	}
	if len(d.Checks) != len(checks) {
		t.Fatalf("checks = %+v", d.Checks)
	}
	for i, c := range checks {
		if got := d.Checks[i]; got.OK != c.OK || got.Text != c.Text || got.OK != (got.Detail == "") {
			t.Errorf("check %d = %+v, want %+v, a detail when not ok", i, got, c)
		}
	}
}

func TestDraftFeatIsMinor(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	released(t, fx)
	r.Branches.Kinds = map[string]string{"enhancement": "feat"} // a flat label, as the branch says it
	issue(t, r, 12, "a", "kind:fix")
	issue(t, r, 14, "c", "enhancement")
	fx.Merge(t, "fix/12-a", "")
	m := fx.Merge(t, "feat/14-c", "")
	onDev(t, fx, m)
	if d := draft(t, r, false); d.Proposed != (release.Version{Major: 1, Minor: 1}) || d.Tag != "v1.1.0-rc.1" {
		t.Errorf("proposed %v, tag %s; want 1.1.0, v1.1.0-rc.1", d.Proposed, d.Tag)
	} else if !strings.HasPrefix(d.Notes, "### enhancement\n\n- #14 c") {
		t.Errorf("notes = %q, want the feature first", d.Notes)
	}
	fx.Tag(t, m, "v1.1.0-rc.1")
	for _, reg := range []string{"ghcr.test/app", "quay.test/app"} {
		for _, img := range []string{"frontend", "backend"} {
			tools.Images[reg+"/"+img+":"+m] = releasetest.FakeImage{Digest: "sha256:" + img, Revision: m}
		}
	}
	d := draft(t, r, false)
	if d.Tag != "v1.1.0-rc.2" || !d.Checks[1].OK {
		t.Errorf("tag %s, images %+v; want rc.2, the images found", d.Tag, d.Checks[1])
	}
	if d := draft(t, r, true); d.Tag != "v1.1.0" || d.Checks[2].Text != "prod moves forward" {
		t.Errorf("final: tag %s, checks %+v", d.Tag, d.Checks)
	}
	issue(t, r, 15, "d", "breaking")
	onDev(t, fx, fx.Merge(t, "15-d", ""))
	if d := draft(t, r, false); d.Proposed != (release.Version{Major: 2}) {
		t.Errorf("breaking: proposed %v, want 2.0.0", d.Proposed)
	}
}

// Apps with no environment between dev and the last skip the rc.
func TestDraftWithoutCandidatesIsFinal(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t, "dev", "prod")
	first := releasetest.Git(t, fx.App, "rev-list", "--max-parents=0", "HEAD")
	fx.Tag(t, first, "v1.0.0")
	both(t, fx, "prod", release.Deployed{Commit: first, Version: "v1.0.0"})
	onDev(t, fx, fx.Commit(t, "fix"))
	if d := draft(t, r, false); d.Tag != "v1.0.1" || d.Checks[2].Text != "prod moves forward" {
		t.Errorf("tag %s, checks %+v; want the final, for prod", d.Tag, d.Checks)
	}
}

// A release carries the issues merged since the final before its version:
// a final on its candidate's commit carries the candidate's, not none.
func TestIssuesOfARelease(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	m := shipped(t, r, fx)
	rc := cut(t, r, release.Cut{Deploy: true})
	final, err := r.Promote(ctx, rc.Name)
	if err != nil {
		t.Fatal(err)
	}
	want := []release.DraftIssue{{Number: "#12", Title: "a", Kind: "fix", Merge: m}}
	for _, tag := range []release.Tag{rc, final} {
		if got, err := r.Issues(ctx, tag); err != nil || !slices.Equal(got, want) {
			t.Errorf("%s: %+v, %v; want %+v", tag.Name, got, err, want)
		}
	}
	first := releasetest.Git(t, fx.App, "rev-list", "--max-parents=0", "HEAD")
	if got, err := r.Issues(ctx, release.Tag{Name: "v1.0.0", Commit: first, Version: release.Version{Major: 1}}); err != nil || len(got) != 0 {
		t.Errorf("v1.0.0: %+v, %v; want none", got, err)
	}
}
