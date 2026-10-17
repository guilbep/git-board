// Package filter implements the git-board filter grammar, GitHub's search
// syntax where they meet: field:value, -field:value,
// field:@current|@next|@previous|@me, is:open|closed, title:*glob*,
// author:login, involves:login|@me (the author, a person field, a comment, a
// mention), follows:@me (what your inbox follows), label:name, no:field
// (no:label: no plain label), #2528 (an issue's number on a forge), CO2-2528
// (its project number), free text and "quoted phrases". Conditions
// are AND'd; a,b is either value (status:ready,qa), as in GitHub's search.
// Pure logic, and the only parser of the grammar: the CLI, the TUI, the
// web API and so the web page all go through it.
package filter

import (
	"path"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
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
)

// Condition is one AND'd term of a parsed filter expression.
type Condition struct {
	Kind    Kind
	Field   string // set for KindField
	Value   string
	Negated bool
	// Hits are, for free text, the issues the full-text index (title,
	// description, comments) matches, by id; nil: the title only.
	Hits map[string]bool
	// Mentions are, for involves: and follows:, the issues whose text
	// mentions the login, by id (board fills it); nil: none.
	Mentions map[string]bool
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
		case fieldName == "no":
			conditions = append(conditions, Condition{Kind: KindNo, Value: value, Negated: negated})
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
func itself(issue gitbugio.Issue, word string) bool {
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
func Resolve(value string, cfg *config.Config, me string, today time.Time) string {
	if value == "@me" {
		return me
	}
	if sprintAliases[value] {
		sprint, err := config.ResolveSprint(cfg.Sprints, value[1:], today)
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
		if c.Kind == KindField || c.Kind == KindAuthor || c.Kind == KindInvolves {
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
func Matches(issue gitbugio.Issue, conditions []Condition, cfg *config.Config, me string, today time.Time) bool {
	labels := map[string]bool{}
	for _, l := range issue.AllLabels() {
		labels[l] = true
	}

	for _, c := range conditions {
		ok := false
		// a,b: either value matches (no:, #n and free text take no list)
		values := []string{c.Value}
		if c.Kind != KindText && c.Kind != KindNo && c.Kind != KindTitle && c.Kind != KindNumber && c.Kind != KindInvolves {
			values = strings.Split(c.Value, ",")
		}
		for _, v := range values {
			ok = ok || matchOne(issue, labels, c, v, cfg, me, today)
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

// matchOne is one condition with one value.
func matchOne(issue gitbugio.Issue, labels map[string]bool, c Condition, value string, cfg *config.Config, me string, today time.Time) bool {
	var ok bool
	switch c.Kind {
	case KindIs:
		ok = issue.Status == value
	case KindTitle:
		// ponytail: path.Match's "*" doesn't cross "/" like Python's
		// fnmatch does; titles containing "/" may not glob-match as
		// expected. Switch to a manual glob if that bites.
		ok, _ = path.Match(strings.ToLower(value), strings.ToLower(issue.Title))
	case KindText:
		if c.Hits != nil {
			ok = c.Hits[issue.ID]
		} else {
			ok = strings.Contains(strings.ToLower(issue.Title), strings.ToLower(value))
		}
		ok = ok || itself(issue, value)
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
		for l := range labels {
			ok = ok || strings.EqualFold(l, value)
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
		in := Involved{AuthorLogins: issue.AuthorLogins, Labels: issue.Labels, CommenterLogins: issue.CommenterLogins, Mentioned: c.Mentions[issue.ID]}
		ok = len(Involvement(in, Resolve(value, cfg, me, today), cfg)) > 0
	case KindField:
		ok = labels[c.Field+":"+Resolve(value, cfg, me, today)]
	}
	return ok
}

// Involved is what Involvement reads of an issue: from the list (an
// Issue) or from its thread (internal/inbox), so the two can't disagree.
type Involved struct {
	AuthorLogins    []string
	Labels          []string
	CommenterLogins []string
	Mentioned       bool // its description or a comment mentions the login
}

// Involvement is how login is in an issue, as reasons: author, a person
// field's name (assignee, reviewer…), commented, mentioned. None: it isn't.
func Involvement(in Involved, login string, cfg *config.Config) []string {
	if login == "" {
		return nil
	}
	is := func(l string) bool { return strings.EqualFold(l, login) }
	var res []string
	if slices.ContainsFunc(in.AuthorLogins, is) {
		res = append(res, "author")
	}
	for _, l := range in.Labels {
		if f, ok := PersonField(l, login, cfg); ok && !slices.Contains(res, f) {
			res = append(res, f)
		}
	}
	if slices.ContainsFunc(in.CommenterLogins, is) {
		res = append(res, "commented")
	}
	if in.Mentioned {
		res = append(res, "mentioned")
	}
	return res
}

// PersonField is the field a label names login in (assignee for
// assignee:guilbep), when that field holds people.
func PersonField(label, login string, cfg *config.Config) (string, bool) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	f, v, ok := strings.Cut(label, ":")
	if !ok || login == "" || !strings.EqualFold(v, login) || cfg.TypeOf(f, nil) != config.TypePerson {
		return "", false
	}
	return f, true
}
