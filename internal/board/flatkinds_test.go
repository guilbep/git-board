package board_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// A view saved as label:bug still finds the issues labelled bug once a
// flat kind reads as kind:bug: the view as the file has it, run as the
// board runs it.
func TestLabelViewFindsAFlatKind(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), config.FileName)
	if err := os.WriteFile(path, []byte("[branches]\nkinds = { bug = \"fix\", feat = \"feat\" }\n\n[views.bugs]\nfilter = \"label:bug\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b := gitbugiotest.New(t)
	b.SetFlatKinds(cfg)
	for title, label := range map[string]string{"crash": "bug", "export": "feat"} {
		bug, _, err := b.TestCache().Bugs().New(t.Context(), title, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := bug.ChangeLabels(t.Context(), []string{label}, nil); err != nil { // as GitHub has it
			t.Fatal(err)
		}
		if err := bug.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	for filter, want := range map[string][]string{
		cfg.Views["bugs"].Filter: {"crash"},
		"kind:bug":               {"crash"},
		"label:BUG":              {"crash"},
		"-label:bug":             {"export"},
		"label:kind:bug":         {"crash"},
	} {
		got, err := board.LoadMatching(b, cfg, filter, "guilbep")
		if err != nil {
			t.Fatal(err)
		}
		var titles []string
		for _, is := range got {
			titles = append(titles, is.Title)
		}
		if !slices.Equal(titles, want) {
			t.Errorf("%s: %q, want %q", filter, titles, want)
		}
	}
}
