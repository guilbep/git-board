package store

import (
	"cmp"
	"encoding/json"
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/syncer"
	"github.com/guilbep/git-board/internal/markdown"
)

// The Go side of web/src/sections.ts, for what sync posts: the body's
// sections, their slugs, the version count. Same rules, same vectors.

type section struct{ slug, heading, html string }

var (
	cutTag = regexp.MustCompile(`(?i)<(/?)(blockquote|h[1-6])\b[^>]*>`)
	anyTag = regexp.MustCompile(`<[^>]*>`)
	spaces = regexp.MustCompile(`\s+`)
)

// textOf is HTML's text: tags stripped, entities decoded, one line.
func textOf(h string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(html.UnescapeString(anyTag.ReplaceAllString(h, "")), " "))
}

// slugs: lower case, runs of other characters as one "-", an index on a
// repeat; taken holds slugs already used.
func slugs(headings []string, taken ...string) []string {
	seen := map[string]int{}
	for _, t := range taken {
		seen[t] = 1
	}
	out := make([]string, len(headings))
	for i, h := range headings {
		base := strings.Trim(strings.Join(strings.FieldsFunc(strings.ToLower(h), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		}), "-"), "-")
		if base == "" {
			base = "section"
		}
		seen[base]++
		out[i] = base
		if n := seen[base]; n > 1 {
			out[i] = base + "-" + strconv.Itoa(n)
		}
	}
	return out
}

// sectionsOf cuts rendered HTML at its h1–h6 outside any blockquote: the
// prose before the first heading is "intro"; no heading, one section.
func sectionsOf(h string) []section {
	type cut struct {
		at      int
		heading string
	}
	var cuts []cut
	depth := 0
	for _, m := range cutTag.FindAllStringSubmatchIndex(h, -1) {
		closing, tag := m[3] > m[2], strings.ToLower(h[m[4]:m[5]])
		switch {
		case tag == "blockquote" && closing:
			depth--
		case tag == "blockquote":
			depth++
		case !closing && depth == 0:
			end := strings.Index(h[m[1]:], "</"+tag+">")
			if end < 0 {
				end = len(h) - m[1]
			}
			cuts = append(cuts, cut{m[0], textOf(h[m[1] : m[1]+end])})
		}
	}
	var out []section
	introEnd := len(h)
	if len(cuts) > 0 {
		introEnd = cuts[0].at
	}
	if intro := h[:introEnd]; strings.TrimSpace(intro) != "" || len(cuts) == 0 {
		out = append(out, section{slug: "intro", html: intro})
	}
	var heads []string
	for _, c := range cuts {
		heads = append(heads, c.heading)
	}
	var taken []string
	if len(out) > 0 {
		taken = []string{"intro"}
	}
	named := slugs(heads, taken...)
	for i, c := range cuts {
		end := len(h)
		if i+1 < len(cuts) {
			end = cuts[i+1].at
		}
		out = append(out, section{slug: named[i], heading: c.heading, html: h[c.at:end]})
	}
	return out
}

// item is the text of a section's k-th list item, 1-based, nested lists
// left out; "" when there is none.
// ponytail: items counted by <li> tags, a nested item counts, as sections.ts
func (s section) item(k int) string {
	f := strings.Split(s.html, "<li")
	if k < 1 || k >= len(f) {
		return ""
	}
	t := f[k][strings.Index(f[k], ">")+1:]
	for _, end := range []string{"</li>", "<ul", "<ol"} {
		if i := strings.Index(t, end); i >= 0 {
			t = t[:i]
		}
	}
	return textOf(t)
}

var box = regexp.MustCompile(`(?m)^(\s*[-*+]\s+)\[[ xX]\]`)

// isTick: the two texts differ only by "- [ ]" ↔ "- [x]" flips.
func isTick(before, after string) bool {
	return before != after && box.ReplaceAllString(before, "$1[ ]") == box.ReplaceAllString(after, "$1[ ]")
}

// bodyHistory is the description's texts, oldest first, from its log: the
// create's, then each edit-text of it.
func bodyHistory(l *oplog.Log) []Version {
	var h []Version
	for _, e := range l.Entries {
		var d struct{ Body, Target string }
		switch e.Kind {
		case "create":
			_ = json.Unmarshal(e.Data, &d)
			h = append(h, Version{ID: e.ID, Body: d.Body})
		case "edit-text":
			if _ = json.Unmarshal(e.Data, &d); len(h) > 0 && d.Target == h[0].ID {
				h = append(h, Version{ID: e.ID, Body: d.Body})
			}
		}
	}
	return h
}

// versions groups a body's history by version, v1 first: each its
// opening entry's id and its last text, ticks included.
func versions(h []Version) []Version {
	var out []Version
	for i, x := range h {
		if i > 0 && (x.Body == h[i-1].Body || isTick(h[i-1].Body, x.Body)) {
			out[len(out)-1].Body = x.Body
			continue
		}
		out = append(out, x)
	}
	return out
}

// sectionsOfText renders a body and cuts it.
func sectionsOfText(body string) []section {
	return sectionsOf(markdown.Render(body, func(u string) string { return u }))
}

// carried are the keys a comment's marker carries, by their short names,
// in that order (docs/design/time-flow/protocol.md).
var carried = [][2]string{
	{"step", "cycle:step"}, {"anchor", "thread:anchor"}, {"reply-to", "thread:reply-to"}, {"kind", "thread:kind"},
	{"resolved", "thread:resolved"}, {"applied", "thread:applied"}, {"verdict", "spec:verdict"}, {"on", "spec:on"},
}

// carriable: a marker's value has no space and no ">".
// ponytail: no escaping, none of the protocol's values has either
func carriable(v string) bool { return v != "" && !strings.ContainsAny(v, " \t\n>") }

var anchorRE = regexp.MustCompile(`^body@v([1-9][0-9]*)/([^/#\s]+)(?:#([1-9][0-9]*))?$`)

// meanings gives sync's options what git-board's keys mean on a forge: the
// marker's keys, the quote over a thread comment, the note on a version
// published, a reply's root. Each reads the issue from s when called.
// ponytail: an issue's log read per comment posted, cache it if runs grow
func meanings(s *gitstore.Store, opt *syncer.Options) {
	opt.Carry = func(meta map[string]string) string {
		var kv []string
		for _, k := range carried {
			if v := meta[k[1]]; carriable(v) {
				kv = append(kv, k[0]+"="+v)
			}
		}
		return strings.Join(kv, " ")
	}
	opt.Restore = func(keys string, marked bool, meta map[string]string) map[string]string {
		out := map[string]string{}
		if !marked {
			for k, v := range meta {
				if v != "" && (strings.HasPrefix(k, "thread:") || strings.HasPrefix(k, "spec:")) {
					out[k] = ""
				}
			}
			return out
		}
		got := map[string]string{}
		for _, kv := range strings.Fields(keys) {
			k, v, _ := strings.Cut(kv, "=")
			got[k] = v
		}
		for _, k := range carried {
			if v := meta[k[1]]; got[k[0]] != v && (v == "" || carriable(v)) {
				out[k[1]] = got[k[0]]
			}
		}
		return out
	}
	opt.ReplyTo = func(meta map[string]string) string { return meta["thread:reply-to"] }
	history := func(id string) []Version {
		l, err := oplog.Load(s, "issue", id)
		if err != nil {
			return nil
		}
		return versions(bodyHistory(l))
	}
	opt.Quote = func(issueID string, c issue.Comment) string {
		m := anchorRE.FindStringSubmatch(c.Meta["thread:anchor"])
		if m == nil {
			return ""
		}
		n, _ := strconv.Atoi(m[1])
		where, item := "§ "+m[2], ""
		if vs := history(issueID); n <= len(vs) {
			for _, sec := range sectionsOfText(vs[n-1].Body) {
				if sec.slug != m[2] {
					continue
				}
				where = "§ " + cmp.Or(sec.heading, "intro")
				if k, _ := strconv.Atoi(m[3]); k > 0 {
					item = sec.item(k)
				}
			}
		}
		if m[3] != "" {
			where += " · " + m[3]
		}
		q := "> **On v" + m[1] + " · " + where + "**"
		if item != "" {
			q += " · _" + item + "_"
		}
		return q
	}
	opt.Published = func(issueID, before, after string) (string, string) {
		vs := history(issueID)
		if before == after || isTick(before, after) || len(vs) < 2 {
			return "", ""
		}
		was := sectionsOfText(before)
		var changed []string
		for _, sec := range sectionsOfText(after) {
			i := slices.IndexFunc(was, func(w section) bool { return w.slug == sec.slug })
			if i < 0 || unchecked(was[i].html) != unchecked(sec.html) {
				changed = append(changed, "§ "+cmp.Or(sec.heading, "intro"))
			}
		}
		note := "**Published v" + strconv.Itoa(len(vs)) + "**"
		if len(changed) > 0 {
			note += " · " + strings.Join(changed, ", ") + " changed"
		}
		return vs[len(vs)-1].ID, note
	}
}

var checked = regexp.MustCompile(`\s+checked(="[^"]*")?`)

// unchecked is HTML with its boxes unticked: a tick changes no section.
func unchecked(h string) string { return checked.ReplaceAllString(h, "") }
