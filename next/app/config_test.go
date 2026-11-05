package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/next/conf"
)

// Each testdata/config/<case>.toml is read as v1 reads it: <case>.err is
// what v1 prints on stderr for it (its refusal, or the keys it doesn't
// know), recorded with v1's binary, the path made .git-board.toml.
func TestConfigAsV1(t *testing.T) {
	t.Parallel()
	cases, err := filepath.Glob("testdata/config/*.toml")
	if err != nil || len(cases) == 0 {
		t.Fatal("no cases", err)
	}
	for _, c := range cases {
		name := strings.TrimSuffix(filepath.Base(c), ".toml")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			want, err := os.ReadFile(strings.TrimSuffix(c, ".toml") + ".err")
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(c)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), conf.FileName)
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
			var got strings.Builder
			f, err := conf.Load(path)
			if err == nil {
				_, err = ReadConfig(f, nil)
			}
			if err != nil {
				got.WriteString("error: " + err.Error() + "\n")
			} else {
				for _, k := range f.Unknown() {
					got.WriteString("warning: " + path + ": " + k + ": git-board doesn't know this key, ignored (git board lint)\n")
				}
			}
			if g := strings.ReplaceAll(got.String(), path, conf.FileName); g != string(want) {
				t.Errorf("got\n%s\nwant (v1's)\n%s", g, want)
			}
		})
	}
}
