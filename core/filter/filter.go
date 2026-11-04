// Package filter implements the git-board filter grammar, GitHub's search
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
package filter

import (
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/core/fields"
	"github.com/guilbep/git-board/core/index"
	"github.com/guilbep/git-board/core/model"
)

// Kind identifies what a Condition matches against.
type Kind string

const (
	KindField    Kind = "field"
	KindIs       Kind = "is"
	KindTitle    Kind = "title"
	KindText     Kind = "text"
	KindAuthor   Kind = "author"
	KindLabel    Kind = "label"
	KindNo       Kind = "no"
	KindNumber   Kind = "number" // #2528
	KindInvolves Kind = "involves"
	KindFollows  Kind = "follows"
	KindDate     Kind = "date"     // created:, updated:, closed: (Field)
	KindComments Kind = "comments" // comments:>5
	KindIn       Kind = "in"       // in:title,body,comments: where free text looks
)

// Condition is one AND'd term of a parsed filter expression.
type Condition struct {
	Kind    Kind
	Field   string // set for KindField
	Value   string
	Negated bool
	// Hits are, for free text, the issues the full-text index (title,
	// description, comments, or where in: says) matches, by id; nil: the
	// issue's own texts are searched (index.MatchText), as the index would.
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

// Parse splits a filter string into whitespace-separated AND'd conditions.
func Parse(text string) []Condition {
	var conditions []Condition
	for _, term := range terms(text) {
		token := strings.ReplaceAll(term, `"`, "")
		negated := strings.HasPrefix(token, "-")
		body := token
		if negated {
			body = token[1:]
		}
		fieldName, value, hasSep := cut(body, ":")
		switch {
		case !hasSep && len(body) > 1 && body[0] == '#' && digits(body[1:]):
			conditions = append(conditions, Condition{Kind: KindNumber, Value: body[1:], Negated: negated})
		case !hasSep:
			conditions = append(conditions, Condition{Kind: KindText, Value: body, Negated: negated})
		case fieldName == "is":
			conditions = append(conditions, Condition{Kind: KindIs, Value: value, Negated: negated})
		case fieldName == "title":
			conditions = append(conditions, Condition{Kind: KindTitle, Value: value, Negated: negated})
		case fieldName == "author":
			conditions = append(conditions, Condition{Kind: KindAuthor, Value: value, Negated: negated})
		case fieldName == "label":
			conditions = append(conditions, Condition{Kind: KindLabel, Value: value, Negated: negated})
		case fieldName == "involves":
			conditions = append(conditions, Condition{Kind: KindInvolves, Value: value, Negated: negated})
		case fieldName == "follows":
			conditions = append(conditions, Condition{Kind: KindFollows, Value: value, Negated: negated})
		case fieldName == "no":
			conditions = append(conditions, Condition{Kind: KindNo, Value: value, Negated: negated})
		case fieldName == "created" || fieldName == "updated" || fieldName == "closed":
			conditions = append(conditions, Condition{Kind: KindDate, Field: fieldName, Value: value, Negated: negated})
		case fieldName == "comments":
			conditions = append(conditions, Condition{Kind: KindComments, Field: fieldName, Value: value, Negated: negated})
		case fieldName == "in":
			conditions = append(conditions, Condition{Kind: KindIn, Field: fieldName, Value: value, Negated: negated})
		default:
			conditions = append(conditions, Condition{Kind: KindField, Field: fieldName, Value: value, Negated: negated})
		}
	}
	return conditions
}

// terms splits a filter at spaces, but not inside "quotes": a quoted
// phrase, or a quoted value (label:"nice to have"), is one term, as typed.
func terms(text string) []string {
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
	for _, term := range terms(text) {
		if c := Parse(term); len(c) == 1 && drop(c[0]) {
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
func itself(issue model.Issue, word string) bool {
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

// Resolve is what a value means: @me your login, @current/@next/@previous
// a sprint's slug (one no label has, when there's no such sprint), else
// itself.
func Resolve(value string, s *fields.Schema, me string, today time.Time) string {
	if value == "@me" {
		return me
	}
	if sprintAliases[value] {
		sprint, err := s.Sprints.Resolve(value[1:], today)
		if err != nil || sprint == nil {
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
		if c.Kind == KindField || c.Kind == KindAuthor || c.Kind == KindInvolves || c.Kind == KindFollows {
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
func Matches(issue model.Issue, conditions []Condition, s *fields.Schema, me string, today time.Time) bool {
	labels := map[string]bool{}
	for _, l := range issue.AllLabels() {
		labels[l] = true
	}
	in := In(conditions)

	for _, c := range conditions {
		if c.Kind == KindIn { // where free text looks, not a condition
			continue
		}
		ok := false
		// a,b: either value matches (no:, #n, free text, dates and counts take no list)
		values := []string{c.Value}
		if !noList[c.Kind] {
			values = strings.Split(c.Value, ",")
		}
		for _, v := range values {
			ok = ok || matchOne(issue, labels, c, v, in, s, me, today)
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
var noList = map[Kind]bool{KindText: true, KindNo: true, KindTitle: true, KindNumber: true, KindInvolves: true,
	KindFollows: true, KindDate: true, KindComments: true}

// matchOne is one condition with one value; in is where free text looks
// (In).
func matchOne(issue model.Issue, labels map[string]bool, c Condition, value string, in []string, s *fields.Schema, me string, today time.Time) bool {
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
	case KindIs:
		ok = issue.Status == value
	case KindTitle:
		ok = titleGlob(value).MatchString(issue.Title)
	case KindText:
		if c.Hits != nil {
			ok = c.Hits[issue.ID]
		} else {
			ok = index.MatchText(value, texts(issue, in)...)
		}
		ok = ok || itself(issue, value)
	case KindDate:
		t, has := dateOf(issue, c.Field)
		r, err := DateRange(value, today)
		ok = has && err == nil && r.Has(t.Unix())
	case KindComments:
		r, err := CountRange(value)
		ok = err == nil && r.Has(int64(issue.Comments))
	case KindNumber:
		ok = issue.HasForgeNumber(value)
	case KindAuthor:
		want := value
		if want == "@me" {
			want = me
		}
		for _, l := range issue.AuthorLogins {
			ok = ok || strings.EqualFold(l, want)
		}
	case KindLabel:
		// a flat kind is stored bug and read kind:bug: label:bug finds it
		kind := "\x00"
		if s != nil && s.Fields["kind"].Flat {
			for _, k := range s.Fields["kind"].Options {
				if strings.EqualFold(k, value) {
					kind = "kind:" + k
				}
			}
		}
		for l := range labels {
			ok = ok || strings.EqualFold(l, value) || l == kind
		}
	case KindNo:
		ok = true
		for l := range labels {
			ns, _, hasNS := strings.Cut(l, ":")
			if (value == "label" && !hasNS) || (hasNS && ns == value) {
				ok = false
			}
		}
	case KindInvolves:
		id := ""
		if value == "@me" {
			id = c.MeID
		}
		ok = len(Involvement(involvedOf(issue, c), Resolve(value, s, me, today), id, s)) > 0
	case KindFollows: // yours only: follows:someone matches nothing
		f := c.Followed
		if f == nil {
			f = &Followed{}
		}
		ok = value == "@me" && !f.Unfollow[issue.ID] &&
			(f.Follow[issue.ID] || f.InViews[issue.ID] || len(Involvement(involvedOf(issue, c), me, c.MeID, s)) > 0)
	case KindField:
		ok = labels[c.Field+":"+Resolve(value, s, me, today)]
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
func involvedOf(issue model.Issue, c Condition) Involved {
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
func Involvement(in Involved, login, id string, s *fields.Schema) []string {
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
func PersonField(label, login string, s *fields.Schema) (string, bool) {
	if s == nil {
		s = &fields.Schema{}
	}
	f, v, ok := strings.Cut(label, ":")
	if !ok || login == "" || !strings.EqualFold(v, login) || s.TypeOf(f, nil) != fields.TypePerson {
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
	var b strings.Builder
	b.WriteString(`(?is)^`)
	rs := []rune(glob)
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
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil { // a class the regexp refuses ([z-a]): the glob as text
		re = regexp.MustCompile(`(?is)^` + regexp.QuoteMeta(glob) + `$`)
	}
	globs.Store(glob, re)
	return re
}
