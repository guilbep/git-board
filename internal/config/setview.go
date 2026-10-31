package config

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"
)

var viewName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// keyLine is `key = "value"  # comment`: the key, the value as written,
// and what follows it.
var keyLine = regexp.MustCompile(`^(\s*([A-Za-z0-9_-]+)\s*=\s*)(\[[^\]]*\]|\{[^}]*\}|"(?:[^"\\]|\\.)*"|'[^']*'|[^#\s]+)(.*)$`)

// SetViewText is text with the [views.<name>] table holding v, the way the
// board's "Save view" writes it: its layout, filter, group and sort lines
// rewritten in place (trailing comments kept, an empty value removed, a
// missing one added after the table's last key), every other line of the
// file untouched; a new table appended when there is none.
func SetViewText(text, name string, v View) (string, error) {
	if !viewName.MatchString(name) {
		return "", fmt.Errorf("a view's name is letters, digits, - and _, not %q", name)
	}
	keys := []string{"layout", "filter", "group", "sort", "columns", "lanes", "limits"}
	// written as TOML: a string quoted, the columns and lanes an array,
	// the limits an inline table
	vals := map[string]string{"layout": tomlString(v.Layout), "filter": tomlString(v.Filter), "group": tomlString(v.Group), "sort": tomlString(v.Sort),
		"columns": tomlArray(v.Columns), "lanes": tomlArray(v.Lanes), "limits": tomlTable(v.Limits)}
	for k, val := range vals {
		if val == `""` {
			vals[k] = "" // empty: no line
		}
	}
	return setTableKeys(text, "[views."+name+"]", keys, vals, false), nil
}

// tomlArray writes strings as a TOML array, "" for none.
func tomlArray(list []string) string {
	if len(list) == 0 {
		return ""
	}
	quoted := make([]string, len(list))
	for i, s := range list {
		quoted[i] = tomlString(s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// tomlTable writes counts as a TOML inline table, keys sorted, "" for none.
func tomlTable(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		key := k
		if !viewName.MatchString(k) { // a bare key is letters, digits, - and _
			key = tomlString(k)
		}
		parts[i] = fmt.Sprintf("%s = %d", key, m[k])
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

// tomlString quotes s as a TOML basic string.
func tomlString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`).Replace(s) + `"`
}

// unquote reads back a value tomlString wrote, or a literal 'string'.
func unquote(v string) string {
	switch {
	case len(v) >= 2 && v[0] == '\'':
		return v[1 : len(v)-1]
	case len(v) >= 2 && v[0] == '"':
		return strings.NewReplacer(`\\`, `\`, `\"`, `"`, `\n`, "\n", `\t`, "\t").Replace(v[1 : len(v)-1])
	}
	return v
}

// SetFieldTable is text with the [fields.<field>.<sub>] table holding
// values, one `value = "x"` line each, sorted: the table rewritten in place,
// else added after [fields.<field>], else at the end; none when values is
// empty. An inline `sub = {…}` in [fields.<field>] goes, values holding
// what it held. Every other line of the file is kept, comments too.
func SetFieldTable(text, field, sub string, values map[string]string) string {
	return setSubTable(text, "fields."+field, sub, values)
}

// SetLabelsTable is SetFieldTable for the [labels] table: [labels.colors],
// [labels.descriptions].
func SetLabelsTable(text, sub string, values map[string]string) string {
	return setSubTable(text, "labels", sub, values)
}

// setSubTable is [parent.sub] written whole, as SetFieldTable says.
func setSubTable(text, parent, sub string, values map[string]string) string {
	lines := strings.Split(text, "\n")
	if start, end := tableSpan(lines, "["+parent+"]"); start >= 0 {
		for i := end - 1; i > start; i-- {
			if m := keyLine.FindStringSubmatch(lines[i]); m != nil && m[2] == sub {
				lines = append(lines[:i], lines[i+1:]...)
			}
		}
	}
	var block []string
	if len(values) > 0 {
		block = append(block, "["+parent+"."+sub+"]")
		for _, k := range slices.Sorted(maps.Keys(values)) {
			key := k
			if !viewName.MatchString(k) { // a bare key is letters, digits, - and _
				key = tomlString(k)
			}
			block = append(block, key+" = "+tomlString(values[k]))
		}
	}
	if start, end := tableSpan(lines, "["+parent+"."+sub+"]"); start >= 0 {
		if len(block) == 0 && start > 0 && strings.TrimSpace(lines[start-1]) == "" {
			start-- // its blank line goes with it
		}
		return strings.Join(slices.Concat(lines[:start], block, lines[end:]), "\n")
	}
	if len(block) == 0 {
		return strings.Join(lines, "\n")
	}
	if _, end := tableSpan(lines, "["+parent+"]"); end >= 0 {
		return strings.Join(slices.Concat(lines[:end], []string{""}, block, lines[end:]), "\n")
	}
	if base := strings.TrimRight(strings.Join(lines, "\n"), "\n"); base != "" {
		return base + "\n\n" + strings.Join(block, "\n") + "\n"
	}
	return strings.Join(block, "\n") + "\n"
}

// tableSpan is where the table under header is in lines: its header, and
// the line after its last key (the blank lines and comments before the next
// table are the next table's); -1, -1 when there is none.
func tableSpan(lines []string, header string) (start, end int) {
	start = slices.IndexFunc(lines, func(l string) bool { return strings.TrimSpace(l) == header })
	if start < 0 {
		return -1, -1
	}
	end = len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "[") {
			end = i
			break
		}
	}
	for end > start+1 {
		if l := strings.TrimSpace(lines[end-1]); l != "" && !strings.HasPrefix(l, "#") {
			break
		}
		end--
	}
	return start, end
}
