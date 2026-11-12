package conf

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// The writers v1's settings forms and init used beside SetKeys: a new
// table joining its family, a sub-table written whole, a table replaced
// whole. Like SetKeys, each rewrites only its own table: every other
// line, comment and their order are untouched.

// SetKeysAfter is SetKeys, but a table the file hasn't goes after the
// anchor table's last key, a blank line over it: a family's new member
// joins its family ([fields.kind] after the last [fields.*]). Without
// anchor, or with an anchor the file hasn't, it is appended.
func SetKeysAfter(text, table, anchor string, keys []string, vals map[string]string) string {
	lines := strings.Split(text, "\n")
	if anchor == "" || slices.ContainsFunc(lines, func(l string) bool { return isHeader(l, table) }) {
		return SetKeys(text, table, keys, vals)
	}
	_, at := tableSpan(lines, anchor)
	if at < 0 {
		return SetKeys(text, table, keys, vals)
	}
	block := []string{"[" + table + "]"}
	for _, k := range keys {
		if vals[k] != "" {
			block = append(block, k+" = "+vals[k])
		}
	}
	if at > 0 && strings.TrimSpace(lines[at-1]) != "" {
		block = append([]string{""}, block...)
	}
	return strings.Join(slices.Concat(lines[:at], block, lines[at:]), "\n")
}

// LastTable is the file's last [prefix.…] table, "" when it has none:
// the anchor of a new table of the family (SetKeysAfter).
func LastTable(text, prefix string) string {
	last := ""
	for _, l := range strings.Split(text, "\n") {
		if h, arr := headerOf(l); !arr && strings.HasPrefix(h, prefix+".") {
			last = h
		}
	}
	return last
}

// SetSubTable is text with the [parent.sub] table holding values, one
// `value = "x"` line each, sorted: the table rewritten in place, else
// added after [parent], else at the end; none when values is empty. An
// inline `sub = {…}` in [parent] goes, values holding what it held.
func SetSubTable(text, parent, sub string, values map[string]string) string {
	lines := strings.Split(text, "\n")
	if start, end := tableSpan(lines, parent); start >= 0 {
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
			if !SimpleName(k) {
				key = Quote(k)
			}
			block = append(block, key+" = "+Quote(values[k]))
		}
	}
	if start, end := tableSpan(lines, parent+"."+sub); start >= 0 {
		if len(block) == 0 && start > 0 && strings.TrimSpace(lines[start-1]) == "" {
			start-- // its blank line goes with it
		}
		return strings.Join(slices.Concat(lines[:start], block, lines[end:]), "\n")
	}
	if len(block) == 0 {
		return strings.Join(lines, "\n")
	}
	if _, end := tableSpan(lines, parent); end >= 0 {
		return strings.Join(slices.Concat(lines[:end], []string{""}, block, lines[end:]), "\n")
	}
	if base := strings.TrimRight(strings.Join(lines, "\n"), "\n"); base != "" {
		return base + "\n\n" + strings.Join(block, "\n") + "\n"
	}
	return strings.Join(block, "\n") + "\n"
}

// ReplaceTable is text with [table] replaced whole by block (its header
// and lines, ending in a newline), the comments over the next table left
// to it; a file without the table has block appended, a blank line over
// it.
func ReplaceTable(text, table, block string) string {
	lines := strings.Split(text, "\n")
	start, end := -1, len(lines)
	for i, l := range lines {
		if start < 0 {
			if isHeader(l, table) {
				start = i
			}
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(l), "[") {
			end = i
			break
		}
	}
	if end < len(lines) {
		for end > start+1 && (strings.TrimSpace(lines[end-1]) == "" || strings.HasPrefix(strings.TrimSpace(lines[end-1]), "#")) {
			end--
		}
		for strings.TrimSpace(lines[end]) == "" {
			end++
		}
	}
	if start < 0 {
		if strings.TrimSpace(text) == "" {
			return block
		}
		return strings.TrimRight(text, "\n") + "\n\n" + block
	}
	var b strings.Builder
	if before := strings.Join(lines[:start], "\n"); before != "" {
		b.WriteString(before)
		b.WriteString("\n")
	}
	b.WriteString(block)
	if after := strings.Join(lines[end:], "\n"); strings.TrimSpace(after) != "" {
		b.WriteString("\n")
		b.WriteString(after)
	}
	return b.String()
}

// tableSpan is where [table] is in lines: its header, and the line after
// its last key (the blank lines and comments before the next table are
// the next table's); -1, -1 when there is none.
func tableSpan(lines []string, table string) (start, end int) {
	start = slices.IndexFunc(lines, func(l string) bool { return isHeader(l, table) })
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

// List is a list of strings as a TOML array, on one line: ["a", "b"];
// "" for none.
func List(list []string) string {
	if len(list) == 0 {
		return ""
	}
	quoted := make([]string, len(list))
	for i, s := range list {
		quoted[i] = Quote(s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// keyAt is where key's line is in [table]: its index in lines and its
// parts (keyLine); -1 when the table or the key isn't there.
func keyAt(lines []string, table, key string) (int, []string) {
	start, end := tableSpan(lines, table)
	for i := start + 1; start >= 0 && i < end; i++ {
		if m := keyLine.FindStringSubmatch(lines[i]); m != nil && m[2] == key {
			return i, m
		}
	}
	return -1, nil
}

// keyName is a key as TOML writes it: quoted unless letters, digits, _
// and -.
func keyName(name string) string {
	if SimpleName(name) {
		return name
	}
	return Quote(name)
}

// InsertInList is text with value added to [table] key, a one-line array
// of strings: after the value after, at the end without it or when the
// array hasn't it; every value there stays, as it is spelled. A missing
// key is added. An array over several lines is refused: its line alone
// can't be rewritten.
func InsertInList(text, table, key, value, after string) (string, error) {
	lines := strings.Split(text, "\n")
	i, m := keyAt(lines, table, key)
	if i < 0 {
		return SetKeys(text, table, []string{key}, map[string]string{key: List([]string{value})}), nil
	}
	arr := m[3]
	if !strings.HasPrefix(arr, "[") || !strings.HasSuffix(arr, "]") {
		return "", fmt.Errorf("%s.%s is written over several lines: add %s by hand", table, key, Quote(value))
	}
	inner := strings.TrimSpace(arr[1 : len(arr)-1])
	at := -1
	if q := Quote(after); after != "" {
		if at = strings.Index(inner, q); at >= 0 {
			at += len(q)
		}
	}
	switch {
	case inner == "":
		inner = Quote(value)
	case at < 0:
		inner = strings.TrimRight(inner, ", ") + ", " + Quote(value)
	default:
		inner = inner[:at] + ", " + Quote(value) + inner[at:]
	}
	lines[i] = m[1] + "[" + inner + "]" + m[4]
	return strings.Join(lines, "\n"), nil
}

// AddInline is text with [table] key, a table, holding one entry more,
// name = literal: at the end of an inline table ({ a = "a" }), its
// entries as they were, in their order; as a line of [table.key] when
// the file writes it so; else as a new inline table, after the table's
// last key.
func AddInline(text, table, key, name, literal string) string {
	entry := keyName(name) + " = " + literal
	lines := strings.Split(text, "\n")
	i, m := keyAt(lines, table, key)
	if i >= 0 && strings.HasPrefix(m[3], "{") && strings.HasSuffix(m[3], "}") {
		inner := strings.TrimSpace(m[3][1 : len(m[3])-1])
		if inner != "" {
			inner = strings.TrimRight(inner, ", ") + ", "
		}
		lines[i] = m[1] + "{ " + inner + entry + " }" + m[4]
		return strings.Join(lines, "\n")
	}
	if hasHeader(text, table+"."+key) {
		return SetKeys(text, table+"."+key, []string{keyName(name)}, map[string]string{keyName(name): literal})
	}
	return SetKeys(text, table, []string{key}, map[string]string{key: "{ " + entry + " }"})
}

// AddArrayTable is text with block, a [[key]] table, after the last
// [[key]] one, else after [parent], else at the end: a blank line over
// it, the comments over the next table left to that table.
func AddArrayTable(text, key, parent, block string) string {
	lines := strings.Split(text, "\n")
	start := -1
	for i, l := range lines {
		if h, arr := headerOf(l); arr && h == key {
			start = i
		}
	}
	if start < 0 {
		start = slices.IndexFunc(lines, func(l string) bool { return isHeader(l, parent) })
	}
	if start < 0 {
		return strings.TrimRight(text, "\n") + "\n\n" + block
	}
	end := len(lines)
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
	out := slices.Concat(lines[:end], []string{""}, strings.Split(strings.TrimRight(block, "\n"), "\n"), lines[end:])
	if end == len(lines) {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}
