package config

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// A key of .git-board.toml, for git board config, is its dotted path:
// github-project.owner, sprints.length_days, views.backlog.filter. A key
// is set on its own line in its table, the way the settings' forms write
// theirs (setTableKeys): every other line and comment stays.

// ErrUnknownKey: git-board doesn't read this key.
var ErrUnknownKey = errors.New("git-board doesn't know this key")

// ErrNoKey: the file doesn't set this key.
var ErrNoKey = errors.New("not set")

// KeyValue is one key the file sets, and its value as TOML writes it.
type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// GetKey is the value of a dotted key in the file's text, as TOML writes
// it: "a string" quoted, [an, array], 14.
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
	return literal(v), nil
}

// ListKeys are every key the file sets, a leaf each, in order; an array of
// tables ([[sprints.list]]) is one key, its value an array of inline
// tables.
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
			out = append(out, KeyValue{prefix + k, literal(t[k])})
		}
	}
	walk("", m)
	return out, nil
}

// literal is v as TOML writes a value.
func literal(v any) string {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(map[string]any{"v": v}); err != nil {
		return fmt.Sprint(v)
	}
	s := strings.TrimSpace(buf.String())
	if rest, ok := strings.CutPrefix(s, "v = "); ok && !strings.Contains(rest, "\n") {
		return rest
	}
	// a table or an array of tables: inline, one line
	switch t := v.(type) {
	case map[string]any:
		var parts []string
		for _, k := range slices.Sorted(maps.Keys(t)) {
			parts = append(parts, k+" = "+literal(t[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []map[string]any:
		var parts []string
		for _, e := range t {
			parts = append(parts, literal(e))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	return strings.ReplaceAll(s, "\n", " ")
}

// Value is what a command line's value is in TOML: itself when it is one
// (true, 14, ["a", "b"], "quoted"), else a string.
func Value(s string) string {
	var m map[string]any
	if _, err := toml.Decode("v = "+s, &m); err == nil && !strings.Contains(s, "\n") {
		return s
	}
	return tomlString(s)
}

// SetKeyText is text with key set to value, a TOML literal (Value), or
// removed when value is "". Only a table's own key is set: a key at the
// top, or one inside an array of tables, is refused. The result is checked
// as Load checks a file, and refused when git-board doesn't read the key
// or the file no longer holds together.
func SetKeyText(text, key, value string) (string, error) {
	i := strings.LastIndexByte(key, '.')
	if i <= 0 || i == len(key)-1 {
		return "", fmt.Errorf("%s: %w", key, ErrUnknownKey)
	}
	table, name := key[:i], key[i+1:]
	for _, part := range strings.Split(key, ".") {
		if part == "" || strings.Trim(part, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") != "" {
			return "", fmt.Errorf("%q: a key is names of letters, digits, _ and -, joined by dots", key)
		}
	}
	if strings.Contains(text, "[["+table+"]]") {
		return "", fmt.Errorf("%s: in an array of tables, edit %s itself", key, FileName)
	}
	out := setTableKeys(text, "["+table+"]", "", []string{name}, map[string]string{name: value}, false)
	_, md, err := parse([]byte(out))
	if err != nil {
		return "", fmt.Errorf("%s = %s: %w", key, value, err)
	}
	for _, k := range md.Undecoded() {
		if k.String() == key || strings.HasPrefix(k.String(), key+".") {
			return "", fmt.Errorf("%s: %w", key, ErrUnknownKey)
		}
	}
	return out, nil
}
