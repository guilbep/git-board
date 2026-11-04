package release

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/datastore/memstore"
	"github.com/guilbep/git-board/core/oplog"
)

const commit = "5e09a71c3d2b8f1e4a6b9c0d7e2f3a4b5c6d7e8f"

var ctx = context.Background()

// name is a person's name as the tests know it: their id, upper-cased.
func name(id string) string { return strings.ToUpper(id) }

func TestRecordRoundTrip(t *testing.T) {
	t.Parallel()
	s := memstore.New("")
	r, err := Create(ctx, s, "alice", Record{Commit: commit, Previous: "v1.4.19",
		Artifacts: []Artifact{{Image: "frontend", Registry: "ghcr.io/x", Digest: "sha256:aa"}},
		Chart:     ChartArtifact{Version: "v1.4.20", Digest: "sha256:cc"},
		Assets:    []Asset{{Name: "git-board_v1.4.20_linux_amd64.tar.gz", SHA256: "ab"}}}, name)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := SetNotes(ctx, s, r.ID, "alice", "A fix release."); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}
	if err := AddComment(ctx, s, r.ID, "bob", "Looks good on stage."); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	got, ok, err := For(s, commit, name)
	if err != nil || !ok {
		t.Fatalf("For = %v, %v", ok, err)
	}
	if got.ID != r.ID || got.Previous != "v1.4.19" || got.Notes != "A fix release." || got.Author != "ALICE" ||
		len(got.Artifacts) != 1 || got.Chart.Digest != "sha256:cc" || len(got.Assets) != 1 || got.Created.IsZero() || got.Yanked != "" {
		t.Errorf("record = %+v", got)
	}
	if len(got.Comments) != 1 || got.Comments[0].Message != "Looks good on stage." || got.Comments[0].Author != "BOB" {
		t.Errorf("comments = %+v", got.Comments)
	}
	if _, ok, _ := For(s, "another", name); ok {
		t.Error("For found a record for a commit without one")
	}
}

// Two records for one commit, as two clones make them before they meet:
// the one whose id sorts first counts.
func TestOneRecordPerCommit(t *testing.T) {
	t.Parallel()
	s := memstore.New("")
	var ids []string
	for range 4 {
		l, err := oplog.Create(ctx, s, Kind, "", []oplog.Op{newOp("create", "alice", Data{Commit: commit})})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, l.ID)
	}
	if _, err := Create(ctx, s, "alice", Record{Commit: "other"}, name); err != nil {
		t.Fatal(err)
	}
	rs, err := Records(s, name)
	if err != nil || len(rs) != 2 {
		t.Fatalf("Records = %+v, %v; want one per commit", rs, err)
	}
	got, _, _ := For(s, commit, name)
	if want := min(ids[0], ids[1], ids[2], ids[3]); got.ID != want {
		t.Errorf("%s counts %s, want %s", commit, got.ID, want)
	}
}

func TestCreateReturnsExisting(t *testing.T) {
	t.Parallel()
	s := memstore.New("")
	first, err := Create(ctx, s, "alice", Record{Commit: commit, Previous: "v1.4.19"}, name)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	second, err := Create(ctx, s, "bob", Record{Commit: commit, Previous: "v1.4.18"}, name)
	if err != nil || second.ID != first.ID || second.Previous != "v1.4.19" {
		t.Fatalf("second Create = %+v, %v; want the first, %s", second, err, first.ID)
	}
	if rs, err := Records(s, name); err != nil || len(rs) != 1 {
		t.Errorf("Records = %+v, %v; want one", rs, err)
	}
	if _, err := Create(ctx, s, "alice", Record{}, name); err == nil {
		t.Error("a record without a commit was created")
	}
}

func TestYank(t *testing.T) {
	t.Parallel()
	s := memstore.New("")
	r, err := Create(ctx, s, "alice", Record{Commit: commit}, name)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := Yank(ctx, s, r.ID, "alice", ""); err == nil {
		t.Error("a yank without a reason was taken")
	}
	if err := AddComment(ctx, s, r.ID, "alice", ""); err == nil {
		t.Error("an empty comment was taken")
	}
	if err := Yank(ctx, s, r.ID, "alice", "breaks the login"); err != nil {
		t.Fatalf("Yank: %v", err)
	}
	if got, _, err := For(s, commit, name); err != nil || got.Yanked != "breaks the login" {
		t.Errorf("For = %+v, %v; want yanked for its reason", got, err)
	}
	if err := Yank(ctx, s, "nope", "alice", "why"); err == nil {
		t.Error("a yank of no record was taken")
	}
}

// A record holding an operation this git-board doesn't know is an error,
// as the core's rule for every kind is.
func TestUnknownOp(t *testing.T) {
	t.Parallel()
	s := memstore.New("")
	r, err := Create(ctx, s, "alice", Record{Commit: commit, Previous: "v1.4.19"}, name)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := edit(ctx, s, r.ID, newOp("shiny", "alice", Data{})); err != nil {
		t.Fatalf("adding the unknown operation: %v", err)
	}
	if rs, err := Records(s, name); err == nil {
		t.Fatalf("Records = %+v; want an error naming the operation", rs)
	}
}

// A record without assets serializes as before this field: the records
// made before keep their IDs.
func TestCreateWithoutAssetsIsUnchanged(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(Data{Commit: "c", Previous: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "assets") {
		t.Errorf("a record without assets serializes them: %s", raw)
	}
}
