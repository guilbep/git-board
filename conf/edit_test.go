package conf

import (
	"errors"
	"testing"
)

func TestSetKeys(t *testing.T) {
	t.Parallel()
	const file = "# the board\n[views.x]  # x\nfilter = \"a\"   # kept\n\n# the next table's\n[sprints]\nlength_days = 14\n"
	for _, c := range []struct {
		name, table string
		vals        map[string]string
		want        string
	}{
		{"in place, its comment kept", "views.x", map[string]string{"filter": `"b"`},
			"# the board\n[views.x]  # x\nfilter = \"b\"   # kept\n\n# the next table's\n[sprints]\nlength_days = 14\n"},
		{"the same value, as written", "views.x", map[string]string{"filter": `'a'`}, file},
		{"after the last key", "views.x", map[string]string{"sort": `"prio asc"`},
			"# the board\n[views.x]  # x\nfilter = \"a\"   # kept\nsort = \"prio asc\"\n\n# the next table's\n[sprints]\nlength_days = 14\n"},
		{"removed", "sprints", map[string]string{"length_days": ""},
			"# the board\n[views.x]  # x\nfilter = \"a\"   # kept\n\n# the next table's\n[sprints]\n"},
		{"a new table at the end", "views.y", map[string]string{"layout": `"board"`}, file + "\n[views.y]\nlayout = \"board\"\n"},
	} {
		var keys []string
		for k := range c.vals {
			keys = append(keys, k)
		}
		if got := SetKeys(file, c.table, keys, c.vals); got != c.want {
			t.Errorf("%s:\n%q\nwant\n%q", c.name, got, c.want)
		}
	}
	if got := SetKeys("", "a", []string{"b"}, map[string]string{"b": "1"}); got != "[a]\nb = 1\n" {
		t.Errorf("no file: %q", got)
	}
}

func TestKeys(t *testing.T) {
	t.Parallel()
	const file = "[views.x]\nfilter = \"a\"\n[[sprints.list]]\nslug = \"s\"\n"
	if v, err := GetKey(file, "views.x.filter"); v != `"a"` || err != nil {
		t.Errorf("GetKey = %s, %v", v, err)
	}
	if _, err := GetKey(file, "views.y.filter"); !errors.Is(err, ErrNoKey) {
		t.Errorf("GetKey of no key: %v", err)
	}
	if kvs, err := ListKeys(file); err != nil || len(kvs) != 2 || kvs[0].Key != "sprints.list" || kvs[0].Value != `[{slug = "s"}]` {
		t.Errorf("ListKeys = %v, %v", kvs, err)
	}
	for _, key := range []string{"top", "a..b", "a b.c"} {
		if _, _, err := SplitKey(file, key); err == nil {
			t.Errorf("SplitKey(%q) takes it", key)
		}
	}
	if _, _, err := SplitKey(file, "sprints.list.slug"); err == nil {
		t.Error("a key of an array of tables is taken")
	}
	for in, want := range map[string]string{"14": "14", "true": "true", `["a"]`: `["a"]`, "is:open": `"is:open"`, "a\"b": `"a\"b"`} {
		if got := Value(in); got != want {
			t.Errorf("Value(%q) = %s, want %s", in, got, want)
		}
	}
}
