package releases

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/domain/people"
	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/store/mem"
	"github.com/guilbep/git-board/view"
)

const commit = "5e09a71c3d2b8f1e4a6b9c0d7e2f3a4b5c6d7e8f"

// board is a board in memory with two persons, alice and bob: a service
// writing as each.
func board(t *testing.T) (alice, bob *Service) {
	t.Helper()
	dir := t.TempDir()
	v, err := view.Open(t.Context(), filepath.Join(dir, "view.db"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := kernel.Open(t.Context(), mem.New(dir), v, people.Kind)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	person := func(name string) kernel.ID {
		raw, _ := json.Marshal(map[string]string{"name": name})
		e, err := people.Kind.Create(t.Context(), b, "", kernel.Op{Kind: "create", Time: 1, Data: raw})
		if err != nil {
			t.Fatal(err)
		}
		return e.ID
	}
	return &Service{Board: b, Me: person("Alice")}, &Service{Board: b, Me: person("Bob")}
}

func TestRecordRoundTrip(t *testing.T) {
	t.Parallel()
	s, bob := board(t)
	r, err := s.Create(t.Context(), Record{Commit: commit, Previous: "v1.4.19",
		Artifacts: []Artifact{{Image: "frontend", Registry: "ghcr.io/x", Digest: "sha256:aa"}},
		Chart:     ChartArtifact{Version: "v1.4.20", Digest: "sha256:cc"},
		Assets:    []Asset{{Name: "git-board_v1.4.20_linux_amd64.tar.gz", SHA256: "ab"}}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.SetNotes(t.Context(), r.ID, "A fix release."); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}
	if err := bob.Comment(t.Context(), r.ID, "Looks good on stage."); err != nil {
		t.Fatalf("Comment: %v", err)
	}
	got, ok, err := s.For(t.Context(), commit)
	if err != nil || !ok {
		t.Fatalf("For = %v, %v", ok, err)
	}
	if got.ID != r.ID || got.Previous != "v1.4.19" || got.Notes != "A fix release." || got.Author != "Alice" ||
		len(got.Artifacts) != 1 || got.Chart.Digest != "sha256:cc" || len(got.Assets) != 1 || got.Created.IsZero() || got.Yanked != "" {
		t.Errorf("record = %+v", got)
	}
	if len(got.Comments) != 1 || got.Comments[0].Message != "Looks good on stage." || got.Comments[0].Author != "Bob" {
		t.Errorf("comments = %+v", got.Comments)
	}
	if _, ok, _ := s.For(t.Context(), "another"); ok {
		t.Error("For found a record for a commit without one")
	}
}

// Two records for one commit, as two clones make them before they meet:
// the one whose id sorts first counts.
func TestOneRecordPerCommit(t *testing.T) {
	t.Parallel()
	s, _ := board(t)
	var ids []string
	for i := range 4 {
		op := s.op("create", Data{Commit: commit})
		op.Time = int64(i + 1) // packs of their own
		e, err := Kind.Create(t.Context(), s.Board, "", op)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, string(e.ID))
	}
	if _, err := s.Create(t.Context(), Record{Commit: "other"}); err != nil {
		t.Fatal(err)
	}
	rs, err := s.Records(t.Context())
	if err != nil || len(rs) != 2 {
		t.Fatalf("Records = %+v, %v; want one per commit", rs, err)
	}
	got, _, _ := s.For(t.Context(), commit)
	if want := slices.Min(ids); got.ID != want {
		t.Errorf("%s counts %s, want %s", commit, got.ID, want)
	}
}

func TestCreateReturnsExisting(t *testing.T) {
	t.Parallel()
	s, bob := board(t)
	first, err := s.Create(t.Context(), Record{Commit: commit, Previous: "v1.4.19"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	second, err := bob.Create(t.Context(), Record{Commit: commit, Previous: "v1.4.18"})
	if err != nil || second.ID != first.ID || second.Previous != "v1.4.19" {
		t.Fatalf("second Create = %+v, %v; want the first, %s", second, err, first.ID)
	}
	if rs, err := s.Records(t.Context()); err != nil || len(rs) != 1 {
		t.Errorf("Records = %+v, %v; want one", rs, err)
	}
	if _, err := s.Create(t.Context(), Record{}); err == nil {
		t.Error("a record without a commit was created")
	}
}

func TestYank(t *testing.T) {
	t.Parallel()
	s, _ := board(t)
	r, err := s.Create(t.Context(), Record{Commit: commit})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Yank(t.Context(), r.ID, ""); err == nil {
		t.Error("a yank without a reason was taken")
	}
	if err := s.Comment(t.Context(), r.ID, ""); err == nil {
		t.Error("an empty comment was taken")
	}
	if err := s.Yank(t.Context(), r.ID, "breaks the login"); err != nil {
		t.Fatalf("Yank: %v", err)
	}
	if got, _, err := s.For(t.Context(), commit); err != nil || got.Yanked != "breaks the login" {
		t.Errorf("For = %+v, %v; want yanked for its reason", got, err)
	}
	if err := s.Yank(t.Context(), strings.Repeat("0", 40), "why"); err == nil {
		t.Error("a yank of no record was taken")
	}
}

// No one to write as: no write.
func TestNoMe(t *testing.T) {
	t.Parallel()
	s, _ := board(t)
	s.Me = ""
	if _, err := s.Create(t.Context(), Record{Commit: commit}); !errors.Is(err, people.ErrNoMe) {
		t.Errorf("Create = %v, want ErrNoMe", err)
	}
}

// An operation this git-board doesn't know is an error, as the kernel's
// rule for every kind is: never written, never folded.
func TestUnknownOp(t *testing.T) {
	t.Parallel()
	s, _ := board(t)
	r, err := s.Create(t.Context(), Record{Commit: commit, Previous: "v1.4.19"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.edit(t.Context(), r.ID, s.op("shiny", Data{})); !errors.Is(err, ErrBad) {
		t.Fatalf("adding the unknown operation = %v, want ErrBad", err)
	}
	log := []kernel.Entry{{ID: "a:0", Op: s.op("create", Data{Commit: commit})}, {ID: "a:1", Op: s.op("shiny", Data{})}}
	if _, err := Kind.Fold("a", log); !errors.Is(err, ErrBad) {
		t.Errorf("Fold = %v, want ErrBad", err)
	}
	if _, err := Kind.Fold("a", log[1:]); !errors.Is(err, ErrBad) {
		t.Errorf("Fold without a create = %v, want ErrBad", err)
	}
}

// A record without assets serializes as before this field: the records
// made before keep their ids.
func TestCreateWithoutAssetsIsUnchanged(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(Data{Commit: "c", Previous: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "assets") || strings.Contains(string(raw), "chart") {
		t.Errorf("a record without assets serializes them: %s", raw)
	}
}
