package release_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/release"
	"github.com/guilbep/git-board/internal/release/releasetest"
)

func TestNewRunner(t *testing.T) {
	src, _, fx := releasetest.NewRunner(t)
	r, err := release.New(&config.Config{Release: src.Cfg}, fx.App, src.B)
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
