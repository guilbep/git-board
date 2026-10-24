package syncer

import (
	"regexp"
	"strings"
	"time"
)

// marker names a git-board issue or comment in a text sync posts.
func marker(kind, id string) string { return "<!-- git-board " + kind + " " + id + " -->" }

var (
	markerRE = regexp.MustCompile(`(?:^|\n\n)<!-- git-board (issue|comment) ([0-9a-f]{40}(?::[0-9]+)?) -->$`)
	headerRE = regexp.MustCompile(`^\*From [^\n]*, \d{4}-\d{2}-\d{2}, (?:in|on) [^\n]*\.\*(?:\n\n|$)`)
)

// header is the line naming who wrote a text, when, and where.
func header(name string, written int64, where string) string {
	return "*From " + name + ", " + time.Unix(written, 0).UTC().Format("2006-01-02") + ", " + where + ".*"
}

// wrapped is a text as sync posts it: a header, the text, a marker. Kind
// and id are the marker's.
type wrapped struct{ header, body, marker, kind, id string }

// unwrap splits a text sync posted. A text without a marker is all body.
func unwrap(text string) wrapped {
	w := wrapped{body: text}
	m := markerRE.FindStringSubmatchIndex(text)
	if m == nil {
		return w
	}
	w.kind, w.id = text[m[2]:m[3]], text[m[4]:m[5]]
	w.marker, w.body = marker(w.kind, w.id), text[:m[0]]
	if h := headerRE.FindString(w.body); h != "" {
		w.header, w.body = strings.TrimSuffix(h, "\n\n"), w.body[len(h):]
	}
	return w
}

// String joins the parts that aren't empty with a blank line.
func (w wrapped) String() string {
	var parts []string
	for _, p := range []string{w.header, w.body, w.marker} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "\n\n")
}

var token = regexp.MustCompile("`+|@[A-Za-z0-9_][A-Za-z0-9_.-]*|#[0-9]+")

// translate defuses a text's mentions, unless it is a title, and gives
// each reference #n what ref answers. Code spans are left as they are: a
// run of backticks opens one only when a run as long closes it.
// ponytail: code spans by backtick runs only; indented code blocks and
// HTML aren't recognized.
func translate(text string, title bool, ref func(n string) string) string {
	var b strings.Builder
	toks := token.FindAllStringIndex(text, -1)
	last := 0
	for i := 0; i < len(toks); i++ {
		s, e := toks[i][0], toks[i][1]
		if s < last {
			continue
		}
		tok := text[s:e]
		var prev byte
		if s > 0 {
			prev = text[s-1]
		}
		switch tok[0] {
		case '`':
			for j := i + 1; j < len(toks); j++ {
				if text[toks[j][0]:toks[j][1]] == tok {
					e, i = toks[j][1], j
					break
				}
			}
			b.WriteString(text[last:e])
			last = e
		case '@':
			if title || isWord(prev) {
				continue
			}
			login := strings.TrimRight(tok[1:], ".")
			b.WriteString(text[last:s])
			if prev == '`' {
				b.WriteByte(' ') // a lone backtick before would join the span's
			}
			b.WriteString("`@" + login + "`")
			last = s + 1 + len(login)
		case '#':
			if isWord(prev) || prev == '&' || prev == '/' || (e < len(text) && isWord(text[e])) {
				continue
			}
			b.WriteString(text[last:s])
			b.WriteString(ref(tok[1:]))
			last = e
		}
	}
	b.WriteString(text[last:])
	return b.String()
}

func isWord(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
