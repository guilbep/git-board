package store_test

import (
	"fmt"
	"testing"

	"github.com/guilbep/git-board/core/release"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

const releaseCommit = "5e09a71c3d2b8f1e4a6b9c0d7e2f3a4b5c6d7e8f"

func TestReleaseRoundTrip(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	r, err := mine.CreateRelease(release.Record{Commit: releaseCommit, Previous: "v1.4.19",
		Artifacts: []release.Artifact{{Image: "frontend", Registry: "ghcr.io/x", Digest: "sha256:aa"}},
		Chart:     release.ChartArtifact{Version: "v1.4.20", Digest: "sha256:cc"},
		Assets:    []release.Asset{{Name: "git-board_v1.4.20_linux_amd64.tar.gz", SHA256: "ab"}}})
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
		len(got.Artifacts) != 1 || got.Artifacts[0] != (release.Artifact{Image: "frontend", Registry: "ghcr.io/x", Digest: "sha256:aa"}) ||
		got.Chart.Digest != "sha256:cc" || len(got.Assets) != 1 || got.Assets[0].Name != "git-board_v1.4.20_linux_amd64.tar.gz" || got.Author == "" || got.Created.IsZero() || got.Yanked != "" {
		t.Errorf("their record = %+v, made %+v", got, r)
	}
	if len(got.Comments) != 1 || got.Comments[0].Message != "Looks good on stage." || got.Comments[0].Author != got.Author {
		t.Errorf("comments = %+v", got.Comments)
	}
	if _, ok, _ := theirs.ReleaseFor("another"); ok {
		t.Error("ReleaseFor found a record for a commit without one")
	}
}

// Two clones create a record for one commit: the smaller id counts, and
// both sides show the same one.
func TestOneRecordPerCommit(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	create := func(b *store.Backend, commit string) string {
		t.Helper()
		r, err := b.CreateRelease(release.Record{Commit: commit})
		if err != nil {
			t.Fatalf("CreateRelease(%s): %v", commit, err)
		}
		return r.ID
	}
	want := map[string]string{"unrelated": ""}
	create(theirs, "unrelated")
	for i := range 8 {
		c := fmt.Sprint("same-", i)
		want[c] = min(create(mine, c), create(theirs, c))
	}
	exchange := func(from, to *store.Backend) {
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
	for name, b := range map[string]*store.Backend{"mine": mine, "theirs": theirs} {
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

// git-board's own push and pull carry the records, and a remote without
// any is no error.
func TestPushPullCarryReleases(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	storetest.NewIssue(t, mine, "Crash")
	if _, err := mine.Push("origin"); err != nil {
		t.Fatalf("Push without releases: %v", err)
	}
	if err := theirs.Pull("origin"); err != nil {
		t.Fatalf("Pull from a remote without releases: %v", err)
	}
	r, err := mine.CreateRelease(release.Record{Commit: releaseCommit})
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
