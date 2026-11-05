package ship_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/next/tools/ship"
	"github.com/guilbep/git-board/next/tools/ship/shiptest"
)

func TestNewRunner(t *testing.T) {
	src, _, fx := shiptest.NewRunner(t)
	r, err := ship.New(t.Context(), src.Cfg, src.Branches, fx.App, src.B, src.Tools, "")
	if err != nil {
		t.Fatal(err)
	}
	common, err := filepath.EvalSymlinks(filepath.Join(fx.App, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	cache, err := filepath.EvalSymlinks(filepath.Dir(filepath.Dir(r.CacheDir)))
	if err != nil {
		t.Fatal(err)
	}
	if cache != common || !strings.HasSuffix(r.CacheDir, filepath.Join("git-board", "release")) {
		t.Errorf("CacheDir = %s, want <%s>/git-board/release", r.CacheDir, common)
	}
	if r.Who != "dev" || r.Dir != fx.App || r.Tools == nil || r.B != src.B {
		t.Errorf("runner = %+v", r)
	}
}
