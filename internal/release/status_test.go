package release_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/release"
	"github.com/guilbep/git-board/internal/release/releasetest"
)

func status(t *testing.T, r *release.Runner) release.Status {
	t.Helper()
	st, err := r.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	return st
}

func envOf(t *testing.T, st release.Status, name string) release.EnvState {
	t.Helper()
	for _, e := range st.Envs {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("no environment %s in %+v", name, st.Envs)
	return release.EnvState{}
}

// both writes d into every target's overlay of env.
func both(t *testing.T, fx *releasetest.Fixture, env string, d release.Deployed) {
	t.Helper()
	for i := range fx.ConfigBare {
		fx.SetOverlay(t, i, env, d)
	}
}

func TestStatusAgreeingTargets(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t, "preview", "dev", "stage", "prod")
	c := fx.Commit(t, "feat")
	both(t, fx, "stage", release.Deployed{Commit: c, Version: "v1.0.0-rc.1"})
	st := status(t, r)
	e := envOf(t, st, "stage")
	if e.Commit != c || e.Version != "v1.0.0-rc.1" || e.Split || !e.Adopted {
		t.Errorf("stage = %+v", e)
	}
	if len(e.Targets) != 2 {
		t.Fatalf("targets = %+v", e.Targets)
	}
	tg := e.Targets[1]
	if tg.Repo != fx.ConfigBare[1] || tg.Path != "app/overlays/stage" || tg.Deployed.Chart != "v1.0.0-rc.1" ||
		tg.Since == nil || tg.Since.Version != "v1.0.0-rc.1" || tg.Deployed.Images["quay.test/app/backend"].Tag != "latest" {
		t.Errorf("target 1 = %+v, since %+v", tg, tg.Since)
	}
	var roles []string
	for _, e := range st.Envs {
		roles = append(roles, e.Name+":"+e.Role)
	}
	if got := strings.Join(roles, " "); got != "preview: dev:follow stage:candidate prod:last" {
		t.Errorf("roles = %s", got)
	}
	if st.Head != c {
		t.Errorf("head = %.7s, want %.7s", st.Head, c)
	}
}

func TestStatusSplit(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	c1 := fx.Commit(t, "one")
	c2 := fx.Commit(t, "two")
	fx.SetOverlay(t, 0, "prod", release.Deployed{Commit: c2, Version: "v1.0.1"})
	fx.SetOverlay(t, 1, "prod", release.Deployed{Commit: c1, Version: "v1.0.0"})
	e := envOf(t, status(t, r), "prod")
	if !e.Split || e.Commit != "" || e.Version != "" || !e.Adopted {
		t.Errorf("prod = %+v", e)
	}
	if e.Targets[0].Deployed.Commit != c2 || e.Targets[1].Deployed.Commit != c1 {
		t.Errorf("targets = %+v", e.Targets)
	}
}

func TestStatusBehind(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	c1 := fx.Commit(t, "deployed")
	both(t, fx, "dev", release.Deployed{Commit: c1, Version: "1.0.1-dev.1.g" + c1[:7]})
	both(t, fx, "stage", release.Deployed{Commit: c1, Version: "v1.0.1-rc.1"})
	fx.Merge(t, "fix/1-a", "")
	head := fx.Merge(t, "feat/2-b", "Merge pull request #9 from org/feat/2-b")
	st := status(t, r)
	want := []string{"Merge branch 'fix/1-a' into dev", "Merge pull request #9 from org/feat/2-b"}
	if got := envOf(t, st, "dev").Behind; !slices.Equal(got, want) {
		t.Errorf("dev behind = %q, want %q", got, want)
	}
	if got := envOf(t, st, "stage").Behind; got != nil {
		t.Errorf("stage behind = %q, want none: follow only", got)
	}
	if st.Head != head {
		t.Errorf("head = %.7s, want %.7s", st.Head, head)
	}
}

func TestStatusNotAdopted(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	fx.SetOverlay(t, 0, "prod", release.Deployed{Commit: fx.Commit(t, "x"), Version: "v1.0.0"})
	st := status(t, r)
	if e := envOf(t, st, "dev"); e.Adopted || e.Commit != "" || e.Split || e.Behind != nil {
		t.Errorf("dev = %+v", e)
	}
	// one target adopted, the other not: not adopted, and split
	if e := envOf(t, st, "prod"); e.Adopted || !e.Split || e.Commit != "" {
		t.Errorf("prod = %+v", e)
	}
}

// Status fetches the app remote and the config repos, unless Offline.
func TestStatusFetchesUnlessOffline(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	status(t, r) // clones the config repos
	other := cloneOf(t, fx.AppBare)
	theirs := releasetest.Commit(t, other, "theirs")
	releasetest.Git(t, other, "push", "-q", "origin", "dev")
	fx.SetOverlay(t, 0, "dev", release.Deployed{Commit: theirs, Version: "1.0.1-dev.1.g" + theirs[:7]})
	r.Offline = true
	st := status(t, r)
	if d := envOf(t, st, "dev").Targets[0].Deployed; st.Head == theirs || d.Commit != "" {
		t.Errorf("offline: head %.7s, dev %+v; want neither fetched", st.Head, d)
	}
	r.Offline = false
	st = status(t, r)
	if d := envOf(t, st, "dev").Targets[0].Deployed; st.Head != theirs || d.Commit != theirs {
		t.Errorf("online: head %.7s, dev %+v; want both fetched", st.Head, d)
	}
}

func TestReleaseRowsRunsIn(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	fx.Tag(t, releasetest.Git(t, fx.App, "rev-parse", "HEAD"), "v0.9.0")
	c := fx.Commit(t, "fix")
	fx.Tag(t, c, "v1.0.0-rc.2", "v1.0.0")
	both(t, fx, "prod", release.Deployed{Commit: c, Version: "v1.0.0"})
	st := status(t, r)
	var tags []string
	for _, row := range st.Releases {
		tags = append(tags, row.Tag.Name)
	}
	if got := strings.Join(tags, " "); got != "v1.0.0 v1.0.0-rc.2 v0.9.0" {
		t.Fatalf("rows = %s", got)
	}
	names := []string{"v1.0.0", "v1.0.0-rc.2"}
	for _, row := range st.Releases[:2] {
		if !slices.Equal(row.Names, names) || !slices.Equal(row.RunsIn, []string{"prod"}) || row.Record != nil {
			t.Errorf("%s = %+v", row.Tag.Name, row)
		}
	}
	if old := st.Releases[2]; !slices.Equal(old.Names, []string{"v0.9.0"}) || old.RunsIn != nil {
		t.Errorf("v0.9.0 = %+v", old)
	}
}

func TestReconcileCreatesMissing(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	first := releasetest.Git(t, fx.App, "rev-parse", "HEAD")
	fx.Tag(t, first, "v0.9.0")
	c := fx.Commit(t, "fix")
	fx.Tag(t, c, "v1.0.0-rc.1")
	tools.Images["ghcr.test/app/frontend:v1.0.0-rc.1"] = releasetest.FakeImage{Digest: "sha256:fe", Revision: c}
	// kept though built for another commit (legacy reuse_unchanged_images)
	tools.Images["quay.test/app/frontend:v1.0.0-rc.1"] = releasetest.FakeImage{Digest: "sha256:qfe", Revision: first}
	// not tagged yet: the image under the commit, when built for it
	tools.Images["ghcr.test/app/backend:"+c] = releasetest.FakeImage{Digest: "sha256:be", Revision: c}
	tools.Images["quay.test/app/backend:"+c] = releasetest.FakeImage{Digest: "sha256:qbe", Revision: first}
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if recs, err := r.B.Releases(); err != nil || len(recs) != 2 {
		t.Fatalf("Releases = %+v, %v; want v0.9.0's and v1.0.0-rc.1's", recs, err)
	}
	rec, ok, err := r.B.ReleaseFor(c)
	want := []gitbugio.Artifact{
		{Image: "frontend", Registry: "ghcr.test/app", Digest: "sha256:fe"},
		{Image: "frontend", Registry: "quay.test/app", Digest: "sha256:qfe"},
		{Image: "backend", Registry: "ghcr.test/app", Digest: "sha256:be"},
	}
	if err != nil || !ok || rec.Previous != "v0.9.0" || !slices.Equal(rec.Artifacts, want) {
		t.Errorf("record = %+v, %v, %v", rec, ok, err)
	}
	if old, _, _ := r.B.ReleaseFor(first); old.Previous != "" {
		t.Errorf("v0.9.0's previous = %q", old.Previous)
	}
	if got := releasetest.Git(t, fx.AppBare, "for-each-ref", "refs/releases"); got == "" {
		t.Error("the records weren't pushed")
	}
	fx.Tag(t, c, "v1.0.0") // the final on the rc's commit: same record
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	if recs, _ := r.B.Releases(); len(recs) != 2 {
		t.Errorf("second Reconcile made records: %+v", recs)
	}
	if row := status(t, r).Releases[0]; row.Tag.Name != "v1.0.0" || row.Record == nil || row.Record.ID != rec.ID {
		t.Errorf("row = %+v", row)
	}
}

// Reconcile pushes the records an earlier push missed; offline it does
// nothing.
func TestReconcilePushesOnlineOnly(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	c := fx.Commit(t, "fix")
	fx.Tag(t, c, "v1.0.0")
	r.Offline = true
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("offline Reconcile: %v", err)
	}
	if recs, _ := r.B.Releases(); len(recs) != 0 {
		t.Errorf("offline Reconcile recorded %+v", recs)
	}
	if _, err := r.B.CreateRelease(gitbugio.ReleaseRecord{Commit: c}); err != nil { // its push failed
		t.Fatal(err)
	}
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("offline Reconcile: %v", err)
	}
	if got := releasetest.Git(t, fx.AppBare, "for-each-ref", "refs/releases"); got != "" {
		t.Errorf("offline Reconcile pushed %s", got)
	}
	r.Offline = false
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := releasetest.Git(t, fx.AppBare, "for-each-ref", "refs/releases"); got == "" {
		t.Error("the record made before wasn't pushed")
	}
}
