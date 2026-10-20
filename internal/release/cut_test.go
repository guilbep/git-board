package release_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

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
	_, deployErr := r.Cut(ctx, release.Cut{Deploy: true})
	_, notesErr := r.Cut(ctx, release.Cut{Notes: "x"})
	_, hotfixErr := r.Cut(ctx, release.Cut{Hotfix: true})
	_, promoteErr := r.Promote(ctx, "v1.0.1-rc.1")
	rollbackErr := r.Rollback(ctx, "prod", "v1.0.0", "x")
	for name, err := range map[string]error{"cut and deploy": deployErr, "cut with notes": notesErr, "hotfix": hotfixErr, "promote": promoteErr, "rollback": rollbackErr} {
		if err == nil || !strings.Contains(err.Error(), "offline") {
			t.Errorf("offline %s: err = %v", name, err)
		}
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != "v1.0.0\nv1.0.1-rc.1" || len(tools.Tagged) != 0 {
		t.Errorf("tags %q, tagged %q offline", got, tools.Tagged)
	}
	if p, err := release.Pending(ctx, fx.App); err != nil || len(p) != 0 {
		t.Errorf("pending %+v, %v after refusals", p, err)
	}
}

// unplug moves the app's remote and the config repos away, as a lost
// network would, until the returned func puts them back.
func unplug(t *testing.T, fx *releasetest.Fixture) func() {
	t.Helper()
	dirs := append([]string{fx.AppBare}, fx.ConfigBare...)
	for _, d := range dirs {
		if err := os.Rename(d, d+".away"); err != nil {
			t.Fatal(err)
		}
	}
	return func() {
		t.Helper()
		for _, d := range dirs {
			if err := os.Rename(d+".away", d); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// cutOffline cuts as release cut --offline --no-deploy, unplugged, once an
// online status has left the config repos' caches.
func cutOffline(t *testing.T, r *release.Runner, fx *releasetest.Fixture) release.Tag {
	t.Helper()
	status(t, r)
	plug := unplug(t, fx)
	defer plug()
	r.Offline = true
	defer func() { r.Offline = false }()
	return cut(t, r, release.Cut{})
}

func TestOfflineCutWritesOnlyPending(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	m := shipped(t, r, fx)
	before := heads(t, fx)
	tg := cutOffline(t, r, fx)
	if tg.Name != "v1.0.1-rc.1" || tg.Commit != m {
		t.Errorf("cut %s on %.7s, want v1.0.1-rc.1 on %.7s", tg.Name, tg.Commit, m)
	}
	if p, err := release.Pending(ctx, fx.App); err != nil || len(p) != 1 || p[0].Name != tg.Name || p[0].Commit != m {
		t.Errorf("pending %+v, %v", p, err)
	}
	for _, repo := range []string{fx.App, fx.AppBare} {
		if got := releasetest.Git(t, repo, "tag", "-l"); got != "v1.0.0" {
			t.Errorf("%s has tags %q", repo, got)
		}
	}
	if len(tools.Built) != 0 || len(tools.Tagged) != 0 || len(tools.Charts) != 0 {
		t.Errorf("built %+v, tagged %q, charts %v offline", tools.Built, tools.Tagged, tools.Charts)
	}
	if recs, err := r.B.Releases(); err != nil || len(recs) != 0 {
		t.Errorf("records %+v, %v offline", recs, err)
	}
	if !slices.Equal(heads(t, fx), before) {
		t.Error("an overlay moved offline")
	}
	plug := unplug(t, fx)
	r.Offline = true
	_, err := r.Cut(ctx, release.Cut{Final: true})
	r.Offline = false
	plug()
	if err == nil || !strings.Contains(err.Error(), tg.Name) {
		t.Errorf("second offline cut: err = %v, want it to name %s", err, tg.Name)
	}
}

// Online, a cut pushes the cut made offline and stops there: it was that
// cut. The tag is made at the push, then the images, the chart and the
// record with the notes drafted then; it deploys nowhere, as it was cut.
func TestOnlineFinishesOfflineCut(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	m := shipped(t, r, fx)
	pending := cutOffline(t, r, fx)
	before, online := heads(t, fx), time.Now().Unix()
	tg := cut(t, r, release.Cut{Deploy: true})
	if tg.Name != pending.Name || tg.Commit != m {
		t.Errorf("cut %s on %.7s, want the pending %s on %.7s", tg.Name, tg.Commit, pending.Name, m)
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != "v1.0.0\nv1.0.1-rc.1" {
		t.Errorf("tags %q", got)
	}
	got := releasetest.Git(t, fx.AppBare, "for-each-ref", "--format=%(creatordate:unix) %(contents:subject)", "refs/tags/"+tg.Name)
	date, subject, _ := strings.Cut(got, " ")
	if n, _ := strconv.ParseInt(date, 10, 64); n < online || subject != tg.Name+" (git-board)" {
		t.Errorf("tag %q, want made at the push (%d) by git-board", got, online)
	}
	if p, err := release.Pending(ctx, fx.App); err != nil || len(p) != 0 {
		t.Errorf("pending %+v, %v once pushed", p, err)
	}
	rec, ok, err := r.B.ReleaseFor(m)
	if err != nil || !ok || len(rec.Artifacts) != 4 || rec.Previous != "v1.0.0" || !strings.Contains(rec.Notes, "#12") ||
		rec.Chart.Version != tg.Name || rec.Chart.Digest != tools.Charts["oci://ghcr.test/app/helm "+tg.Name] {
		t.Errorf("record = %+v, %v, %v", rec, ok, err)
	}
	if got := records(t, fx.AppBare); len(got) != 2 { // one ref, its object
		t.Errorf("pushed records %q", got)
	}
	if !slices.Equal(heads(t, fx), before) {
		t.Error("finishing a cut made without a deploy deployed")
	}
}

// A cut made offline that is pushed but doesn't finish is said, not
// returned: a rollback, a promotion or a deploy never waits on it.
func TestOfflineCutUnfinishedNeverBlocks(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	m := shipped(t, r, fx)
	tg := cutOffline(t, r, fx)
	tools.FailScan["ghcr.test/app/backend:scan-"+m] = true
	said := saidBy(r)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v, want the unfinished cut said, not returned", err)
	}
	if !slices.ContainsFunc(*said, func(s string) bool {
		return strings.HasPrefix(s, "pushed "+tg.Name+", cut offline, but didn't finish: ")
	}) {
		t.Errorf("said %q", *said)
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != "v1.0.0\nv1.0.1-rc.1" {
		t.Errorf("tags %q", got)
	}
	if p, err := release.Pending(ctx, fx.App); err != nil || len(p) != 0 {
		t.Errorf("pending %+v, %v once pushed", p, err)
	}
}

// An online cut that pushed the cut made offline returns its failure to
// finish: it was that cut.
func TestOnlineCutReturnsOfflineCutUnfinished(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	m := shipped(t, r, fx)
	pending := cutOffline(t, r, fx)
	tools.FailScan["ghcr.test/app/backend:scan-"+m] = true
	tg, err := r.Cut(ctx, release.Cut{})
	if tg.Name != pending.Name || err == nil || !strings.Contains(err.Error(), "vulnerabilities") {
		t.Errorf("Cut = %s, %v, want %s and its scan's failure", tg.Name, err, pending.Name)
	}
}

// A cut made offline whose push fails, its name not taken, is kept for
// the next online command, and said; a cut stops on it, as it would take
// its name.
func TestOfflineCutKeptOnAFailedPush(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	shipped(t, r, fx)
	tg := cutOffline(t, r, fx)
	hook := filepath.Join(fx.AppBare, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nif grep -q ' refs/tags/'; then echo no tags today >&2; exit 1; fi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	said := saidBy(r)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v, want the failed push said, not returned", err)
	}
	if !slices.ContainsFunc(*said, func(s string) bool { return strings.HasPrefix(s, "kept "+tg.Name+", cut offline: ") }) {
		t.Errorf("said %q", *said)
	}
	if _, err := r.Cut(ctx, release.Cut{}); err == nil || !strings.Contains(err.Error(), tg.Name) {
		t.Errorf("Cut: %v, want it stopped on the kept %s", err, tg.Name)
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != "v1.0.0" {
		t.Errorf("tags %q while pushes fail", got)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != "v1.0.0\nv1.0.1-rc.1" {
		t.Errorf("tags %q once pushes pass", got)
	}
	if p, err := release.Pending(ctx, fx.App); err != nil || len(p) != 0 {
		t.Errorf("pending %+v, %v once pushed", p, err)
	}
}

// saidBy collects the details r's done steps say.
func saidBy(r *release.Runner) *[]string {
	said := &[]string{}
	r.Step = func(s release.Step) {
		if s.State == "done" {
			*said = append(*said, s.Detail)
		}
	}
	return said
}

func TestOfflineCutTakenDropsIt(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	m := shipped(t, r, fx)
	tg := cutOffline(t, r, fx)
	other := cloneOf(t, fx.AppBare)
	releasetest.Git(t, other, "tag", "-a", tg.Name, "-m", "theirs", m)
	releasetest.Git(t, other, "push", "-q", "origin", "refs/tags/"+tg.Name)
	said := saidBy(r)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if want := "dropped v1.0.1-rc.1, cut offline: another clone took it; cut again"; !slices.Contains(*said, want) {
		t.Errorf("said %q, want %q", *said, want)
	}
	if p, err := release.Pending(ctx, fx.App); err != nil || len(p) != 0 {
		t.Errorf("pending %+v, %v once taken", p, err)
	}
	if got := releasetest.Git(t, fx.App, "tag", "-l", tg.Name, "--format=%(contents:subject)"); got != "theirs" {
		t.Errorf("local %s says %q, want theirs", tg.Name, got)
	}
	if len(tools.Built) != 0 || len(tools.Tagged) != 0 || len(tools.Charts) != 0 {
		t.Errorf("built %+v, tagged %q, charts %v after a taken name", tools.Built, tools.Tagged, tools.Charts)
	}
	if recs, err := r.B.Releases(); err != nil || len(recs) != 0 {
		t.Errorf("records %+v, %v after a taken name", recs, err)
	}
}

// Online, the cut made offline is checked again on what is fetched: one
// that no longer holds is dropped, and nothing is pushed.
func TestOfflineCutCheckedAgainOnline(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	shipped(t, r, fx)
	first := releasetest.Git(t, fx.App, "rev-list", "--max-parents=0", "HEAD")
	tg := cutOffline(t, r, fx)
	other := cloneOf(t, fx.AppBare) // meanwhile prod took a hotfix the cut lacks
	releasetest.Git(t, other, "checkout", "-q", "-b", "hotfix/1.0.1", first)
	h := releasetest.Commit(t, other, "hotfix")
	releasetest.Git(t, other, "push", "-q", "origin", "hotfix/1.0.1")
	both(t, fx, "prod", release.Deployed{Commit: h, Version: "v1.0.1"})
	said := saidBy(r)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !slices.ContainsFunc(*said, func(s string) bool {
		return strings.HasPrefix(s, "dropped "+tg.Name+", cut offline: it doesn't contain what runs in prod")
	}) {
		t.Errorf("said %q, want %s dropped for prod's hotfix", *said, tg.Name)
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != "v1.0.0" {
		t.Errorf("tags %q", got)
	}
	if p, err := release.Pending(ctx, fx.App); err != nil || len(p) != 0 {
		t.Errorf("pending %+v, %v once dropped", p, err)
	}
}

// singleEnv is a fonts-like app: prod alone, following the trunk, running
// a commit it returns.
func singleEnv(t *testing.T) (*release.Runner, *releasetest.Fixture, string) {
	t.Helper()
	r, _, fx := releasetest.NewRunner(t, "prod")
	prod := "prod"
	r.Cfg.Follow = &prod
	c := fx.Commit(t, "fix")
	both(t, fx, "prod", release.Deployed{Commit: c, Version: "0.0.1-dev.2.g" + c[:7]})
	return r, fx, c
}

// A single-environment app tags what it runs, a final, and deploys
// nowhere: prod already runs the commit.
func TestSingleEnvironmentCutsWhatRuns(t *testing.T) {
	r, fx, c := singleEnv(t)
	if d := draft(t, r, false); d.From != c || strings.Contains(d.Tag, "-rc.") {
		t.Errorf("draft from %.7s tag %s, want %.7s and a final", d.From, d.Tag, c)
	}
	before := heads(t, fx)
	tg := cut(t, r, release.Cut{Deploy: true})
	if tg.Commit != c || tg.Version.RC != 0 {
		t.Errorf("cut %s on %.7s, want a final on %.7s", tg.Name, tg.Commit, c)
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != tg.Name {
		t.Errorf("tags %q, want %s", got, tg.Name)
	}
	if _, recorded, err := r.B.ReleaseFor(c); err != nil || !recorded {
		t.Errorf("recorded %v, %v", recorded, err)
	}
	if !slices.Equal(heads(t, fx), before) {
		t.Error("the cut moved an overlay")
	}
}

// Offline, a single-environment app cuts its final as it would online,
// deploy asked or not: it deploys nowhere either way.
func TestSingleEnvironmentCutsOffline(t *testing.T) {
	r, fx, c := singleEnv(t)
	status(t, r)
	before := heads(t, fx)
	plug := unplug(t, fx)
	r.Offline = true
	pending := cut(t, r, release.Cut{Deploy: true})
	r.Offline = false
	plug()
	tg := cut(t, r, release.Cut{Deploy: true})
	if tg.Name != pending.Name || tg.Commit != c || tg.Version.RC != 0 {
		t.Errorf("cut %s on %.7s, want the pending final %s on %.7s", tg.Name, tg.Commit, pending.Name, c)
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != tg.Name {
		t.Errorf("tags %q, want %s", got, tg.Name)
	}
	if _, recorded, err := r.B.ReleaseFor(c); err != nil || !recorded {
		t.Errorf("recorded %v, %v", recorded, err)
	}
	if !slices.Equal(heads(t, fx), before) {
		t.Error("the cut moved an overlay")
	}
}

func TestSingleEnvironmentPromoteRefused(t *testing.T) {
	r, fx, _ := singleEnv(t)
	_, err := r.Promote(ctx, "v0.0.1-rc.1")
	want := "prod follows dev and is the last environment: nothing to promote; release cut tags what it runs"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l"); got != "" {
		t.Errorf("tags %q", got)
	}
}

// A rollback goes back to a single-environment app's tag.
func TestSingleEnvironmentRollback(t *testing.T) {
	r, fx, c := singleEnv(t)
	tg := cut(t, r, release.Cut{})
	n := fx.Commit(t, "newer")
	if err := r.Deploy(ctx, "prod", release.Target{Commit: n}); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := r.Rollback(ctx, "prod", tg.Name, "test"); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if e := envOf(t, status(t, r), "prod"); e.Commit != c || e.Version != tg.Name {
		t.Errorf("prod runs %.7s as %s, want %.7s as %s", e.Commit, e.Version, c, tg.Name)
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

// A tag a cut pushes says git-board made it; a tag made by hand doesn't.
func TestCutTagSaysGitBoard(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	shipped(t, r, fx)
	rc := cut(t, r, release.Cut{})
	for _, row := range status(t, r).Releases {
		switch row.Tag.Name {
		case rc.Name:
			if row.BeforeGitBoard || row.Tag.Message != rc.Name+" (git-board)" {
				t.Errorf("cut %s: BeforeGitBoard %v, message %q", rc.Name, row.BeforeGitBoard, row.Tag.Message)
			}
		case "v1.0.0":
			if !row.BeforeGitBoard {
				t.Errorf("hand-made v1.0.0 isn't BeforeGitBoard")
			}
		}
	}
}
