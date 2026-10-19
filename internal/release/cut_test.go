package release_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/release"
	"github.com/guilbep/git-board/internal/release/releasetest"
)

func cut(t *testing.T, r *release.Runner, c release.Cut) release.Tag {
	t.Helper()
	tg, err := r.Cut(ctx, c)
	if err != nil {
		t.Fatalf("Cut %+v: %v", c, err)
	}
	return tg
}

// shipped makes v1.0.0 (the first commit) run in stage and prod, and dev
// run a merge of fix/12-a (issue #12), which it returns.
func shipped(t *testing.T, r *release.Runner, fx *releasetest.Fixture) string {
	t.Helper()
	released(t, fx)
	issue(t, r, 12, "a", "kind:fix")
	m := fx.Merge(t, "fix/12-a", "")
	onDev(t, fx, m)
	return m
}

// pushed puts commit's image on the fake registry as a build would:
// <repo>:<commit>, and <repo>@<digest>.
func pushed(tools *releasetest.Tools, repo, commit, digest string) {
	img := releasetest.FakeImage{Digest: digest, Revision: commit}
	tools.Images[repo+":"+commit], tools.Images[repo+"@"+digest] = img, img
}

// records is the refs of the release records in repo: one per record made.
func records(t *testing.T, repo string) []string {
	t.Helper()
	return strings.Fields(releasetest.Git(t, repo, "for-each-ref", "--format=%(refname) %(objectname)", "refs/releases"))
}

func TestCutTagTakenWritesNothing(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	m := shipped(t, r, fx)
	other := cloneOf(t, fx.AppBare)
	r.Step = func(s release.Step) {
		if s.Name == "tag" && s.State == "running" { // another clone pushes it first
			releasetest.Git(t, other, "tag", "-a", "v1.0.1-rc.1", "-m", "theirs", m)
			releasetest.Git(t, other, "push", "-q", "origin", "refs/tags/v1.0.1-rc.1")
		}
	}
	before := heads(t, fx)
	_, err := r.Cut(ctx, release.Cut{Deploy: true})
	if !errors.Is(err, release.ErrTaken) {
		t.Fatalf("err = %v, want ErrTaken", err)
	}
	if len(tools.Tagged) != 0 || len(tools.Built) != 0 || len(tools.Charts) != 0 {
		t.Errorf("tagged %q, built %+v, charts %v after a taken tag", tools.Tagged, tools.Built, tools.Charts)
	}
	if recs, err := r.B.Releases(); err != nil || len(recs) != 0 {
		t.Errorf("records %+v, %v after a taken tag", recs, err)
	}
	if !slices.Equal(heads(t, fx), before) {
		t.Error("an overlay was written after a taken tag")
	}
	if got := releasetest.Git(t, fx.App, "tag", "-l", "v1.0.1-rc.1", "--format=%(contents:subject)"); got != "" {
		t.Errorf("local tag left: %q", got)
	}
	r.Step = nil
	if tg := cut(t, r, release.Cut{}); tg.Name != "v1.0.1-rc.2" {
		t.Errorf("next cut = %s, want v1.0.1-rc.2", tg.Name)
	}
}

func TestCutAndPromoteSameDigests(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	m := shipped(t, r, fx)
	rc := cut(t, r, release.Cut{Deploy: true})
	if rc.Name != "v1.0.1-rc.1" || rc.Commit != m {
		t.Fatalf("cut %s on %.7s, want v1.0.1-rc.1 on %.7s", rc.Name, rc.Commit, m)
	}
	if got := releasetest.Git(t, fx.AppBare, "rev-parse", rc.Name+"^{commit}"); got != m {
		t.Errorf("the authority's %s is on %.7s", rc.Name, got)
	}
	stage := envOf(t, status(t, r), "stage")
	if stage.Commit != m || stage.Version != rc.Name {
		t.Errorf("stage = %s %.7s", stage.Version, stage.Commit)
	}
	built := len(tools.Built)
	if built != 2 {
		t.Errorf("built %+v, want frontend and backend", tools.Built)
	}
	rec, ok, err := r.B.ReleaseFor(m)
	if err != nil || !ok || len(rec.Artifacts) != 4 || rec.Previous != "v1.0.0" || !strings.Contains(rec.Notes, "#12") ||
		rec.Chart.Version != rc.Name || rec.Chart.Digest != tools.Charts["oci://ghcr.test/app/helm "+rc.Name] {
		t.Errorf("record = %+v, %v, %v", rec, ok, err)
	}
	for _, a := range rec.Artifacts {
		if !slices.Contains(tools.Tagged, a.Registry+"/"+a.Image+"@"+a.Digest+" → "+rc.Name) {
			t.Errorf("%s/%s not tagged %s: %q", a.Registry, a.Image, rc.Name, tools.Tagged)
		}
	}
	if got := records(t, fx.AppBare); !slices.Equal(got, records(t, fx.App)) {
		t.Errorf("the authority's records %q, want ours", got)
	}

	if err := r.B.SetReleaseNotes(rec.ID, "Hand-edited."); err != nil { // promoting keeps them
		t.Fatal(err)
	}
	charts := 0 // R19: once, by the deploy (tools.Charts is keyed by version: a second push wouldn't show)
	r.Step = func(s release.Step) {
		if s.Name == "chart" && s.State == "done" {
			charts++
		}
	}
	final, err := r.Promote(ctx, rc.Name)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if r.Step = nil; charts != 1 {
		t.Errorf("Promote packaged the chart %d times, want once", charts)
	}
	if final.Name != "v1.0.1" || final.Commit != rc.Commit {
		t.Errorf("promoted %s on %.7s", final.Name, final.Commit)
	}
	if len(tools.Built) != built {
		t.Errorf("Promote built %+v", tools.Built[built:])
	}
	st := status(t, r)
	prod := envOf(t, st, "prod")
	if prod.Commit != m || prod.Version != "v1.0.1" {
		t.Errorf("prod = %s %.7s", prod.Version, prod.Commit)
	}
	for i, tg := range envOf(t, st, "stage").Targets {
		for name, img := range tg.Deployed.Images {
			if got := prod.Targets[i].Deployed.Images[name]; got.Digest != img.Digest || got.Tag != "v1.0.1" {
				t.Errorf("prod %s = %+v, want stage's digest %s", name, got, img.Digest)
			}
		}
	}
	if got := records(t, fx.App); len(got) != 2 { // one ref, one object
		t.Errorf("records %q, want the rc's only", got)
	}
	if after, _, _ := r.B.ReleaseFor(m); after.ID != rec.ID || after.Notes != "Hand-edited." {
		t.Errorf("record after promote = %+v", after)
	}
	if tools.Charts["oci://ghcr.test/app/helm v1.0.1"] == "" || !slices.Contains(tools.Tagged, rec.Artifacts[0].Registry+"/"+rec.Artifacts[0].Image+"@"+rec.Artifacts[0].Digest+" → v1.0.1") {
		t.Errorf("charts %v, tagged %q; want both at v1.0.1", tools.Charts, tools.Tagged)
	}
	if _, err := r.Promote(ctx, rc.Name); !errors.Is(err, release.ErrTaken) {
		t.Errorf("second promote: err = %v, want ErrTaken", err)
	}
}

// A final on a commit that has a record keeps it; notes given replace its
// notes.
func TestCutFinalKeepsTheRecord(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	shipped(t, r, fx)
	rc := cut(t, r, release.Cut{Deploy: true})
	cut(t, r, release.Cut{Final: true, Commit: rc.Commit[:7], Notes: "Edited."})
	if got := records(t, fx.App); len(got) != 2 {
		t.Errorf("records %q, want one", got)
	}
	if rec, _, _ := r.B.ReleaseFor(rc.Commit); rec.Notes != "Edited." {
		t.Errorf("notes = %q", rec.Notes)
	}
}

func TestFinalNeedsACandidate(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	m := shipped(t, r, fx)
	want := "no candidate of " + m[:7] + " ran in stage: cut one and deploy it there first"
	if _, err := r.Cut(ctx, release.Cut{Final: true}); err == nil || err.Error() != want {
		t.Errorf("final never in stage: err = %v, want %q", err, want)
	}
	fx.Tag(t, m, "v1.0.1-rc.1") // a candidate, never deployed
	if _, err := r.Cut(ctx, release.Cut{Final: true}); err == nil || err.Error() != want {
		t.Errorf("final of a candidate never in stage: err = %v, want %q", err, want)
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != "v1.0.0\nv1.0.1-rc.1" || len(tools.Tagged) != 0 {
		t.Errorf("tags %q, tagged %q after a refusal", got, tools.Tagged)
	}
}

func TestCutRefusesYanked(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	m := shipped(t, r, fx)
	record(t, r, tools, m, gitbugio.ChartArtifact{})
	rec, _, _ := r.B.ReleaseFor(m)
	if err := r.B.YankRelease(rec.ID, "corrupts data"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Cut(ctx, release.Cut{}); err == nil || !strings.Contains(err.Error(), "corrupts data") {
		t.Errorf("err = %v, want the yank's reason", err)
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != "v1.0.0" {
		t.Errorf("tags %q after a refusal", got)
	}
}

func TestHotfixRules(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	p := fx.Commit(t, "p")
	fx.Tag(t, p, "v1.0.1")
	both(t, fx, "stage", release.Deployed{Commit: p, Version: "v1.0.1"})
	both(t, fx, "prod", release.Deployed{Commit: p, Version: "v1.0.1"})
	issue(t, r, 12, "a", "kind:feat")
	onDev(t, fx, fx.Merge(t, "fix/12-a", ""))
	releasetest.Git(t, fx.App, "checkout", "-q", "-b", "hotfix/1.0.2", p)
	h := releasetest.Commit(t, fx.App, "hotfix")
	releasetest.Git(t, fx.App, "push", "-q", "origin", "hotfix/1.0.2")
	releasetest.Git(t, fx.App, "checkout", "-q", "dev")

	hf := cut(t, r, release.Cut{Final: true, Hotfix: true, Commit: h, Deploy: true, Notes: "Emergency fix."})
	if hf.Name != "v1.0.2" || hf.Commit != h {
		t.Errorf("hotfix %s on %.7s, want v1.0.2 on %.7s", hf.Name, hf.Commit, h)
	}
	if rec, _, _ := r.B.ReleaseFor(h); rec.Notes != "Emergency fix." {
		t.Errorf("new record's notes = %q, want the cut's", rec.Notes)
	}
	if prod := envOf(t, status(t, r), "prod"); prod.Commit != h || prod.Version != "v1.0.2" {
		t.Errorf("prod = %s %.7s", prod.Version, prod.Commit)
	}
	if len(tools.Built) != 2 || tools.Built[0].Revision != h {
		t.Errorf("built %+v, want the hotfix's images", tools.Built)
	}
	_, err := r.Cut(ctx, release.Cut{})
	if want := "it doesn't contain what runs in prod (v1.0.2): merge hotfix/1.0.2 into dev first"; err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != "v1.0.1\nv1.0.2" {
		t.Errorf("tags %q after a refusal", got)
	}

	// merged back, the hotfix lifts the refusal and names no issue (#1)
	issue(t, r, 1, "old", "breaking")
	releasetest.Git(t, fx.App, "merge", "-q", "--no-ff", "-m", "Merge branch 'hotfix/1.0.2' into dev", "hotfix/1.0.2")
	releasetest.Git(t, fx.App, "push", "-q", "origin", "dev")
	onDev(t, fx, releasetest.Git(t, fx.App, "rev-parse", "HEAD"))
	d := draft(t, r, false)
	if d.Previous == nil || d.Previous.Name != "v1.0.2" {
		t.Fatalf("after the merge back: previous %+v, want v1.0.2", d.Previous)
	}
	if len(d.Issues) != 1 || d.Issues[0].Number != "#12" || d.Proposed != (release.Version{Major: 1, Minor: 1}) {
		t.Errorf("after the merge back: issues %+v, proposed %v", d.Issues, d.Proposed)
	}
	if tg := cut(t, r, release.Cut{}); tg.Name != "v1.1.0-rc.1" {
		t.Errorf("cut %s, want v1.1.0-rc.1", tg.Name)
	}
}

func TestRollbackComments(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	first := released(t, fx)
	m := fx.Commit(t, "fix")
	fx.Tag(t, m, "v1.0.1")
	record(t, r, tools, m, gitbugio.ChartArtifact{Version: "v1.0.1", Digest: "sha256:chart"})
	both(t, fx, "prod", release.Deployed{Commit: m, Version: "v1.0.1"})
	before := heads(t, fx)
	err := r.Rollback(ctx, "prod", "v1.0.0", "login broken")
	if want := "frontend for " + first[:7] + " is in no registry: rollback ships what ran, it doesn't build"; err == nil || err.Error() != want {
		t.Errorf("rollback to images no registry has: err = %v, want %q", err, want)
	}
	if len(tools.Built) != 0 || !slices.Equal(heads(t, fx), before) {
		t.Errorf("a refused rollback built %+v or wrote an overlay", tools.Built)
	}
	for _, img := range []string{"frontend", "backend"} { // on one registry: the other gets a copy
		pushed(tools, "ghcr.test/app/"+img, first, "sha256:"+img)
	}
	if err := r.Rollback(ctx, "prod", "v1.0.0", "login broken"); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if len(tools.Built) != 0 || len(tools.Copied) != 2 {
		t.Errorf("built %+v, copied %q; want copies only", tools.Built, tools.Copied)
	}
	if prod := envOf(t, status(t, r), "prod"); prod.Commit != first || prod.Version != "v1.0.0" {
		t.Errorf("prod = %s %.7s, want v1.0.0", prod.Version, prod.Commit)
	}
	rec, _, _ := r.B.ReleaseFor(m)
	if len(rec.Comments) != 1 || rec.Comments[0].Message != "Rolled back from prod to v1.0.0: login broken" {
		t.Errorf("comments = %+v", rec.Comments)
	}
	if got := records(t, fx.AppBare); !slices.Equal(got, records(t, fx.App)) {
		t.Errorf("the authority's records %q, want ours", got)
	}
	if err := r.Rollback(ctx, "prod", "v9.9.9", "x"); err == nil {
		t.Error("rollback to an unknown tag: no error")
	}
}

// A split environment rolled back comments on each release it ran.
func TestRollbackSplit(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	first := released(t, fx)
	record(t, r, tools, first, gitbugio.ChartArtifact{})
	m1 := fx.Commit(t, "one")
	m2 := fx.Commit(t, "two")
	fx.Tag(t, m1, "v1.0.1")
	fx.Tag(t, m2, "v1.0.2")
	record(t, r, tools, m1, gitbugio.ChartArtifact{})
	record(t, r, tools, m2, gitbugio.ChartArtifact{})
	fx.SetOverlay(t, 0, "prod", release.Deployed{Commit: m2, Version: "v1.0.2"})
	fx.SetOverlay(t, 1, "prod", release.Deployed{Commit: m1, Version: "v1.0.1"})
	if err := r.Rollback(ctx, "prod", "v1.0.0", "both broken"); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if prod := envOf(t, status(t, r), "prod"); prod.Split || prod.Commit != first {
		t.Errorf("prod = %+v, want both targets on v1.0.0", prod)
	}
	for _, c := range []string{m1, m2} {
		if rec, _, _ := r.B.ReleaseFor(c); len(rec.Comments) != 1 || rec.Comments[0].Message != "Rolled back from prod to v1.0.0: both broken" {
			t.Errorf("%.7s comments = %+v, want one", c, rec.Comments)
		}
	}
}

// Promote ships what ran: an image no registry has refuses it before the
// tag; one a registry lacks is copied.
func TestPromoteNeverBuilds(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	m := shipped(t, r, fx)
	fx.Tag(t, m, "v1.0.1-rc.1")
	both(t, fx, "stage", release.Deployed{Commit: m, Version: "v1.0.1-rc.1"})
	pushed(tools, "ghcr.test/app/frontend", m, "sha256:fe")
	_, err := r.Promote(ctx, "v1.0.1-rc.1") // backend was deleted from every registry
	if want := "backend for " + m[:7] + " is in no registry: promote ships what ran, it doesn't build"; err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != "v1.0.0\nv1.0.1-rc.1" || len(tools.Built) != 0 || len(tools.Tagged) != 0 {
		t.Errorf("tags %q, built %+v, tagged %q after a refusal", got, tools.Built, tools.Tagged)
	}
	pushed(tools, "quay.test/app/backend", m, "sha256:be")
	if _, err := r.Promote(ctx, "v1.0.1-rc.1"); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if len(tools.Built) != 0 || len(tools.Copied) != 2 {
		t.Errorf("built %+v, copied %q; want copies only", tools.Built, tools.Copied)
	}
}

func TestOfflineRefusesWrites(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	m := shipped(t, r, fx)
	fx.Tag(t, m, "v1.0.1-rc.1")
	r.Offline = true
	if d := draft(t, r, false); d.From != m {
		t.Errorf("offline draft from %.7s, want dev's", d.From)
	}
	_, cutErr := r.Cut(ctx, release.Cut{})
	_, promoteErr := r.Promote(ctx, "v1.0.1-rc.1")
	rollbackErr := r.Rollback(ctx, "prod", "v1.0.0", "x")
	for name, err := range map[string]error{"cut": cutErr, "promote": promoteErr, "rollback": rollbackErr} {
		if err == nil || !strings.Contains(err.Error(), "offline") {
			t.Errorf("offline %s: err = %v", name, err)
		}
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != "v1.0.0\nv1.0.1-rc.1" || len(tools.Tagged) != 0 {
		t.Errorf("tags %q, tagged %q offline", got, tools.Tagged)
	}
}

func TestDevOnlyNeverTags(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t, "dev")
	c := fx.Commit(t, "fix")
	both(t, fx, "dev", release.Deployed{Commit: c, Version: "0.0.1-dev.2.g" + c[:7]})
	want := "dev is the only environment: dev-only apps don't release"
	_, draftErr := r.Draft(ctx, false)
	_, cutErr := r.Cut(ctx, release.Cut{})
	_, promoteErr := r.Promote(ctx, "v0.0.1-rc.1")
	for name, err := range map[string]error{"draft": draftErr, "cut": cutErr, "promote": promoteErr} {
		if err == nil || err.Error() != want {
			t.Errorf("%s: err = %v, want %q", name, err, want)
		}
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != "" {
		t.Errorf("tags %q", got)
	}
}

// With Deploy, a cut whose environment wouldn't take it is refused before
// the tag; a hotfix may leave the line of an environment but the last.
func TestCutPrechecksDeploy(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	first := releasetest.Git(t, fx.App, "rev-list", "--max-parents=0", "HEAD")
	fx.Tag(t, first, "v1.0.0")
	both(t, fx, "prod", release.Deployed{Commit: first, Version: "v1.0.0"})
	m := fx.Commit(t, "fix")
	onDev(t, fx, m)
	_, err := r.Cut(ctx, release.Cut{Deploy: true}) // stage not adopted
	if !errors.Is(err, release.ErrNotAdopted) {
		t.Errorf("err = %v, want ErrNotAdopted", err)
	}
	n := fx.Commit(t, "newer")
	both(t, fx, "stage", release.Deployed{Commit: n, Version: "v1.0.1-rc.1"})
	if _, err := r.Cut(ctx, release.Cut{Deploy: true}); !errors.Is(err, release.ErrOlder) || !strings.Contains(err.Error(), "stage") {
		t.Errorf("err = %v, want ErrOlder naming stage", err)
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != "v1.0.0" || len(tools.Built) != 0 {
		t.Errorf("tags %q, built %+v after refusals", got, tools.Built)
	}
	if tg := cut(t, r, release.Cut{Deploy: true, Hotfix: true}); envOf(t, status(t, r), "stage").Commit != m || tg.Commit != m {
		t.Errorf("a hotfix candidate didn't go to stage")
	}
}
