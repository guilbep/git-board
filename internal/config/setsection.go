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
// doesn't name goes. A table the file hasn't is appended after the
// anchor's table (a family's new member joins its family, the file's
// areas stay whole), else at the end.
func setTableKeys(text, header, anchor string, keys []string, vals map[string]string, own bool) string {
	lines := strings.Split(text, "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == header {
			start = i
			break
		}
	}
	if start < 0 {
		var block []string
		block = append(block, header)
		for _, k := range keys {
			if vals[k] != "" {
				block = append(block, k+" = "+vals[k])
			}
		}
		if anchor != "" {
			if _, aEnd := tableSpan(lines, anchor); aEnd >= 0 {
				at := aEnd // the span's trailing blanks are its next table's
				if at > 0 && strings.TrimSpace(lines[at-1]) != "" {
					block = append([]string{""}, block...) // one blank line over it
				}
				return strings.Join(slices.Concat(lines[:at], block, lines[at:]), "\n")
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
// subtables (their [[dotted.*]] array tables too): their spans and the
// comment block directly above each header (it describes the table), one
// blank line above gone as well. Nothing there is, nothing changes.
func deleteTable(text, dotted string) string {
	for {
		lines := strings.Split(text, "\n")
		start := -1
		for i, l := range lines {
			t := strings.TrimSpace(l)
			if t == "["+dotted+"]" || t == "[["+dotted+"]]" || strings.HasPrefix(t, "["+dotted+".") || strings.HasPrefix(t, "[["+dotted+".") {
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
		text = strings.Join(slices.Concat(lines[:start], lines[end:]), "\n")
	}
}

// tableSpanOf is tableSpan on the text: where the table under header is,
// -1 when there is none.
func tableSpanOf(text, header string) (start, end int) {
	return tableSpan(strings.Split(text, "\n"), header)
}

// lastTableHeader is the file's last [prefix…] table's header, "" when
// there is none: where a new table of the family joins.
func lastTableHeader(text, prefix string) string {
	last := ""
	for _, l := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "["+prefix+".") && strings.HasSuffix(t, "]") {
			last = t
		}
	}
	return last
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
		text = setTableKeys(text, "[fields."+name+"]", lastTableHeader(text, "fields"), []string{"single", "options", "numeric", "type", "unit"}, vals, false)
		text = SetFieldTable(text, name, "aliases", f.Aliases)
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
	return setTableKeys(text, "[roles]", "", keys, vals, true), nil
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
	text = setTableKeys(text, "[labels]", "", []string{"list"}, map[string]string{"list": tomlArrayAlways(l.List)}, false)
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
		text = setTableKeys(text, "[sprints]", "", keys, vals, false)
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

// tomlTableString is a map as an inline table, sorted: { a = "x" }.
func tomlTableString(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m))
	for _, k := range slices.Sorted(maps.Keys(m)) {
		key := k
		if !viewName.MatchString(k) {
			key = tomlString(k)
		}
		parts = append(parts, key+" = "+tomlString(m[k]))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

// tomlArrayArray is a list of lists as a TOML array of arrays, the argv
// lists of [cycle.tests.<name>] tools: [["go", "version"]].
func tomlArrayArray(list [][]string) string {
	if len(list) == 0 {
		return ""
	}
	parts := make([]string, len(list))
	for i, item := range list {
		parts[i] = tomlArray(item)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// SetBranchesText is text with the [branches] table holding b: base, name,
// and the kinds as [branches.kinds]. Nothing of the three, the table goes.
func SetBranchesText(text string, b Branches) (string, error) {
	vals := map[string]string{"base": tomlString(b.Base), "name": tomlString(b.Name)}
	for k, v := range vals {
		if v == `""` {
			vals[k] = ""
		}
	}
	if vals["base"] == "" && vals["name"] == "" && len(b.Kinds) == 0 {
		return deleteTable(text, "branches"), nil
	}
	text = setTableKeys(text, "[branches]", "", []string{"base", "name"}, vals, false)
	return setSubTable(text, "branches", "kinds", b.Kinds), nil
}

// SetCycleText is text with the [cycle] family on or off and holding c:
// the table's keys, the kinds as [cycle.kinds] string rules (a kind's
// table form holds the same rule, tests — a string writes it back as
// one), and the suites as [cycle.tests.<name>] and [cycle.checks.<name>]
// with their [cycle.tests.<name>.env]. Off, every [cycle.*] table goes.
func SetCycleText(text string, on bool, c Cycle) (string, error) {
	if !on {
		return deleteTable(text, "cycle"), nil
	}
	vals := map[string]string{
		"ready":       tomlString(c.Ready),
		"in-progress": tomlArray(c.InProgress),
		"in-review":   tomlArray(c.InReview),
		"validation":  tomlArray(c.Validation),
		"plans":       tomlString(c.Plans),
		"plans-in-pr": "",
		"test-files":  tomlArray(c.TestFiles),
		"criteria":    tomlArray(c.Criteria),
		"steps":       tomlArray(c.Steps),
		"risk":        tomlString(c.Risk),
		"plan":        tomlString(c.Plan),
		"review":      tomlString(c.Review),
		"spec":        tomlString(c.Spec),
		"signoffs":    tomlTableString(c.Signoffs),
		"signers":     tomlString(c.Signers),
	}
	if c.PlansInPR != nil {
		vals["plans-in-pr"] = strconv.FormatBool(*c.PlansInPR)
	}
	for k, v := range vals {
		if v == `""` {
			vals[k] = ""
		}
	}
	text = setTableKeys(text, "[cycle]", "", slices.Sorted(maps.Keys(vals)), vals, false)
	text = setSubTable(text, "cycle", "kinds", c.Kinds)
	text = setSuites(text, "cycle.tests", c.Tests)
	return setSuites(text, "cycle.checks", c.Checks), nil
}

// setSuites is every suite of a family as its table: [parent.<name>] with
// run, files, where, dir, tools, and env as the subtable. A suite the file
// has that suites doesn't is deleted; a new one joins its family, after
// the family's last table.
func setSuites(text, parent string, suites map[string]Suite) string {
	for _, name := range subTables(text, parent) {
		if _, keep := suites[name]; !keep {
			text = deleteTable(text, parent+"."+name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(suites)) {
		if !viewName.MatchString(name) {
			continue // the check refuses it; the writer writes what it can
		}
		s := suites[name]
		vals := map[string]string{
			"run":   tomlArray(s.Run),
			"files": tomlArray(s.Files),
			"where": tomlString(s.Where),
			"dir":   tomlString(s.Dir),
			"tools": tomlArrayArray(s.Tools),
		}
		for k, v := range vals {
			if v == "" || v == `""` || v == "[]" {
				vals[k] = ""
			}
		}
		text = setTableKeys(text, "["+parent+"."+name+"]", lastTableHeader(text, parent), slices.Sorted(maps.Keys(vals)), vals, false)
		text = setSubTable(text, parent+"."+name, "env", s.Env)
	}
	return text
}

// subTables are the bare names of the [parent.<name>] tables in the text,
// in file order. A quoted name is skipped: suites are bare keys.
func subTables(text, parent string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		t := strings.TrimSpace(l)
		after, ok := strings.CutPrefix(t, "["+parent+".")
		if !ok {
			continue
		}
		name, _, isTable := strings.Cut(after, "]")
		if isTable && viewName.MatchString(name) {
			out = append(out, name)
		}
	}
	return out
}

// SetReposText is text with the [repos] table holding the list and, when
// it isn't native, assignees. No list, the table goes.
func SetReposText(text string, repos []string, assignees string) (string, error) {
	if len(repos) == 0 && (assignees == "" || assignees == "native") {
		return deleteTable(text, "repos"), nil
	}
	vals := map[string]string{"list": tomlArrayAlways(repos), "assignees": ""}
	if assignees != "" && assignees != "native" {
		vals["assignees"] = tomlString(assignees)
	}
	return setTableKeys(text, "[repos]", "", []string{"list", "assignees"}, vals, true), nil
}

// SetUploadsText is text with the [uploads] table holding u. Every key
// empty (the defaults), the table goes.
func SetUploadsText(text string, u Uploads) (string, error) {
	vals := map[string]string{
		"endpoint":   tomlString(u.Endpoint),
		"public_url": tomlString(u.PublicURL),
		"prefix":     tomlString(u.Prefix),
		"region":     tomlString(u.Region),
	}
	empty := true
	for k, v := range vals {
		if v == `""` {
			vals[k] = ""
		} else {
			empty = false
		}
	}
	if empty {
		return deleteTable(text, "uploads"), nil
	}
	return setTableKeys(text, "[uploads]", "", slices.Sorted(maps.Keys(vals)), vals, false), nil
}

// SetReleaseText is text with the [release] family holding r: the table's
// keys, the chart as [release.chart], the deploy targets as
// [[release.deploy]] (replaced whole, in the form's order). Nothing of
// release in it, every [release.*] table goes.
func SetReleaseText(text string, r Release) (string, error) {
	vals := map[string]string{
		"remote":         tomlString(r.Remote),
		"environments":   tomlArray(r.Environments),
		"follow":         "",
		"checks":         tomlString(r.Checks),
		"images":         tomlImages(r.Images),
		"version_values": tomlArray(r.VersionValues),
		"builder":        tomlString(r.Builder),
		"binaries":       tomlArray(r.Binaries),
		"platforms":      tomlArray(r.Platforms),
		"build":          tomlString(r.Build),
	}
	if r.Follow != nil {
		vals["follow"] = tomlString(*r.Follow)
	}
	empty := true
	for k, v := range vals {
		if v == "" || v == `""` || v == "[]" {
			vals[k] = ""
		} else {
			empty = false
		}
	}
	if empty && r.Chart == nil && len(r.Deploy) == 0 {
		return deleteTable(text, "release"), nil
	}
	text = setTableKeys(text, "[release]", "", slices.Sorted(maps.Keys(vals)), vals, false)
	if r.Chart == nil {
		text = deleteTable(text, "release.chart")
	} else {
		chart := map[string]string{"path": r.Chart.Path, "registry": r.Chart.Registry}
		for k, v := range chart {
			if v == "" {
				delete(chart, k)
			}
		}
		text = setSubTable(text, "release", "chart", chart)
	}
	return setDeploy(text, r.Deploy), nil
}

// tomlImages is the images as an inline array of inline tables.
func tomlImages(images []Image) string {
	if len(images) == 0 {
		return ""
	}
	parts := make([]string, len(images))
	for i, im := range images {
		parts[i] = "{ name = " + tomlString(im.Name) + ", context = " + tomlString(im.Context) + " }"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// setDeploy replaces the [[release.deploy]] tables with targets, in order:
// the old ones go whole, the new ones follow the [release] table, else the
// file's end. No targets, none stay.
func setDeploy(text string, targets []Target) string {
	lines := strings.Split(text, "\n")
	for {
		start := slices.IndexFunc(lines, func(l string) bool { return strings.TrimSpace(l) == "[[release.deploy]]" })
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
		lines = slices.Concat(lines[:start], lines[end:])
	}
	if len(targets) == 0 {
		return strings.Join(lines, "\n")
	}
	var block []string
	for i, t := range targets {
		if i > 0 {
			block = append(block, "")
		}
		block = append(block, "[[release.deploy]]",
			"repo = "+tomlString(t.Repo),
			"path = "+tomlString(t.Path))
		if t.Registry != "" {
			block = append(block, "registry = "+tomlString(t.Registry))
		}
	}
	at := len(lines)
	if _, end := tableSpan(lines, "[release]"); end >= 0 {
		at = end
	}
	if at > 0 && strings.TrimSpace(lines[at-1]) != "" {
		block = append([]string{""}, block...) // one blank line over the first
	}
	return strings.Join(slices.Concat(lines[:at], block, lines[at:]), "\n")
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

// MoveFieldText is text with the [fields.<name>] table — its subtables
// ([fields.<name>.colors] and friends) moving with it — where the
// [fields.<before>] table was, the file's order the fields page's list:
// before "" puts it last. The table's comment block moves with it; the
// target's comment block stays its own; the other tables' lines are
// untouched.
func MoveFieldText(text, name, before string) (string, error) {
	if !viewName.MatchString(name) {
		return "", fmt.Errorf("a field's name is letters, digits, - and _, not %q", name)
	}
	if before != "" && !viewName.MatchString(before) {
		return "", fmt.Errorf("a field's name is letters, digits, - and _, not %q", before)
	}
	if name == before {
		return text, nil
	}
	lines := strings.Split(text, "\n")
	start, end := fieldSpan(lines, name)
	if start < 0 {
		return "", fmt.Errorf("no [fields.%s] table to move", name)
	}
	from := start // the comment block above describes the field: it moves too
	for from > 0 && strings.HasPrefix(strings.TrimSpace(lines[from-1]), "#") {
		from--
	}
	block := slices.Clone(lines[from:end])
	without := slices.Concat(lines[:from], lines[end:])
	if from > 0 && strings.TrimSpace(lines[from-1]) == "" && (end >= len(lines) || strings.TrimSpace(lines[end]) == "") {
		without = slices.Concat(lines[:from-1], lines[end:]) // its blank line above goes with it, when one stays below
	}

	to := len(without) // last: the block goes at the end
	if before != "" {
		tStart, _ := fieldSpan(without, before)
		if tStart < 0 {
			return "", fmt.Errorf("no [fields.%s] table to move before", before)
		}
		to = tStart
		for to > 0 && strings.HasPrefix(strings.TrimSpace(without[to-1]), "#") {
			to-- // the comments above the target describe the target: the block goes over them
		}
	}
	if to > 0 && strings.TrimSpace(without[to-1]) != "" {
		block = append([]string{""}, block...) // one blank line over it
	}
	if to >= len(without) || strings.TrimSpace(without[to]) != "" {
		block = append(block, "") // one blank line under it
	}
	out := slices.Concat(without[:to], block, without[to:])
	return strings.Join(out, "\n"), nil
}

// fieldSpan is the [fields.<name>] table's span with its subtables
// ([fields.<name>.colors] and friends): from the table's first header
// line to the line before the next header outside it — the blank lines
// and the comment block above that one stay out — -1 when there is none.
func fieldSpan(lines []string, name string) (start, end int) {
	head := "[fields." + name + "]"
	sub := "[fields." + name + "."
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t == head || strings.HasPrefix(t, sub) {
			start = i
			break
		}
	}
	if start < 0 {
		return -1, -1
	}
	end = len(lines)
	for i := start + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "[") && !strings.HasPrefix(t, sub) {
			end = i
			break
		}
	}
	for end > start+1 { // the next table's comments and blanks are its own
		if l := strings.TrimSpace(lines[end-1]); l != "" && !strings.HasPrefix(l, "#") {
			break
		}
		end--
	}
	return start, end
}
