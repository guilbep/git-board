package config

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// The settings' forms write one section of the file at a time: each writer
// here rewrites only its own tables, the way SetViewText does — every other
// line, comment and their order untouched, a missing table appended, one
// emptied of content removed. The form's save is the whole section's value
// for the tables it owns; a table it doesn't name is left alone.

// setTableKeys is text with the table under header holding keys = vals:
// SetViewText's core, with the header and the keys free. A key's empty
// value removes its line, a missing one is added after the table's last
// key, trailing comments are kept. The table's own key lines stay when
// own is false (a key the writer doesn't know is someone else's — a
// future one); own is true, the form owns the table and a key line it
// doesn't name goes.
func setTableKeys(text, header string, keys []string, vals map[string]string, own bool) string {
	lines := strings.Split(text, "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == header {
			start = i
			break
		}
	}
	if start < 0 {
		var b strings.Builder
		b.WriteString(header + "\n")
		for _, k := range keys {
			if vals[k] != "" {
				b.WriteString(k + " = " + vals[k] + "\n")
			}
		}
		if base := strings.TrimRight(text, "\n"); base != "" {
			return base + "\n\n" + b.String()
		}
		return b.String()
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
	lastKey := -1 // in block: where missing keys go
	for _, l := range lines[start+1 : end] {
		m := keyLine.FindStringSubmatch(l)
		if m == nil {
			block = append(block, l)
			continue
		}
		k := m[2]
		want, ours := vals[k]
		if !ours {
			if own {
				continue // the form owns the table: a key it doesn't name goes
			}
			block = append(block, l)
			lastKey = len(block) - 1
			continue
		}
		seen[k] = true
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
	block = append(block[:lastKey+1], append(missing, block[lastKey+1:]...)...)

	out := append(append(append([]string{}, lines[:start+1]...), block...), lines[end:]...)
	return strings.Join(out, "\n")
}

// deleteTable is text without the [dotted] table and its [dotted.*]
// subtables: their spans and the comment block directly above each header
// (it describes the table), one blank line above gone as well. Nothing
// there is, nothing changes.
func deleteTable(text, dotted string) string {
	for {
		lines := strings.Split(text, "\n")
		start := -1
		for i, l := range lines {
			if t := strings.TrimSpace(l); t == "["+dotted+"]" || strings.HasPrefix(t, "["+dotted+".") {
				start = i
				break
			}
		}
		if start < 0 {
			return text
		}
		end := len(lines)
		for i := start + 1; i < len(lines); i++ {
			if strings.HasPrefix(strings.TrimSpace(lines[i]), "[") {
				end = i
				break
			}
		}
		for start > 0 { // the comment block above describes the table
			prev := strings.TrimSpace(lines[start-1])
			if !strings.HasPrefix(prev, "#") {
				break
			}
			start--
		}
		if start > 0 && strings.TrimSpace(lines[start-1]) == "" && (end >= len(lines) || strings.TrimSpace(lines[end]) == "") {
			start-- // its blank line above goes with it, when one stays below
		}
		text = strings.Join(slices.Concat(lines[:start], lines[end:]), "\n")
	}
}

// tableSpanOf is tableSpan on the text: where the table under header is,
// -1 when there is none.
func tableSpanOf(text, header string) (start, end int) {
	return tableSpan(strings.Split(text, "\n"), header)
}

// SetFieldsText is text with the fields named in set holding their value
// and the fields named in remove gone, one table at a time: every field
// the map doesn't name keeps its lines, comments included. A field's
// subtables ([fields.<name>.project|colors|descriptions]) follow the
// field's value, as SetFieldTable writes them.
func SetFieldsText(text string, set map[string]Field, remove []string) (string, error) {
	for _, name := range remove {
		text = deleteTable(text, "fields."+name)
	}
	for _, name := range slices.Sorted(maps.Keys(set)) {
		if !viewName.MatchString(name) {
			return "", fmt.Errorf("a field's name is letters, digits, - and _, not %q", name)
		}
		f := set[name]
		vals := map[string]string{}
		if f.Single {
			vals["single"] = "true"
		}
		if len(f.Options) > 0 {
			vals["options"] = tomlArray(f.Options)
		}
		if f.Numeric {
			vals["numeric"] = "true"
		}
		if f.Type != "" {
			vals["type"] = tomlString(f.Type)
		}
		if f.Unit != "" {
			vals["unit"] = tomlString(f.Unit)
		}
		text = setTableKeys(text, "[fields."+name+"]", []string{"single", "options", "numeric", "type", "unit"}, vals, false)
		text = SetFieldTable(text, name, "project", f.Project)
		text = SetFieldTable(text, name, "colors", f.Colors)
		text = SetFieldTable(text, name, "descriptions", f.Descriptions)
	}
	return text, nil
}

// SetRolesText is text with the [roles] table holding roles whole: the
// form owns the table. None, the table goes.
func SetRolesText(text string, roles map[string]string) (string, error) {
	keys, vals := []string{}, map[string]string{}
	for _, role := range slices.Sorted(maps.Keys(roles)) {
		if roles[role] == "" {
			continue
		}
		if !viewName.MatchString(role) {
			return "", fmt.Errorf("a role's name is letters, digits, - and _, not %q", role)
		}
		keys = append(keys, role)
		vals[role] = tomlString(roles[role])
	}
	if len(keys) == 0 {
		return deleteTable(text, "roles"), nil
	}
	return setTableKeys(text, "[roles]", keys, vals, true), nil
}

// tomlArrayAlways is tomlArray that writes an empty list as []: [labels]
// with an empty list is the project's "no plain label allowed", worth its
// line.
func tomlArrayAlways(list []string) string {
	if len(list) == 0 {
		return "[]"
	}
	return tomlArray(list)
}

// SetLabelsText is text with the [labels] table holding l: the list line,
// the colors and descriptions as SetLabelsTable writes them. Without a
// table and nothing to hold, the file is untouched; with one, the list
// stays explicit even when empty (an empty list allows none).
func SetLabelsText(text string, l Labels) (string, error) {
	start, _ := tableSpanOf(text, "[labels]")
	hasTable := start >= 0
	if !hasTable && len(l.List) == 0 && len(l.Colors) == 0 && len(l.Descriptions) == 0 {
		return text, nil
	}
	text = setTableKeys(text, "[labels]", []string{"list"}, map[string]string{"list": tomlArrayAlways(l.List)}, false)
	text = SetLabelsTable(text, "colors", l.Colors)
	text = SetLabelsTable(text, "descriptions", l.Descriptions)
	return text, nil
}

// SetSprintsText is text with the [sprints] table holding s: its keys
// rewritten in place, the [[sprints.list]] entries the form's list (they
// are data: slug, title, start, three lines each), written after the
// table, else at the end.
func SetSprintsText(text string, s SprintsConfig) (string, error) {
	// the old entries go first: they are replaced, not edited in place
	for {
		lines := strings.Split(text, "\n")
		start := slices.IndexFunc(lines, func(l string) bool { return strings.TrimSpace(l) == "[[sprints.list]]" })
		if start < 0 {
			break
		}
		end := len(lines)
		for i := start + 1; i < len(lines); i++ {
			if strings.HasPrefix(strings.TrimSpace(lines[i]), "[") {
				end = i
				break
			}
		}
		for end > start+1 { // the comments before the next table are its
			if l := strings.TrimSpace(lines[end-1]); l != "" && !strings.HasPrefix(l, "#") {
				break
			}
			end--
		}
		if start > 0 && strings.TrimSpace(lines[start-1]) == "" {
			start--
		}
		text = strings.Join(slices.Concat(lines[:start], lines[end:]), "\n")
	}
	var keys []string
	vals := map[string]string{}
	if s.LengthDays > 0 {
		keys = append(keys, "length_days")
		vals["length_days"] = strconv.Itoa(s.LengthDays)
	}
	if s.StartWeekday != "" {
		keys = append(keys, "start_weekday")
		vals["start_weekday"] = tomlString(s.StartWeekday)
	}
	if len(keys) > 0 {
		text = setTableKeys(text, "[sprints]", keys, vals, false)
	}
	if len(s.List) == 0 {
		return text, nil
	}
	// the entries follow the [sprints] table, else the file's end
	lines := strings.Split(text, "\n")
	at := len(lines)
	if start, end := tableSpan(lines, "[sprints]"); start >= 0 {
		at = end
	}
	var block []string
	for i, sp := range s.List {
		if i > 0 {
			block = append(block, "")
		}
		block = append(block, "[[sprints.list]]",
			"slug = "+tomlString(sp.Slug),
			"title = "+tomlString(sp.Title),
			"start = "+sp.Start.Format("2006-01-02"))
	}
	if at > 0 && strings.TrimSpace(lines[at-1]) != "" {
		block = append([]string{""}, block...) // one blank line over the first
	}
	return strings.Join(slices.Concat(lines[:at], block, lines[at:]), "\n"), nil
}

// DeleteViewText is text without the [views.<name>] table.
func DeleteViewText(text, name string) (string, error) {
	if !viewName.MatchString(name) {
		return "", fmt.Errorf("a view's name is letters, digits, - and _, not %q", name)
	}
	return deleteTable(text, "views."+name), nil
}

// MoveViewText is text with the [views.<name>] table where [views.before]
// was, the file's order the tabs': before "" puts it last. The table's
// comment block moves with it; the other tables' lines are untouched.
func MoveViewText(text, name, before string) (string, error) {
	if !viewName.MatchString(name) {
		return "", fmt.Errorf("a view's name is letters, digits, - and _, not %q", name)
	}
	if before != "" && !viewName.MatchString(before) {
		return "", fmt.Errorf("a view's name is letters, digits, - and _, not %q", before)
	}
	if name == before {
		return text, nil
	}
	lines := strings.Split(text, "\n")
	start, end := tableSpan(lines, "[views."+name+"]")
	if start < 0 {
		return "", fmt.Errorf("no [views.%s] table to move", name)
	}
	from := start // the comment block above describes the view: it moves too
	for from > 0 && strings.HasPrefix(strings.TrimSpace(lines[from-1]), "#") {
		from--
	}
	block := lines[from:end]
	for len(block) > 0 && strings.TrimSpace(block[len(block)-1]) == "" {
		block = block[:len(block)-1] // the file's trailing newline doesn't move with it
	}
	block = append(slices.Clone(block), "") // one blank line under it
	without := slices.Concat(lines[:from], lines[end:])
	if from > 0 && strings.TrimSpace(lines[from-1]) == "" && (end >= len(lines) || strings.TrimSpace(lines[end]) == "") {
		without = slices.Concat(lines[:from-1], lines[end:]) // its blank line above goes with it, when one stays below
	}

	to := len(without) // last: the block goes at the end
	if before != "" {
		tStart, _ := tableSpan(without, "[views."+before+"]")
		if tStart < 0 {
			return "", fmt.Errorf("no [views.%s] table to move before", before)
		}
		to = tStart // the comments above the target stay where they are: the next table is theirs
	}
	if to > 0 {
		prev := strings.TrimSpace(without[to-1])
		if prev != "" && !strings.HasPrefix(prev, "#") {
			block = append([]string{""}, block...) // one blank line over it
		}
	}
	out := slices.Concat(without[:to], block, without[to:])
	return strings.Join(out, "\n"), nil
}
