package sync

import (
	"net/url"
	"regexp"
	"strings"
	"time"
)

// MarkerOf names a git-board issue, comment or version in a text sync
// posts. quoted says the text starts with a quote line, after its header;
// keys are the "k=v" pairs a comment's marker carries after its id.
func MarkerOf(kind, id string, quoted bool, keys string) string {
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

// HeaderLine is the line naming who wrote a text, when, and where.
func HeaderLine(name string, written int64, where string) string {
	return "*From " + name + ", " + time.Unix(written, 0).UTC().Format("2006-01-02") + ", " + where + ".*"
}

// Wrapped is a text as sync posts it: a header, a quote, the text, a
// marker. Kind, id and keys are the marker's. fork is the fork's own start of a text it
// posted, kept as it is: its marker, forkID its id, and its header, whose
// link is the original's URL.
type Wrapped struct{ Header, Quote, Body, Marker, Kind, ID, Keys, Fork, ForkID, Link string }

// Unwrap splits a text sync posted, or the fork did. A text with neither
// form is all body.
func Unwrap(text string) Wrapped {
	w := Wrapped{Body: text}
	m := markerRE.FindStringSubmatchIndex(text)
	if m == nil {
		w.Fork, w.ForkID, w.Link = ForkStart(text)
		w.Body = text[len(w.Fork):]
		return w
	}
	w.Kind, w.ID, w.Keys = text[m[2]:m[3]], text[m[4]:m[5]], strings.TrimPrefix(text[m[8]:m[9]], " ")
	w.Body = text[:m[0]]
	if h := headerRE.FindString(w.Body); h != "" {
		w.Header, w.Body = strings.TrimSuffix(h, "\n\n"), w.Body[len(h):]
	}
	if q := quoteRE.FindString(w.Body); m[6] >= 0 && q != "" {
		w.Quote, w.Body = strings.TrimSuffix(q, "\n\n"), w.Body[len(q):]
	}
	w.Mark(w.Keys)
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

// ForkStart is the fork's marker and header at the start of a text, the
// marker's id, and the URL of the original the header links.
func ForkStart(text string) (start, id, link string) {
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

// CommentLinked is the original comment a fork header's link names: the
// forge's host and the comment's id, from GitHub's #issuecomment-<id> or
// GitLab's #note_<id>; "" for an issue's link.
func CommentLinked(link string) (host, id string) {
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

// Forked says the text is in the fork's form.
func (w Wrapped) Forked() bool { return w.Fork != "" }

// Mark sets the marker for w's kind, id and quote, carrying keys.
func (w *Wrapped) Mark(keys string) { w.Marker = MarkerOf(w.Kind, w.ID, w.Quote != "", keys) }

// String joins the parts that aren't empty with a blank line, after the
// fork's start.
func (w Wrapped) String() string {
	var parts []string
	for _, p := range []string{w.Header, w.Quote, w.Body, w.Marker} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return w.Fork + strings.Join(parts, "\n\n")
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
		s = codeTicks.ReplaceAllString(Unwrap(s).Body, "")
		return strings.TrimSpace(spaces.ReplaceAllString(refLink.ReplaceAllString(s, "#"), " "))
	}
	return norm(a) == norm(b)
}

var token = regexp.MustCompile("`+|@[A-Za-z0-9_][A-Za-z0-9_.-]*|#[0-9]+")

// Translate defuses a text's mentions, unless it is a title, and gives
// each reference #n what ref answers. Code spans are left as they are: a
// run of backticks opens one only when a run as long closes it.
// ponytail: code spans by backtick runs only; indented code blocks and
// HTML aren't recognized.
func Translate(text string, title bool, ref func(n string) string) string {
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
