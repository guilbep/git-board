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

// A file is read at this git-board's schema: an older one migrated in
// memory, silently, and written upgraded by a set alone; a newer one
// read, never written.
func TestConfigSchema(t *testing.T) {
	t.Parallel()
	const v1 = "# the board\n\n[sprints]\nlength_days = 14\nstart_weekday = \"monday\"\n\n[repos]\nassignees = \"x\"\n"
	read := func(text string) (*Config, []string, error) {
		f, err := conf.Parse(conf.FileName, []byte(text))
		if err != nil {
			return nil, nil, err
		}
		c, err := ReadConfig(f, nil)
		return c, f.Unknown(), err
	}
	// v1 refused assignees = "x"; schema 2 sets the key aside, unread
	if c, unknown, err := read(v1); err != nil || len(unknown) != 0 || c.SchemaVersion != 1 {
		t.Errorf("v1: %v, unknown %v, schema %d", err, unknown, c.SchemaVersion)
	}
	// at schema 2 they are keys git-board doesn't know
	if _, unknown, err := read("schema = 2\n[sprints]\nstart_weekday = \"monday\"\n[repos]\nassignees = \"labels\"\n"); err != nil ||
		strings.Join(unknown, " ") != "sprints.start_weekday repos.assignees" {
		t.Errorf("v2: %v, unknown %v", err, unknown)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, conf.FileName)
	write := func(text string) {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	file := func() string {
		data, _ := os.ReadFile(path)
		return string(data)
	}
	write(v1)
	if s, err := ConfigSchema(dir); err != nil || s.File != 1 || s.Binary != CurrentSchema || len(s.Pending) != CurrentSchema-1 {
		t.Errorf("ConfigSchema: %+v, %v", s, err)
	}
	// a set that changes nothing writes nothing, upgrade aside
	if w, err := SetConfig(dir, "sprints.length_days", "14"); err != nil || w.Upgraded() || file() != v1 {
		t.Errorf("a set of the same value: %+v, %v\n%s", w, err, file())
	}
	w, err := SetConfig(dir, "sprints.length_days", "21")
	if err != nil || !w.Upgraded() || w.Notice() != ".git-board.toml upgraded from schema 1 to 2 (git board config schema says what changed)" {
		t.Fatalf("set: %+v, %v", w, err)
	}
	const v2 = "# the board\n\nschema = 2\n\n[sprints]\nlength_days = 21\n# start_weekday = \"monday\"  # schema 2: git-board doesn't read this key\n\n[repos]\n# assignees = \"x\"  # schema 2: git-board doesn't read this key\n"
	if file() != v2 {
		t.Errorf("written:\n%s", conf.Diff("want", "got", v2, file()))
	}
	if w, err := SetConfig(dir, "sprints.length_days", "7"); err != nil || w.Upgraded() || w.Notice() != "" {
		t.Errorf("the next set: %+v, %v", w, err) // once
	}
	m, err := MigrateConfig(dir, 1, false, true)
	if err != nil || m.From != 2 || m.To != 1 || len(m.Steps) != 1 || !strings.Contains(m.Diff, "+start_weekday = \"monday\"\n") || !strings.Contains(file(), "length_days = 7") || !strings.HasPrefix(file(), "# the board\n\nschema = 2\n") {
		t.Errorf("dry downgrade: %+v, %v", m, err)
	}
	if _, err := MigrateConfig(dir, 1, false, false); err != nil || file() != strings.Replace(v1, "14", "7", 1) {
		t.Errorf("downgrade: %v\n%s", err, file())
	}
	if _, err := MigrateConfig(dir, 2, false, false); Classify(err) != Usage {
		t.Errorf("a downgrade up: %v", err)
	}
	// newer: read, with what it knows; never written
	newer := "schema = 9\n[sprints]\nlength_days = 3\n"
	write(newer)
	if c, _, err := read(newer); err != nil || c.SchemaVersion != 9 || c.Schema.Sprints.LengthDays != 3 {
		t.Errorf("a newer file read: %v", err)
	}
	if _, err := SetConfig(dir, "sprints.length_days", "4"); Classify(err) != Usage || !strings.Contains(err.Error(), "is schema 9, and this git-board knows schema 2 at most") || file() != newer {
		t.Errorf("a newer file written: %v", err)
	}
	if _, err := MigrateConfig(dir, 2, true, false); err == nil || file() != newer {
		t.Errorf("a newer file upgraded: %v", err)
	}
}
