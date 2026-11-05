package conf

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// The file is written as a person writes it: a key on its own line in
// its table, every other line and comment as it was. One writer, SetKeys,
// does it for every table (v1 had one per settings form).

// ErrUnknownKey: git-board doesn't read this key.
var ErrUnknownKey = errors.New("git-board doesn't know this key")

// ErrNoKey: the file doesn't set this key.
var ErrNoKey = errors.New("not set")

// KeyValue is a key the file sets, and its value as TOML writes it.
type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// GetKey is a dotted key's value in text, as TOML writes it: "a string"
// quoted, [an, array], 14.
func GetKey(text, key string) (string, error) {
	var m map[string]any
	if _, err := toml.Decode(text, &m); err != nil {
		return "", err
	}
	var v any = m
	for _, part := range strings.Split(key, ".") {
		t, ok := v.(map[string]any)
		if !ok {
			return "", fmt.Errorf("%s: %w", key, ErrNoKey)
		}
		if v, ok = t[part]; !ok {
			return "", fmt.Errorf("%s: %w", key, ErrNoKey)
		}
	}
	return Literal(v), nil
}

// ListKeys are the keys text sets, a leaf each, sorted; an array of
// tables is one key, its value an array of inline tables.
func ListKeys(text string) ([]KeyValue, error) {
	var m map[string]any
	if _, err := toml.Decode(text, &m); err != nil {
		return nil, err
	}
	var out []KeyValue
	var walk func(prefix string, t map[string]any)
	walk = func(prefix string, t map[string]any) {
		for _, k := range slices.Sorted(maps.Keys(t)) {
			if sub, ok := t[k].(map[string]any); ok {
				walk(prefix+k+".", sub)
				continue
			}
			out = append(out, KeyValue{prefix + k, Literal(t[k])})
		}
	}
	walk("", m)
	return out, nil
}

// Literal is v as TOML writes a value, on one line.
func Literal(v any) string {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(map[string]any{"v": v}); err != nil {
		return fmt.Sprint(v)
	}
	s := strings.TrimSpace(buf.String())
	if rest, ok := strings.CutPrefix(s, "v = "); ok && !strings.Contains(rest, "\n") {
		return rest
	}
	switch t := v.(type) { // a table or an array of tables: inline
	case map[string]any:
		var parts []string
		for _, k := range slices.Sorted(maps.Keys(t)) {
			parts = append(parts, k+" = "+Literal(t[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []map[string]any:
		var parts []string
		for _, e := range t {
			parts = append(parts, Literal(e))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []any:
		var parts []string
		for _, e := range t {
			parts = append(parts, Literal(e))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	return strings.ReplaceAll(s, "\n", " ")
}

// Value is a command line's value in TOML: itself when it is a TOML value
// (true, 14, ["a", "b"], "quoted"), else a string.
func Value(s string) string {
	var m map[string]any
	if _, err := toml.Decode("v = "+s, &m); err == nil && !strings.Contains(s, "\n") {
		return s
	}
	return Quote(s)
}

// Quote is s as a TOML basic string.
func Quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`).Replace(s) + `"`
}

// unquote reads back a value Quote wrote, or a literal 'string'.
func unquote(v string) string {
	switch {
	case len(v) >= 2 && v[0] == '\'':
		return v[1 : len(v)-1]
	case len(v) >= 2 && v[0] == '"':
		return strings.NewReplacer(`\\`, `\`, `\"`, `"`, `\n`, "\n", `\t`, "\t").Replace(v[1 : len(v)-1])
	}
	return v
}

var keyLine = regexp.MustCompile(`^(\s*([A-Za-z0-9_-]+)\s*=\s*)(\[(?:[^\[\]]|\[[^\[\]]*\])*\]|\{[^}]*\}|"(?:[^"\\]|\\.)*"|'[^']*'|[^#\s]+)(.*)$`)

// SimpleName says name is one key's name: letters, digits, _ and -.
func SimpleName(name string) bool {
	return name != "" && strings.Trim(name, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") == ""
}

// SplitKey checks a dotted key and splits it into its table and its own
// name: a key at the top, or inside an array of tables, isn't one.
func SplitKey(text, key string) (table, name string, err error) {
	i := strings.LastIndexByte(key, '.')
	if i <= 0 || i == len(key)-1 {
		return "", "", fmt.Errorf("%s: %w", key, ErrUnknownKey)
	}
	for _, part := range strings.Split(key, ".") {
		if !SimpleName(part) {
			return "", "", fmt.Errorf("%q: a key is names of letters, digits, _ and -, joined by dots", key)
		}
	}
	table, name = key[:i], key[i+1:]
	if strings.Contains(text, "[["+table+"]]") {
		return "", "", fmt.Errorf("%s: in an array of tables, edit %s itself", key, FileName)
	}
	return table, name, nil
}

// SetKeys is text with [table] holding vals, TOML literals by key: a
// key's line rewritten in place (its trailing comment kept), a value ""
// removing it, a missing key added after the table's last key, in keys'
// order. A table the file hasn't is appended, a blank line over it. Every
// other line stays as it was.
func SetKeys(text, table string, keys []string, vals map[string]string) string {
	header := "[" + table + "]"
	lines := strings.Split(text, "\n")
	start := slices.IndexFunc(lines, func(l string) bool { return isHeader(l, table) })
	if start < 0 {
		block := []string{header}
		for _, k := range keys {
			if vals[k] != "" {
				block = append(block, k+" = "+vals[k])
			}
		}
		if base := strings.TrimRight(text, "\n"); base != "" {
			return base + "\n\n" + strings.Join(block, "\n") + "\n"
		}
		return strings.Join(block, "\n") + "\n"
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "[") {
			end = i
			break
		}
	}
	var block []string
	seen := map[string]bool{}
	lastKey := -1 // in block: where the missing keys go
	for _, l := range lines[start+1 : end] {
		m := keyLine.FindStringSubmatch(l)
		if m == nil {
			block = append(block, l)
			continue
		}
		want, ours := vals[m[2]]
		if !ours {
			block = append(block, l)
			lastKey = len(block) - 1
			continue
		}
		seen[m[2]] = true
		if want == "" {
			continue
		}
		if m[3] != want && unquote(m[3]) != unquote(want) {
			l = m[1] + want + m[4]
		}
		block = append(block, l)
		lastKey = len(block) - 1
	}
	var missing []string
	for _, k := range keys {
		if !seen[k] && vals[k] != "" {
			missing = append(missing, k+" = "+vals[k])
		}
	}
	block = slices.Concat(block[:lastKey+1], missing, block[lastKey+1:])
	return strings.Join(slices.Concat(lines[:start+1], block, lines[end:]), "\n")
}

// tableHeader matches a table's header line: the spaces TOML allows in
// the brackets, a comment after them.
var tableHeader = regexp.MustCompile(`^\s*\[\s*([^\[\]]*?)\s*\]\s*(#.*)?$`)

// isHeader says line is table's header: [ a . b ]  # c is [a.b]. (v1
// matched the line exactly, and wrote a header with a comment twice.)
func isHeader(line, table string) bool {
	m := tableHeader.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	var parts []string
	for _, p := range strings.Split(m[1], ".") {
		parts = append(parts, strings.TrimSpace(p))
	}
	return strings.Join(parts, ".") == table
}
