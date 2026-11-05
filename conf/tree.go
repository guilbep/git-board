package conf

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// SetValue is text with the dotted key holding v, a whole subtree: what
// a settings form saves (PATCH /api/config). nil removes the key, a
// table with its sub-tables. A table (a map) replaces the key's table:
// its keys set in place, those it hasn't removed, a sub-table each
// likewise, so the lines it keeps keep their comments. An array of
// tables replaces the [[key]] blocks where the first one was. Anything
// else is SetKeys's one line. A string written where the file holds a
// date, and that reads as one, stays a date (a sprint's start).
func SetValue(text, key string, v any) (string, error) {
	var doc map[string]any
	if _, err := toml.Decode(text, &doc); err != nil {
		return "", err
	}
	out, err := setValue(text, key, v, lookup(doc, key))
	if parent, _ := splitLast(key); err == nil && v == nil && parent != "" {
		out = dropEmpty(out, parent) // [views] once its last view goes
	}
	return out, err
}

func setValue(text, key string, v, old any) (string, error) {
	v = asDate(v, old)
	parent, name := splitLast(key)
	switch x := v.(type) {
	case nil:
		text = removeBlocks(text, func(h string, arr bool) bool { return h == key || strings.HasPrefix(h, key+".") })
		if parent != "" || name != key {
			text = SetKeys(text, parent, []string{name}, map[string]string{name: ""})
		}
		return text, nil
	case map[string]any:
		oldT, isTable := old.(map[string]any)
		headed := hasHeader(text, key) || hasHeaderUnder(text, key)
		if isTable && !headed { // inline in its parent: one line
			if parent == "" {
				return "", fmt.Errorf("%s: %w", key, ErrUnknownKey)
			}
			return SetKeys(text, parent, []string{name}, map[string]string{name: Literal(x)}), nil
		}
		if !isTable { // a new table: its header, then its keys
			if old != nil && parent != "" {
				text = SetKeys(text, parent, []string{name}, map[string]string{name: ""})
			}
			if text = strings.TrimRight(text, "\n"); text != "" {
				text += "\n\n"
			}
			text += "[" + key + "]\n"
		}
		var err error
		for _, k := range slices.Sorted(maps.Keys(oldT)) {
			if _, keep := x[k]; !keep {
				if text, err = setValue(text, key+"."+k, nil, oldT[k]); err != nil {
					return "", err
				}
			}
		}
		for _, k := range orderOf(oldT, x) {
			if text, err = setValue(text, key+"."+k, x[k], oldT[k]); err != nil {
				return "", err
			}
		}
		return text, nil
	case []any:
		if tables(x) && (len(x) > 0 || hasArrayHeader(text, key)) {
			return setArray(text, key, x, old), nil
		}
	}
	if parent == "" {
		return "", fmt.Errorf("%s: %w", key, ErrUnknownKey)
	}
	if _, isTable := old.(map[string]any); isTable && hasHeader(text, key) { // a table becomes a value
		text = removeBlocks(text, func(h string, _ bool) bool { return h == key || strings.HasPrefix(h, key+".") })
	}
	return SetKeys(text, parent, []string{name}, map[string]string{name: Literal(v)}), nil
}

// orderOf is v's keys, those old has first in old's sorted order, then
// the new ones sorted: a stable order for the lines added.
func orderOf(old, v map[string]any) []string {
	var out []string
	for _, k := range slices.Sorted(maps.Keys(v)) {
		if _, ok := old[k]; ok {
			out = append(out, k)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(v)) {
		if _, ok := old[k]; !ok {
			out = append(out, k)
		}
	}
	return out
}

// setArray writes an array of tables as [[key]] blocks, where the first
// of the old ones was, or at the end.
func setArray(text, key string, list []any, old any) string {
	oldList, _ := old.([]map[string]any)
	var block []string
	for i, e := range list {
		var was map[string]any
		if i < len(oldList) {
			was = oldList[i]
		}
		t, _ := e.(map[string]any)
		block = append(block, "[["+key+"]]")
		for _, k := range orderOf(was, t) {
			block = append(block, k+" = "+Literal(asDate(t[k], was[k])))
		}
		block = append(block, "")
	}
	lines := strings.Split(text, "\n")
	at := slices.IndexFunc(lines, func(l string) bool { h, arr := headerOf(l); return arr && h == key })
	if at < 0 {
		return strings.TrimRight(text, "\n") + "\n\n" + strings.Join(block, "\n")
	}
	text = removeBlocks(text, func(h string, arr bool) bool { return h == key || strings.HasPrefix(h, key+".") })
	lines = strings.Split(text, "\n")
	at = min(at, len(lines))
	return strings.Join(slices.Concat(lines[:at], block, lines[at:]), "\n")
}

func tables(list []any) bool {
	for _, e := range list {
		if _, ok := e.(map[string]any); !ok {
			return false
		}
	}
	return true
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// asDate is v as a date when it is a string that reads as one and old
// was one.
func asDate(v, old any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	switch t := old.(type) {
	case time.Time:
		if dateRe.MatchString(s) { // in old's zone: a local date stays one
			if d, err := time.Parse(time.DateOnly, s); err == nil {
				// (the encoder writes a local date's UTC day)
				return d.In(t.Location())
			}
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	return v
}

func lookup(doc map[string]any, key string) any {
	var v any = doc
	for _, part := range strings.Split(key, ".") {
		t, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		if v, ok = t[part]; !ok {
			return nil
		}
	}
	return v
}

func splitLast(key string) (string, string) {
	i := strings.LastIndexByte(key, '.')
	if i < 0 {
		return "", key
	}
	return key[:i], key[i+1:]
}

// headerOf is a header line's table, and whether it is an array's.
func headerOf(line string) (string, bool) {
	arr := false
	m := arrayHeader.FindStringSubmatch(line)
	if m != nil {
		arr = true
	} else if m = tableHeader.FindStringSubmatch(line); m == nil {
		return "", false
	}
	var parts []string
	for _, p := range strings.Split(m[1], ".") {
		parts = append(parts, strings.TrimSpace(p))
	}
	return strings.Join(parts, "."), arr
}

func hasHeader(text, key string) bool {
	for _, l := range strings.Split(text, "\n") {
		if h, arr := headerOf(l); !arr && h == key {
			return true
		}
	}
	return false
}

func hasHeaderUnder(text, key string) bool {
	for _, l := range strings.Split(text, "\n") {
		if h, _ := headerOf(l); strings.HasPrefix(h, key+".") {
			return true
		}
	}
	return false
}

func hasArrayHeader(text, key string) bool {
	for _, l := range strings.Split(text, "\n") {
		if h, arr := headerOf(l); arr && h == key {
			return true
		}
	}
	return false
}

// removeBlocks is text without the tables whose header drop takes: the
// header, its lines, and the comments and blank lines just over it.
func removeBlocks(text string, drop func(table string, array bool) bool) string {
	lines := strings.Split(text, "\n")
	var out []string
	skipping := false
	for _, l := range lines {
		if h, arr := headerOf(l); h != "" || arr {
			skipping = drop(h, arr)
			if skipping {
				for len(out) > 0 && (strings.TrimSpace(out[len(out)-1]) == "" || strings.HasPrefix(strings.TrimSpace(out[len(out)-1]), "#")) {
					out = out[:len(out)-1]
				}
				if len(out) > 0 {
					out = append(out, "")
				}
				continue
			}
		}
		if !skipping {
			out = append(out, l)
		}
	}
	for len(out) > 1 && out[len(out)-1] == "" && out[len(out)-2] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// dropEmpty removes [table] when it holds nothing but blank lines and
// no sub-table needs it there.
func dropEmpty(text, table string) string {
	lines := strings.Split(text, "\n")
	start := slices.IndexFunc(lines, func(l string) bool { return isHeader(l, table) })
	if start < 0 {
		return text
	}
	for i := start + 1; i < len(lines); i++ {
		if _, arr := headerOf(lines[i]); strings.HasPrefix(strings.TrimSpace(lines[i]), "[") || arr {
			break
		}
		if strings.TrimSpace(lines[i]) != "" {
			return text
		}
	}
	return removeBlocks(text, func(h string, arr bool) bool { return !arr && h == table })
}
