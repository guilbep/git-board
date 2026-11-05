package issues

// The filter grammar, copied from v1's core/filter and owned here: GitHub's search
// syntax where they meet: field:value, -field:value,
// field:@current|@next|@previous|@me, is:open|closed, title:*glob*,
// author:login, involves:login|@me (the author, a person field, a comment, a
// mention), follows:@me (what your inbox follows), label:name, no:field
// (no:label: no plain label), #2528 (an issue's number on a forge), CO2-2528
// (its project number), created:, updated:, closed: (>2026-10-01, a..b,
// @today-7d), comments:>5, in:title,body,comments (where free text looks),
// free text and "quoted phrases". Conditions
// are AND'd; a,b is either value (status:ready,qa), as in GitHub's search.
// Pure logic, and the only parser of the grammar: the CLI, the TUI, the
// web API and so the web page all go through it. The page explains it in
// web/src/filterhelp.ts (and docs/guide/configuration.md, Filters): a word
// added here is added there too.

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/next/kernel/search"
)

// Qualifier is what a Condition matches against.
type Qualifier string

const (
	QField    Qualifier = "field"
	QIs       Qualifier = "is"
	QTitle    Qualifier = "title"
	QText     Qualifier = "text"
	QAuthor   Qualifier = "author"
	QLabel    Qualifier = "label"
	QNo       Qualifier = "no"
	QNumber   Qualifier = "number" // #2528
	QInvolves Qualifier = "involves"
	QFollows  Qualifier = "follows"
	QDate     Qualifier = "date"     // created:, updated:, closed: (Field)
	QComments Qualifier = "comments" // comments:>5
	QIn       Qualifier = "in"       // in:title,body,comments: where free text looks
)

// Condition is one AND'd term of a parsed filter expression.
type Condition struct {
	Kind    Qualifier
	Field   string // set for QField
	Value   string
	Negated bool
	// Hits are, for free text, the issues the full-text index (title,
	// description, comments, or where in: says) matches, by id; nil: the
	// issue's own texts are searched (search.Match), as the index would.
	Hits map[string]bool
	// Mentions are, for involves: and follows:, the issues whose text
	// mentions the login, by id (board fills it); nil: none.
	Mentions map[string]bool
	// MeID is, for @me in involves: and follows:, your identity (board
	// fills it): you are it as well as your login.
	MeID string
	// Mine are, for @me without --me, every name a label may name you by
	// (board fills it): your forge logins and your name. @me matches when
	// any does, so assignee:Ada, written before Ada had an account, is
	// still hers once @me is her login. Nil: @me is me alone.
	Mine []string
	// Followed is, for follows:@me, what your inbox follows besides what
	// you're in (board fills it); nil: only what you're in.
	Followed *Followed
}

// ParseFilter splits a filter string into whitespace-separated AND'd conditions.
func ParseFilter(text string) []Condition {
	var conditions []Condition
	for _, term := range splitTerms(text) {
		token := strings.ReplaceAll(term, `"`, "")
		negated := strings.HasPrefix(token, "-")
		body := token
		if negated {
			body = token[1:]
		}
		fieldName, value, hasSep := cut(body, ":")
		switch {
		case !hasSep && len(body) > 1 && body[0] == '#' && digits(body[1:]):
			conditions = append(conditions, Condition{Kind: QNumber, Value: body[1:], Negated: negated})
		case !hasSep:
			conditions = append(conditions, Condition{Kind: QText, Value: body, Negated: negated})
		case fieldName == "is":
			conditions = append(conditions, Condition{Kind: QIs, Value: value, Negated: negated})
		case fieldName == "title":
			conditions = append(conditions, Condition{Kind: QTitle, Value: value, Negated: negated})
		case fieldName == "author":
			conditions = append(conditions, Condition{Kind: QAuthor, Value: value, Negated: negated})
		case fieldName == "label":
			conditions = append(conditions, Condition{Kind: QLabel, Value: value, Negated: negated})
		case fieldName == "involves":
			conditions = append(conditions, Condition{Kind: QInvolves, Value: value, Negated: negated})
		case fieldName == "follows":
			conditions = append(conditions, Condition{Kind: QFollows, Value: value, Negated: negated})
		case fieldName == "no":
			conditions = append(conditions, Condition{Kind: QNo, Value: value, Negated: negated})
		case fieldName == "created" || fieldName == "updated" || fieldName == "closed":
			conditions = append(conditions, Condition{Kind: QDate, Field: fieldName, Value: value, Negated: negated})
		case fieldName == "comments":
			conditions = append(conditions, Condition{Kind: QComments, Field: fieldName, Value: value, Negated: negated})
		case fieldName == "in":
			conditions = append(conditions, Condition{Kind: QIn, Field: fieldName, Value: value, Negated: negated})
		default:
			conditions = append(conditions, Condition{Kind: QField, Field: fieldName, Value: value, Negated: negated})
		}
	}
	return conditions
}

// splitTerms splits a filter at spaces, but not inside "quotes": a quoted
// phrase, or a quoted value (label:"nice to have"), is one term, as typed.
func splitTerms(text string) []string {
	var res []string
	var cur strings.Builder
	quoted, any := false, false
	for _, r := range text {
		switch {
		case r == '"':
			quoted, any = !quoted, true
			cur.WriteRune(r)
		case !quoted && (r == ' ' || r == '\t' || r == '\n'):
			if any {
				res = append(res, cur.String())
				cur.Reset()
				any = false
			}
		default:
			cur.WriteRune(r)
			any = true
		}
	}
	if any {
		res = append(res, cur.String())
	}
	return res
}

// Without is text without the terms drop picks, the others as typed: the
// filter a facet writes its own term after, a view's without its is:.
func Without(text string, drop func(Condition) bool) string {
	var keep []string
	for _, term := range splitTerms(text) {
		if c := ParseFilter(term); len(c) == 1 && drop(c[0]) {
			continue
		}
		keep = append(keep, term)
	}
	return strings.Join(keep, " ")
}

func digits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

// itself: a word that is an issue's number (2528: its project number once
// the project numbers its issues, else a forge's), its project number as
// written (CO2-2528), or the start of its git-bug id, 7 characters at least
// as git-bug shows it, finds that issue too.
func itself(issue Issue, word string) bool {
	if digits(word) {
		return issue.HasNumber(word)
	}
	if issue.Number != "" && strings.EqualFold(word, issue.Number) {
		return true
	}
	word = strings.ToLower(word)
	return len(word) >= 7 && strings.Trim(word, "0123456789abcdef") == "" && strings.HasPrefix(issue.ID, word)
}

func cut(s, sep string) (before, after string, found bool) {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

var sprintAliases = map[string]bool{"@current": true, "@next": true, "@previous": true}

// resolveValue is what a value means: @me your login, @current/@next/@previous
// a sprint's slug (one no label has, when there's no such sprint), else
// itself.
func resolveValue(value string, s *Schema, me string, today time.Time) string {
	if value == "@me" {
		return me
	}
	if sprintAliases[value] {
		sprint := s.Sprints.Resolve(value[1:], today)
		if sprint == nil {
			return "\x00no-such-sprint"
		}
		return sprint.Slug
	}
	return value
}

// RequiresMe reports whether any condition uses the @me alias, so callers
// can require --me/GIT_BOARD_ME before evaluating the filter.
func RequiresMe(conditions []Condition) bool {
	for _, c := range conditions {
		if c.Kind == QField || c.Kind == QAuthor || c.Kind == QInvolves || c.Kind == QFollows {
			for _, v := range strings.Split(c.Value, ",") {
				if v == "@me" {
					return true
				}
			}
		}
	}
	return false
}

// Matches reports whether issue satisfies every condition.
func Matches(issue Issue, conditions []Condition, s *Schema, me string, today time.Time) bool {
	labels := issue.AllLabels()
	in := TextsIn(conditions)

	for _, c := range conditions {
		if c.Kind == QIn { // where free text looks, not a condition
			continue
		}
		ok := false
		// a,b: either value matches (no:, #n, free text, dates and counts take no list)
		if noList[c.Kind] {
			ok = matchOne(issue, labels, c, c.Value, in, s, me, today)
		} else {
			for v := range strings.SplitSeq(c.Value, ",") {
				ok = ok || matchOne(issue, labels, c, v, in, s, me, today)
			}
		}
		if c.Negated {
			ok = !ok
		}
		if !ok {
			return false
		}
	}
	return true
}

// noList are the kinds whose value is one, commas and all.
var noList = map[Qualifier]bool{QText: true, QNo: true, QTitle: true, QNumber: true, QInvolves: true,
	QFollows: true, QDate: true, QComments: true}

// matchOne is one condition with one value; in is where free text looks
// (In).
func matchOne(issue Issue, labels []string, c Condition, value string, in []string, s *Schema, me string, today time.Time) bool {
	if value == "@me" && len(c.Mine) > 0 {
		mine := c.Mine
		c.Mine = nil
		for _, name := range mine {
			if matchOne(issue, labels, c, value, in, s, name, today) {
				return true
			}
		}
		return false
	}
	var ok bool
	switch c.Kind {
	case QIs:
		ok = issue.Status == value
	case QTitle:
		ok = titleGlob(value).MatchString(issue.Title)
	case QText:
		if c.Hits != nil {
			ok = c.Hits[issue.ID]
		} else {
			ok = search.Match(value, texts(issue, in)...)
		}
		ok = ok || itself(issue, value)
	case QDate:
		t, has := dateOf(issue, c.Field)
		r, err := DateRange(value, today)
		ok = has && err == nil && r.Has(t.Unix())
	case QComments:
		r, err := CountRange(value)
		ok = err == nil && r.Has(int64(issue.Comments))
	case QNumber:
		ok = issue.HasForgeNumber(value)
	case QAuthor:
		want := value
		if want == "@me" {
			want = me
		}
		for _, l := range issue.AuthorLogins {
			ok = ok || strings.EqualFold(l, want)
		}
	case QLabel:
		// a flat kind is stored bug and read kind:bug: label:bug finds it
		kind := "\x00"
		if s != nil && s.Fields["kind"].Flat {
			for _, k := range s.Fields["kind"].Options {
				if strings.EqualFold(k, value) {
					kind = "kind:" + k
				}
			}
		}
		for _, l := range labels {
			ok = ok || strings.EqualFold(l, value) || l == kind
		}
	case QNo:
		ok = true
		for _, l := range labels {
			ns, _, hasNS := strings.Cut(l, ":")
			if (value == "label" && !hasNS) || (hasNS && ns == value) {
				ok = false
			}
		}
	case QInvolves:
		id := ""
		if value == "@me" {
			id = c.MeID
		}
		ok = len(Involvement(involvedOf(issue, c), resolveValue(value, s, me, today), id, s)) > 0
	case QFollows: // yours only: follows:someone matches nothing
		f := c.Followed
		if f == nil {
			f = &Followed{}
		}
		ok = value == "@me" && !f.Unfollow[issue.ID] &&
			(f.Follow[issue.ID] || f.InViews[issue.ID] || len(Involvement(involvedOf(issue, c), me, c.MeID, s)) > 0)
	case QField:
		ok = slices.Contains(labels, c.Field+":"+resolveValue(value, s, me, today))
	}
	return ok
}

// Involved is what Involvement reads of an issue: from the list (an
// Issue) or from its thread (core/inbox), so the two can't disagree.
type Involved struct {
	AuthorLogins    []string
	AuthorID        string
	Labels          []string
	CommenterLogins []string
	CommenterIDs    []string
	Mentioned       bool // its description or a comment mentions the login
}

// involvedOf reads the issue's own labels: a role it takes from the project
// isn't involvement (the inbox shows the project's new and closed issues to
// its holder, not their discussions); pm:@me still finds it, as a field.
func involvedOf(issue Issue, c Condition) Involved {
	return Involved{AuthorLogins: issue.AuthorLogins, AuthorID: issue.AuthorID, Labels: issue.Labels,
		CommenterLogins: issue.CommenterLogins, CommenterIDs: issue.CommenterIDs, Mentioned: c.Mentions[issue.ID]}
}

// Followed is your inbox's file as follows:@me needs it: what you
// followed and unfollowed by hand, and the issues of the views you follow.
type Followed struct {
	Follow, Unfollow, InViews map[string]bool
}

// Involvement is how someone is in an issue, by login, or by identity id
// when given (yours: you are your identity with or without a login), as
// reasons: author, a person field's name (assignee, reviewer…),
// commented, mentioned. None: they aren't.
func Involvement(in Involved, login, id string, s *Schema) []string {
	if login == "" && id == "" {
		return nil
	}
	is := func(l string) bool { return login != "" && strings.EqualFold(l, login) }
	var res []string
	if slices.ContainsFunc(in.AuthorLogins, is) || (id != "" && in.AuthorID == id) {
		res = append(res, "author")
	}
	for _, l := range in.Labels {
		if f, ok := PersonField(l, login, s); ok && !slices.Contains(res, f) {
			res = append(res, f)
		}
	}
	if slices.ContainsFunc(in.CommenterLogins, is) || (id != "" && slices.Contains(in.CommenterIDs, id)) {
		res = append(res, "commented")
	}
	if in.Mentioned {
		res = append(res, "mentioned")
	}
	return res
}

// PersonField is the field a label names login in (assignee for
// assignee:guilbep), when that field holds people.
func PersonField(label, login string, s *Schema) (string, bool) {
	if s == nil {
		s = &Schema{}
	}
	f, v, ok := strings.Cut(label, ":")
	if !ok || login == "" || !strings.EqualFold(v, login) || s.TypeOf(f, nil) != TypePerson {
		return "", false
	}
	return f, true
}

// globs is titleGlob's cache: a filter runs over every issue.
var globs sync.Map

// titleGlob is a title: pattern as a regexp, case-insensitive and whole:
// * is any text, / included (fnmatch's, not path.Match's), ? one
// character, [a-z] [!a-z] [^a-z] a class. A [ that closes no class is a
// [, so no pattern is invalid: *[wip* finds "[WIP" titles, where a
// syntax error would have matched nothing.
func titleGlob(glob string) *regexp.Regexp {
	if re, ok := globs.Load(glob); ok {
		return re.(*regexp.Regexp)
	}
	// A * at either end is any text there: the regexp then isn't anchored
	// on that side, rather than backtrack through a .* (a glob ending in
	// * isn't in a class: a class ends in ]).
	core := strings.TrimLeft(glob, "*")
	head := map[bool]string{true: "^", false: ""}[core == glob]
	trimmed := strings.TrimRight(core, "*")
	tail := map[bool]string{true: "$", false: ""}[trimmed == core]
	var b strings.Builder
	b.WriteString(`(?is)` + head)
	rs := []rune(trimmed)
	for i := 0; i < len(rs); i++ {
		switch r := rs[i]; r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		case '[':
			end := i + 1
			if end < len(rs) && (rs[end] == '!' || rs[end] == '^') {
				end++
			}
			if end < len(rs) && rs[end] == ']' { // a ] first is in the class
				end++
			}
			for end < len(rs) && rs[end] != ']' {
				end++
			}
			if end >= len(rs) {
				b.WriteString(regexp.QuoteMeta("["))
				continue
			}
			class := rs[i+1 : end]
			b.WriteString("[")
			if class[0] == '!' || class[0] == '^' {
				b.WriteString("^")
				class = class[1:]
			}
			for _, c := range class {
				if c == '\\' || c == '[' || c == ']' {
					b.WriteRune('\\')
				}
				b.WriteRune(c)
			}
			b.WriteString("]")
			i = end
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString(tail)
	re, err := regexp.Compile(b.String())
	if err != nil { // a class the regexp refuses ([z-a]): the glob as text
		re = regexp.MustCompile(`(?is)^` + regexp.QuoteMeta(glob) + `$`)
	}
	globs.Store(glob, re)
	return re
}

// Range is [From, Until): a nil bound is open.
type Range struct{ From, Until *int64 }

// Has tells x is in the range.
func (r Range) Has(x int64) bool {
	return (r.From == nil || x >= *r.From) && (r.Until == nil || x < *r.Until)
}

// span reads a range as GitHub's search writes one, over atoms that are
// each [lo, hi): >a, >=a, <a, <=a, a..b (either end * for open), or a
// alone, a's own span.
func span(v string, atom func(string) (lo, hi int64, err error)) (Range, error) {
	end := func(s string, hi bool) (*int64, error) {
		if s == "*" {
			return nil, nil
		}
		lo, h, err := atom(s)
		if hi {
			lo = h
		}
		return &lo, err
	}
	var r Range
	var err error
	switch {
	case strings.HasPrefix(v, ">="):
		r.From, err = end(v[2:], false)
	case strings.HasPrefix(v, "<="):
		r.Until, err = end(v[2:], true)
	case strings.HasPrefix(v, ">"):
		r.From, err = end(v[1:], true)
	case strings.HasPrefix(v, "<"):
		r.Until, err = end(v[1:], false)
	case strings.Contains(v, ".."):
		a, b, _ := strings.Cut(v, "..")
		if r.From, err = end(a, false); err == nil {
			r.Until, err = end(b, true)
		}
	default:
		var lo, hi int64
		lo, hi, err = atom(v)
		r = Range{&lo, &hi}
	}
	return r, err
}

// DateRange is a created:, updated: or closed: value as a range of Unix
// seconds. A date (2026-10-01) is that day, in today's time zone; a time
// (2026-10-01T14:30, with :05 seconds or a zone, RFC 3339) that minute or
// second; @today that day, @today-7d (d, w, m for months, y) that many
// days, weeks, months or years before it, or + after.
func DateRange(v string, today time.Time) (Range, error) {
	r, err := span(v, func(s string) (int64, int64, error) { return dateAtom(s, today) })
	if err != nil {
		return r, fmt.Errorf("%q: a date (2026-10-01), a time (2026-10-01T14:30) or @today-7d, after >, >=, <, <= or around .. (%w)", v, err)
	}
	return r, nil
}

var relative = regexp.MustCompile(`^([+-])(\d+)([dwmy])$`)

func dateAtom(s string, today time.Time) (int64, int64, error) {
	loc := today.Location()
	if rest, ok := strings.CutPrefix(s, "@today"); ok {
		y, m, d := today.Date()
		day := time.Date(y, m, d, 0, 0, 0, 0, loc)
		if rest != "" {
			p := relative.FindStringSubmatch(rest)
			if p == nil {
				return 0, 0, fmt.Errorf("not a while: %q", rest)
			}
			n, err := strconv.Atoi(p[2])
			if err != nil {
				return 0, 0, err
			}
			if p[1] == "-" {
				n = -n
			}
			switch p[3] {
			case "d":
				day = day.AddDate(0, 0, n)
			case "w":
				day = day.AddDate(0, 0, 7*n)
			case "m":
				day = day.AddDate(0, n, 0)
			case "y":
				day = day.AddDate(n, 0, 0)
			}
		}
		return day.Unix(), day.AddDate(0, 0, 1).Unix(), nil
	}
	if t, err := time.ParseInLocation(time.DateOnly, s, loc); err == nil {
		return t.Unix(), t.AddDate(0, 0, 1).Unix(), nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", s, loc); err == nil {
		return t.Unix(), t.Unix() + 60, nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", s, loc); err == nil {
		return t.Unix(), t.Unix() + 1, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Unix(), t.Unix() + 1, nil
	}
	return 0, 0, fmt.Errorf("not a date: %q", s)
}

// CountRange is a comments: value as a range: 5, >5, >=5, <5, <=5, 2..5,
// 5..*.
func CountRange(v string) (Range, error) {
	r, err := span(v, func(s string) (int64, int64, error) {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n < 0 {
			return 0, 0, fmt.Errorf("not a count: %q", s)
		}
		return n, n + 1, nil
	})
	if err != nil {
		return r, fmt.Errorf("%q: a count (5), after >, >=, <, <= or around .. (2..5) (%w)", v, err)
	}
	return r, nil
}

// dateOf is the time a date qualifier reads, and false when the issue has
// none (closed: of an open issue).
func dateOf(issue Issue, field string) (time.Time, bool) {
	switch field {
	case "created":
		return issue.CreatedAt, true
	case "updated":
		return issue.UpdatedAt, true
	}
	return issue.ClosedAt, !issue.ClosedAt.IsZero()
}

// TextsIn is where free text looks, from the in: conditions (search.Texts:
// title, body, comments): nil for everywhere. Several in: add up; -in:x
// is everywhere but x.
func TextsIn(conditions []Condition) []string {
	var in []string
	seen := false
	for _, c := range conditions {
		if c.Kind != QIn {
			continue
		}
		seen = true
		names := strings.Split(c.Value, ",")
		for _, t := range search.Texts {
			if slices.Contains(names, t) != c.Negated && !slices.Contains(in, t) {
				in = append(in, t)
			}
		}
	}
	if !seen {
		return nil
	}
	if in == nil {
		return []string{} // nowhere: free text matches nothing
	}
	slices.SortFunc(in, func(a, b string) int { return slices.Index(search.Texts, a) - slices.Index(search.Texts, b) })
	return in
}

// texts are an issue's texts in names, as search.Match takes them.
func texts(issue Issue, in []string) []string {
	if in == nil {
		in = search.Texts
	}
	res := make([]string, 0, len(in))
	for _, t := range in {
		switch t {
		case "title":
			res = append(res, issue.Title)
		case "body":
			res = append(res, issue.Body)
		case "comments":
			res = append(res, strings.Join(issue.CommentBodies, "\n"))
		}
	}
	return res
}

// CheckFilter tells what in conditions can't be answered as written: a date, a
// count or an in: that isn't one. Matches finds nothing for them.
func CheckFilter(conditions []Condition, today time.Time) error {
	for _, c := range conditions {
		var err error
		switch c.Kind {
		case QDate:
			_, err = DateRange(c.Value, today)
		case QComments:
			_, err = CountRange(c.Value)
		case QIn:
			for _, v := range strings.Split(c.Value, ",") {
				if !slices.Contains(search.Texts, v) {
					err = fmt.Errorf("%q: title, body or comments", v)
				}
			}
		}
		if err != nil {
			return fmt.Errorf("%s:%w", c.Field, err)
		}
	}
	return nil
}
