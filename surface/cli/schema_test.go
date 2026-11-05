package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// config schema, upgrade and downgrade, end to end. They chdir (-C): serial.
func TestConfigSchemaCommands(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, ".git-board.toml")
	const v1 = "[sprints]\nlength_days = 14\nstart_weekday = \"monday\"\n"
	if err := os.WriteFile(path, []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}
	file := func() string { b, _ := os.ReadFile(path); return string(b) }
	for _, c := range []struct {
		args []string
		code int
		has  string
		file string // after it; "" the same as before
	}{
		{[]string{"config", "schema"}, 0, ".git-board.toml: schema 1\ngit-board: schema 2\npending:\n  1 to 2: sets aside", v1},
		{[]string{"config", "schema", "--json", "file,binary"}, 0, "\"binary\": 2", v1},
		{[]string{"config", "upgrade", "--dry-run"}, 0, "+schema = 2\n+\n [sprints]\n length_days = 14\n-start_weekday", v1},
		{[]string{"config", "downgrade"}, 2, "needs --to", v1},
		{[]string{"config", "upgrade", "--to", "3"}, 2, "this git-board knows schema 1 to 2", v1},
		{[]string{"config", "upgrade"}, 0, ".git-board.toml: schema 1 to 2\n", "schema = 2\n\n[sprints]\nlength_days = 14\n# start_weekday"},
		{[]string{"config", "upgrade"}, 0, "schema 2 already", ""},
		{[]string{"config", "downgrade", "--to", "1"}, 0, "2 to 1, undoing what 1 to 2 does", v1},
		{[]string{"config", "set", "sprints.length_days", "21"}, 0, ".git-board.toml upgraded from schema 1 to 2", "schema = 2\n\n[sprints]\nlength_days = 21\n"},
		{[]string{"config", "set", "sprints.length_days", "7"}, 0, "", "schema = 2\n\n[sprints]\nlength_days = 7\n"},
	} {
		before := file()
		out, errs, code := run(t, c.args...)
		if code != c.code || !strings.Contains(out+errs, c.has) {
			t.Errorf("%s: exit %d, want %d; want %q in\n%s%s", strings.Join(c.args, " "), code, c.code, c.has, out, errs)
		}
		if c.has == "" && errs != "" {
			t.Errorf("%s: said %q", strings.Join(c.args, " "), errs)
		}
		want := c.file
		if want == "" {
			want = before
		}
		if !strings.HasPrefix(file(), want) {
			t.Errorf("%s: the file is\n%s\nwant it to start\n%s", strings.Join(c.args, " "), file(), want)
		}
	}
}
