package conf

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// The edits a migration is made of. Each changes the lines it names,
// written as SetKeys writes them (a key = value line under its [table]
// header) and leaves every other byte; a key it can't find in that form
// but the file sets (inline, dotted) is an error, not a silent miss: it
// checks the file it gives back, parsed. A key the file doesn't set is
// nothing to do.

// Edit is a change of a file's text.
type Edit func(text string) (string, error)

// Chain is edits one after the other.
func Chain(edits ...Edit) Edit {
	return func(text string) (string, error) {
		for _, e := range edits {
			var err error
			if text, err = e(text); err != nil {
				return "", err
			}
		}
		return text, nil
	}
}

// RenameKey renames [table] old to new, its value and comment as they
// were. RenameKey(table, new, old) undoes it.
func RenameKey(table, old, new string) Edit {
	return func(text string) (string, error) {
		lines, i := find(text, table, old)
		if i >= 0 {
			at := strings.Index(lines[i], old)
			lines[i] = lines[i][:at] + new + lines[i][at+len(old):]
			text = strings.Join(lines, "\n")
		}
		return text, check(text, table, old, false)
	}
}

// MoveKey moves key's line from [from] to [to], after [to]'s last key,
// [to] made at the end of the file when it has none. MoveKey(to, from,
// key) undoes it when key was [from]'s last and [to] was there.
func MoveKey(from, to, key string) Edit {
	return func(text string) (string, error) {
		lines, i := find(text, from, key)
		if i < 0 {
			return text, check(text, from, key, false)
		}
		j, err := extent(lines, i, from, key)
		if err != nil {
			return "", err
		}
		value := slices.Clone(lines[i:j])
		lines = slices.Delete(lines, i, j)
		line := strings.Join(value, "\n")
		start, end := block(lines, to)
		if start < 0 {
			base := strings.TrimRight(strings.Join(lines, "\n"), "\n")
			text = base + "\n\n[" + to + "]\n" + line + "\n"
			if base == "" {
				text = "[" + to + "]\n" + line + "\n"
			}
		} else {
			at := start + 1
			for j := start + 1; j < end; j++ {
				if keyLine.MatchString(lines[j]) {
					if e, err := extent(lines, j, to, ""); err == nil {
						at, j = e, e-1
					}
				}
			}
			text = strings.Join(slices.Insert(lines, at, value...), "\n")
		}
		if err := check(text, from, key, false); err != nil {
			return "", err
		}
		return text, check(text, to, key, true)
	}
}

// arrayHeader matches an array of tables' header: [[a.b]].
var arrayHeader = regexp.MustCompile(`^\s*\[\[\s*([^\[\]]*?)\s*\]\]\s*(#.*)?$`)

// RenameTable renames [old] and the tables under it, [old.x] and
// [[old.x]]: their headers. RenameTable(new, old) undoes it.
func RenameTable(old, new string) Edit {
	return func(text string) (string, error) {
		lines := strings.Split(text, "\n")
		for i, l := range lines {
			m := arrayHeader.FindStringSubmatch(l)
			if m == nil {
				m = tableHeader.FindStringSubmatch(l)
			}
			if m == nil {
				continue
			}
			name := dotted(m[1])
			if name != old && !strings.HasPrefix(name, old+".") {
				continue
			}
			at := strings.Index(l, m[1])
			lines[i] = l[:at] + new + strings.TrimPrefix(name, old) + l[at+len(m[1]):]
		}
		text = strings.Join(lines, "\n")
		return text, check(text, "", old, false)
	}
}

// MapValue rewrites [table] key's value, a TOML literal, with f: its
// comment stays. A MapValue with f's inverse undoes it.
func MapValue(table, key string, f func(literal string) (string, error)) Edit {
	return func(text string) (string, error) {
		lines, i := find(text, table, key)
		if i < 0 {
			return text, check(text, table, key, false)
		}
		if j, err := extent(lines, i, table, key); err != nil {
			return "", err
		} else if j != i+1 {
			return "", fmt.Errorf("[%s] %s: a value over several lines, which this migration can't map; write it on one line, or edit it by hand", table, key)
		}
		m := keyLine.FindStringSubmatch(lines[i])
		v, err := f(m[3])
		if err != nil {
			return "", fmt.Errorf("[%s] %s: %w", table, key, err)
		}
		lines[i] = m[1] + v + m[4]
		text = strings.Join(lines, "\n")
		return text, check(text, table, key, true)
	}
}

// DropKey removes [table] key's line. Nothing undoes it: a migration
// that drops a key is lossy, unless it uses CommentKey.
func DropKey(table, key string) Edit {
	return func(text string) (string, error) {
		lines, i := find(text, table, key)
		if i >= 0 {
			j, err := extent(lines, i, table, key)
			if err != nil {
				return "", err
			}
			text = strings.Join(slices.Delete(lines, i, j), "\n")
		}
		return text, check(text, table, key, false)
	}
}

// CommentKey sets [table] key aside: its lines become comments, why
// beside the first. The file keeps its lines, a value over several (an
// array, a multi-line string) all of them, and the value is there for
// UncommentKey(table, key, why), which undoes it.
func CommentKey(table, key, why string) Edit {
	return func(text string) (string, error) {
		lines, i := find(text, table, key)
		if i >= 0 {
			j, err := extent(lines, i, table, key)
			if err != nil {
				return "", err
			}
			lines[i] = "# " + lines[i] + "  # " + why
			for k := i + 1; k < j; k++ {
				lines[k] = "# " + lines[k]
			}
			text = strings.Join(lines, "\n")
		}
		return text, check(text, table, key, false)
	}
}

// UncommentKey gives back the lines CommentKey set aside with why: the
// first, beside why, and the commented lines after it up to the value's
// end. A file without them is left as it is.
func UncommentKey(table, key, why string) Edit {
	return func(text string) (string, error) {
		lines := strings.Split(text, "\n")
		start, end := block(lines, table)
		for i := start + 1; start >= 0 && i < end; i++ {
			l, pre := strings.CutPrefix(lines[i], "# ")
			l, post := strings.CutSuffix(l, "  # "+why)
			if !pre || !post {
				continue
			}
			if m := keyLine.FindStringSubmatch(l); m == nil || m[2] != key {
				continue
			}
			value := []string{l}
			for j := i + 1; !decodes(value); j++ {
				next, ok := "", false
				if j < len(lines) {
					next, ok = strings.CutPrefix(lines[j], "# ")
				}
				if !ok {
					return "", fmt.Errorf("[%s] %s: its value, set aside over several lines, doesn't end before line %d; edit it by hand", table, key, j+1)
				}
				value = append(value, next)
			}
			copy(lines[i:], value)
			text = strings.Join(lines, "\n")
			return text, check(text, table, key, true)
		}
		return text, nil
	}
}

// extent is the index after the last line of the value whose key is on
// lines[i]: i+1 for a value on its line, more for an array or a
// multi-line string. A value that never ends is an error, never a line
// left behind.
func extent(lines []string, i int, table, key string) (int, error) {
	for j := i + 1; j <= len(lines); j++ {
		if decodes(lines[i:j]) {
			return j, nil
		}
	}
	return 0, fmt.Errorf("[%s] %s: its value doesn't end; edit it by hand", table, cmp.Or(key, strings.TrimSpace(lines[i])))
}

// decodes says lines, a key and its value, are TOML on their own.
func decodes(lines []string) bool {
	var v map[string]any
	_, err := toml.Decode(strings.Join(lines, "\n"), &v)
	return err == nil
}

// block is [table]'s lines, its header's index and the one after its
// last line; -1 when the file has no such header.
func block(lines []string, table string) (start, end int) {
	start = slices.IndexFunc(lines, func(l string) bool { return isHeader(l, table) })
	if start < 0 {
		return -1, -1
	}
	for end = start + 1; end < len(lines); end++ {
		if strings.HasPrefix(strings.TrimSpace(lines[end]), "[") {
			break
		}
		// an array's lines, [1, 2] on its own, aren't a header
		if keyLine.MatchString(lines[end]) {
			if e, err := extent(lines, end, table, ""); err == nil {
				end = e - 1
			}
		}
	}
	return start, end
}

// find is text's lines and [table] key's own line among them, -1 when
// it has none.
func find(text, table, key string) ([]string, int) {
	lines := strings.Split(text, "\n")
	start, end := block(lines, table)
	for i := start + 1; start >= 0 && i < end; i++ {
		m := keyLine.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		if m[2] == key {
			return lines, i
		}
		// a multi-line value's lines aren't keys, though one reads x = 1
		if e, err := extent(lines, i, table, m[2]); err == nil {
			i = e - 1
		}
	}
	return lines, -1
}

// check parses an edit's text: it is TOML, and sets table.key (a table
// itself when table is "") or not, as want says. A key no line but a
// comment names isn't set: that needs no parse (a read migrates in
// memory, and parses the text it gives once, after).
func check(text, table, key string, want bool) error {
	if !want && !named(text, key) {
		return nil
	}
	var v any
	if _, err := toml.Decode(text, &v); err != nil {
		return err
	}
	path := key
	if table != "" {
		path = table + "." + key
	}
	for _, part := range strings.Split(path, ".") {
		t, ok := v.(map[string]any)
		if !ok {
			v = nil
			break
		}
		v = t[part]
	}
	switch {
	case want && v == nil:
		return fmt.Errorf("%s: not set after the edit", path)
	case !want && v != nil && table == "":
		return fmt.Errorf("%s: written in a form this migration can't edit (inline, or dotted keys); write it as a [%s] table, or edit it by hand", path, path)
	case !want && v != nil:
		return fmt.Errorf("%s: written in a form this migration can't edit (inline, or a dotted key); write it as %s = … under [%s], or edit it by hand", path, key, table)
	}
	return nil
}

// named says a line of text that isn't a comment has name in it.
func named(text, name string) bool {
	for l := range strings.Lines(text) {
		if strings.Contains(l, name) && !strings.HasPrefix(strings.TrimSpace(l), "#") {
			return true
		}
	}
	return false
}

// dotted is a header's name without the spaces TOML allows: a . b is a.b.
func dotted(name string) string {
	parts := strings.Split(name, ".")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return strings.Join(parts, ".")
}
