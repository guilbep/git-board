package conf

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestTables(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte("[fields.b]\nsingle = true\n[fields.a]\n[sprints]\nlength_days = \"two\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	type field struct {
		Single bool `toml:"single"`
	}
	fields := Declare(f, Table[map[string]field]{Name: "fields"})
	sprints := Declare(f, Table[struct {
		LengthDays int `toml:"length_days"`
	}]{Name: "sprints"})
	roles := Declare(f, Table[map[string]string]{Name: "roles", Default: func() map[string]string { return map[string]string{"pm": "x"} }})
	if v, probs := fields.Get(); len(probs) != 0 || !v["b"].Single || len(v) != 2 {
		t.Errorf("[fields] = %v, %v", v, probs)
	}
	if got := fields.Keys(); !slices.Equal(got, []string{"b", "a"}) {
		t.Errorf("Keys = %v, want the file's order", got)
	}
	// a broken table is its own problem: the others answer
	if _, probs := sprints.Get(); len(probs) != 1 || !strings.Contains(probs[0].Error(), "sprints.length_days") {
		t.Errorf("[sprints] problems = %v", probs)
	}
	if v, probs := roles.Get(); len(probs) != 0 || v["pm"] != "x" {
		t.Errorf("[roles] = %v, %v; want its default", v, probs)
	}
	defer func() {
		if recover() == nil {
			t.Error("a table declared twice doesn't panic")
		}
	}()
	Declare(f, Table[int]{Name: "roles"})
}

func TestLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if f, err := Load(filepath.Join(dir, "none.toml")); err != nil || len(f.tables) != 0 {
		t.Errorf("a missing file = %v, %v; want an empty one", f, err)
	}
	bad := filepath.Join(dir, "bad.toml")
	os.WriteFile(bad, []byte("[fields\n"), 0o644)
	if _, err := Load(bad); err == nil {
		t.Error("a file that isn't TOML loads")
	}
}

// The server reads tables from every request at once: a decode shares the
// file's MetaData, which toml mutates (a fatal "concurrent map writes").
func TestGetConcurrently(t *testing.T) {
	t.Parallel()
	f, err := Parse(FileName, []byte("[fields.a]\nsingle = true\n[fields.b]\n"))
	if err != nil {
		t.Fatal(err)
	}
	fields := Declare(f, Table[map[string]struct {
		Single bool `toml:"single"`
	}]{Name: "fields"})
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if v, probs := fields.Get(); len(probs) != 0 || !v["a"].Single {
				t.Errorf("[fields] = %v, %v", v, probs)
			}
			f.Unknown()
		})
	}
	wg.Wait()
}
