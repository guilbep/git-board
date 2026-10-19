package gitbugio_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/git-bug/git-bug/entities/identity"
	"github.com/git-bug/git-bug/entity"
	"github.com/git-bug/git-bug/entity/dag"

	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

const releaseCommit = "5e09a71c3d2b8f1e4a6b9c0d7e2f3a4b5c6d7e8f"

func TestReleaseRoundTrip(t *testing.T) {
	t.Parallel()
	mine, theirs := gitbugiotest.NewPair(t)
	r, err := mine.CreateRelease(gitbugio.ReleaseRecord{Commit: releaseCommit, Previous: "v1.4.19",
		Artifacts: []gitbugio.Artifact{{Image: "frontend", Registry: "ghcr.io/x", Digest: "sha256:aa"}},
		Chart:     gitbugio.ChartArtifact{Version: "v1.4.20", Digest: "sha256:cc"}})
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if err := mine.SetReleaseNotes(r.ID, "A fix release."); err != nil {
		t.Fatalf("SetReleaseNotes: %v", err)
	}
	if err := mine.CommentRelease(r.ID, "Looks good on stage."); err != nil {
		t.Fatalf("CommentRelease: %v", err)
	}
	if err := theirs.PullReleases("origin"); err != nil {
		t.Fatalf("PullReleases before any push: %v", err)
	}
	if err := mine.PushReleases("origin"); err != nil {
		t.Fatalf("PushReleases: %v", err)
	}
	if err := theirs.PullReleases("origin"); err != nil {
		t.Fatalf("PullReleases: %v", err)
	}
	got, ok, err := theirs.ReleaseFor(releaseCommit)
	if err != nil || !ok {
		t.Fatalf("ReleaseFor = %v, %v", ok, err)
	}
	if got.ID != r.ID || got.Commit != releaseCommit || got.Previous != "v1.4.19" || got.Notes != "A fix release." ||
		len(got.Artifacts) != 1 || got.Artifacts[0] != (gitbugio.Artifact{Image: "frontend", Registry: "ghcr.io/x", Digest: "sha256:aa"}) ||
		got.Chart.Digest != "sha256:cc" || got.Author == "" || got.Created.IsZero() || got.Yanked != "" {
		t.Errorf("their record = %+v, made %+v", got, r)
	}
	if len(got.Comments) != 1 || got.Comments[0].Message != "Looks good on stage." || got.Comments[0].Author != got.Author {
		t.Errorf("comments = %+v", got.Comments)
	}
	if _, ok, _ := theirs.ReleaseFor("another"); ok {
		t.Error("ReleaseFor found a record for a commit without one")
	}
}

// Two clones create a record for one commit: the earlier create Lamport
// time counts, then the smaller id; both sides show the same one.
func TestOneRecordPerCommit(t *testing.T) {
	t.Parallel()
	mine, theirs := gitbugiotest.NewPair(t)
	create := func(b *gitbugio.Backend, commit string) string {
		t.Helper()
		r, err := b.CreateRelease(gitbugio.ReleaseRecord{Commit: commit})
		if err != nil {
			t.Fatalf("CreateRelease(%s): %v", commit, err)
		}
		return r.ID
	}
	sameA, sameB := create(mine, "same"), create(theirs, "same") // both at time 1
	create(theirs, "unrelated")                                  // theirs' clock moves on
	// mine's is one tick earlier each time, whichever id is smaller: eight
	// tries, so a tie-break by id alone shows but once in 256
	want := map[string]string{"same": min(sameA, sameB), "unrelated": ""}
	for i := range 8 {
		c := fmt.Sprint("later-", i)
		want[c] = create(mine, c)
		create(theirs, c)
	}
	exchange := func(from, to *gitbugio.Backend) {
		t.Helper()
		if err := from.PushReleases("origin"); err != nil {
			t.Fatalf("PushReleases: %v", err)
		}
		if err := to.PullReleases("origin"); err != nil {
			t.Fatalf("PullReleases: %v", err)
		}
	}
	exchange(mine, theirs)
	exchange(theirs, mine)
	for name, b := range map[string]*gitbugio.Backend{"mine": mine, "theirs": theirs} {
		rs, err := b.Releases()
		if err != nil || len(rs) != len(want) {
			t.Fatalf("%s: Releases = %d, %v; want %d", name, len(rs), err, len(want))
		}
		for _, r := range rs {
			if w := want[r.Commit]; w != "" && r.ID != w {
				t.Errorf("%s: %s counts %s, want %s", name, r.Commit, r.ID[:7], w[:7])
			}
		}
	}
}

func TestCreateReturnsExisting(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	first, err := b.CreateRelease(gitbugio.ReleaseRecord{Commit: releaseCommit, Previous: "v1.4.19"})
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	second, err := b.CreateRelease(gitbugio.ReleaseRecord{Commit: releaseCommit, Previous: "v1.4.18"})
	if err != nil || second.ID != first.ID || second.Previous != "v1.4.19" {
		t.Fatalf("second CreateRelease = %+v, %v; want the first, %s", second, err, first.ID)
	}
	if rs, err := b.Releases(); err != nil || len(rs) != 1 {
		t.Errorf("Releases = %+v, %v; want one", rs, err)
	}
	if _, err := b.CreateRelease(gitbugio.ReleaseRecord{}); err == nil {
		t.Error("a record without a commit was created")
	}
}

func TestYankedRecord(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	r, err := b.CreateRelease(gitbugio.ReleaseRecord{Commit: releaseCommit})
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if err := b.YankRelease(r.ID, ""); err == nil {
		t.Error("a yank without a reason was taken")
	}
	if err := b.YankRelease(r.ID, "breaks the login"); err != nil {
		t.Fatalf("YankRelease: %v", err)
	}
	if got, _, err := b.ReleaseFor(releaseCommit); err != nil || got.Yanked != "breaks the login" {
		t.Errorf("ReleaseFor = %+v, %v; want yanked for its reason", got, err)
	}
}

// futureOp is an operation a later git-board adds: unknown here.
type futureOp struct {
	dag.OpBase
	Shiny string `json:"shiny"`
}

func (op *futureOp) Id() entity.Id   { return dag.IdOperation(op, &op.OpBase) }
func (op *futureOp) Validate() error { return op.OpBase.Validate(op, 99) }

// A record holding an operation this git-board doesn't know still lists,
// edits and travels: the operation is kept and skipped.
func TestUnknownReleaseOp(t *testing.T) {
	t.Parallel()
	mine, theirs := gitbugiotest.NewPair(t)
	r, err := mine.CreateRelease(gitbugio.ReleaseRecord{Commit: releaseCommit, Previous: "v1.4.19"})
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if err := gitbugio.EditRelease(mine, r.ID, func(me identity.Interface) dag.Operation {
		return &futureOp{OpBase: dag.NewOpBase(99, me, time.Now().Unix()), Shiny: "yes"}
	}); err != nil {
		t.Fatalf("adding the unknown operation: %v", err)
	}
	if rs, err := mine.Releases(); err != nil || len(rs) != 1 || rs[0].ID != r.ID || rs[0].Previous != "v1.4.19" {
		t.Fatalf("Releases = %+v, %v; want the record", rs, err)
	}
	if err := mine.SetReleaseNotes(r.ID, "Still editable."); err != nil {
		t.Fatalf("SetReleaseNotes: %v", err)
	}
	if err := mine.PushReleases("origin"); err != nil {
		t.Fatalf("PushReleases: %v", err)
	}
	if err := theirs.PullReleases("origin"); err != nil {
		t.Fatalf("PullReleases: %v", err)
	}
	if got, ok, err := theirs.ReleaseFor(releaseCommit); err != nil || !ok || got.Notes != "Still editable." {
		t.Errorf("their ReleaseFor = %+v, %v, %v", got, ok, err)
	}
}

// git-board's own push and pull carry the records, and a remote without
// any is no error.
func TestPushPullCarryReleases(t *testing.T) {
	t.Parallel()
	mine, theirs := gitbugiotest.NewPair(t)
	gitbugiotest.NewIssue(t, mine, "Crash")
	if _, err := mine.Push("origin"); err != nil {
		t.Fatalf("Push without releases: %v", err)
	}
	if err := theirs.Pull("origin"); err != nil {
		t.Fatalf("Pull from a remote without releases: %v", err)
	}
	r, err := mine.CreateRelease(gitbugio.ReleaseRecord{Commit: releaseCommit})
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if _, err := mine.Push("origin"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := theirs.Pull("origin"); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if got, ok, err := theirs.ReleaseFor(releaseCommit); err != nil || !ok || got.ID != r.ID {
		t.Errorf("their ReleaseFor = %+v, %v, %v; want %s", got, ok, err, r.ID)
	}
}
