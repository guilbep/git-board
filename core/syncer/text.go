package syncer

import (
	"net/url"
	"regexp"
	"strings"
	"time"
)

// marker names a git-board issue, comment or version in a text sync
// posts. quoted says the text starts with a quote line, after its header;
// keys are the "k=v" pairs a comment's marker carries after its id.
func marker(kind, id string, quoted bool, keys string) string {
	if quoted {
		id += " quoted"
	}
	if keys != "" {
		id += " " + keys
	}
	return "<!-- git-board " + kind + " " + id + " -->"
}

var (
	markerRE = regexp.MustCompile(`(?:^|\n\n)<!-- git-board (issue|comment|version) ([0-9a-f]{40}(?::[0-9]+)?)( quoted)?((?: [a-z][a-z-]*=[^\s>]*)*) -->$`)
	headerRE = regexp.MustCompile(`^\*From [^\n]*, \d{4}-\d{2}-\d{2}, (?:in|on) [^\n]*\.\*(?:\n\n|$)`)
	quoteRE  = regexp.MustCompile(`^> [^\n]*(?:\n\n|$)`)
)

// header is the line naming who wrote a text, when, and where.
func header(name string, written int64, where string) string {
	return "*From " + name + ", " + time.Unix(written, 0).UTC().Format("2006-01-02") + ", " + where + ".*"
}

// wrapped is a text as sync posts it: a header, a quote, the text, a
// marker. Kind, id and keys are the marker's. fork is the fork's own start of a text it
// posted, kept as it is: its marker, forkID its id, and its header, whose
// link is the original's URL.
type wrapped struct{ header, quote, body, marker, kind, id, keys, fork, forkID, link string }

// unwrap splits a text sync posted, or the fork did. A text with neither
// form is all body.
func unwrap(text string) wrapped {
	w := wrapped{body: text}
	m := markerRE.FindStringSubmatchIndex(text)
	if m == nil {
		w.fork, w.forkID, w.link = forkStart(text)
		w.body = text[len(w.fork):]
		return w
	}
	w.kind, w.id, w.keys = text[m[2]:m[3]], text[m[4]:m[5]], strings.TrimPrefix(text[m[8]:m[9]], " ")
	w.body = text[:m[0]]
	if h := headerRE.FindString(w.body); h != "" {
		w.header, w.body = strings.TrimSuffix(h, "\n\n"), w.body[len(h):]
	}
	if q := quoteRE.FindString(w.body); m[6] >= 0 && q != "" {
		w.quote, w.body = strings.TrimSuffix(q, "\n\n"), w.body[len(q):]
	}
	w.mark(w.keys)
	return w
}

// The fork's form (sync.md's "Switching from the fork"): its marker alone on
// the first line, with a word after its id ("attributed") when it posted
// someone else's text, which then starts with a header in italics linking
// the original issue or comment.
var (
	forkMarkerRE = regexp.MustCompile(`^<!-- git-bug:([0-9a-f]{64})(?: [a-z]+)? -->(?:\n|$)`)
	forkHeaderRE = regexp.MustCompile(`^\*Originally posted by [^\n]*\[view on [^\]\n]*\]\(([^)\s]+)\)\*(?:\n\n|\n|$)`)
)

// forkStart is the fork's marker and header at the start of a text, the
// marker's id, and the URL of the original the header links.
func forkStart(text string) (start, id, link string) {
	m := forkMarkerRE.FindStringSubmatch(text)
	if m == nil {
		return "", "", ""
	}
	start, id = m[0], m[1]
	if h := forkHeaderRE.FindStringSubmatch(text[len(start):]); h != nil {
		start, link = start+h[0], h[1]
	}
	return start, id, link
}

// commentLinked is the original comment a fork header's link names: the
// forge's host and the comment's id, from GitHub's #issuecomment-<id> or
// GitLab's #note_<id>; "" for an issue's link.
func commentLinked(link string) (host, id string) {
	u, err := url.Parse(link)
	if err != nil {
		return "", ""
	}
	for _, prefix := range []string{"issuecomment-", "note_"} {
		if n, ok := strings.CutPrefix(u.Fragment, prefix); ok && n != "" && strings.Trim(n, "0123456789") == "" {
			return u.Host, n
		}
	}
	return "", ""
}

// forked says the text is in the fork's form.
func (w wrapped) forked() bool { return w.fork != "" }

// mark sets the marker for w's kind, id and quote, carrying keys.
func (w *wrapped) mark(keys string) { w.marker = marker(w.kind, w.id, w.quote != "", keys) }

// String joins the parts that aren't empty with a blank line, after the
// fork's start.
func (w wrapped) String() string {
	var parts []string
	for _, p := range []string{w.header, w.quote, w.body, w.marker} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return w.fork + strings.Join(parts, "\n\n")
}

var (
	codeTicks = regexp.MustCompile("`+")
	refLink   = regexp.MustCompile(`\[[^\]\n]*#[0-9]+\]\([^)\s]*\)|#[0-9]+`)
	spaces    = regexp.MustCompile(`\s+`)
)

// SameText says two texts are one, as a mirror shows it: without the fork's
// start, and with mentions and references as either side translated them.
func SameText(a, b string) bool { return same(a, b) }

func same(a, b string) bool {
	norm := func(s string) string {
		s = codeTicks.ReplaceAllString(unwrap(s).body, "")
		return strings.TrimSpace(spaces.ReplaceAllString(refLink.ReplaceAllString(s, "#"), " "))
	}
	return norm(a) == norm(b)
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
