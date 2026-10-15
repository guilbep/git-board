// Package filter implements the git-board filter grammar, GitHub's search
// syntax where they meet: field:value, -field:value,
// field:@current|@next|@previous|@me, is:open|closed, title:*glob*,
// author:login, label:name, no:field (no:label: no plain label), free text
// and "quoted phrases". Conditions are AND'd. Pure logic.
package filter

import (
	"path"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// Kind identifies what a Condition matches against.
type Kind string

const (
	KindField  Kind = "field"
	KindIs     Kind = "is"
	KindTitle  Kind = "title"
	KindText   Kind = "text"
	KindAuthor Kind = "author"
	KindLabel  Kind = "label"
	KindNo     Kind = "no"
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
}

// Parse splits a filter string into whitespace-separated AND'd conditions.
func Parse(text string) []Condition {
	var conditions []Condition
	for _, token := range tokens(text) {
		negated := strings.HasPrefix(token, "-")
		body := token
		if negated {
			body = token[1:]
		}
		fieldName, value, hasSep := cut(body, ":")
		switch {
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
		case fieldName == "no":
			conditions = append(conditions, Condition{Kind: KindNo, Value: value, Negated: negated})
		default:
			conditions = append(conditions, Condition{Kind: KindField, Field: fieldName, Value: value, Negated: negated})
		}
	}
	return conditions
}

// tokens splits a filter at spaces, but not inside "quotes": a quoted
// phrase, or a quoted value (label:"nice to have"), is one token, unquoted.
func tokens(text string) []string {
	var res []string
	var cur strings.Builder
	quoted, any := false, false
	for _, r := range text {
		switch {
		case r == '"':
			quoted, any = !quoted, true
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

func cut(s, sep string) (before, after string, found bool) {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

var sprintAliases = map[string]bool{"@current": true, "@next": true, "@previous": true}

func resolveValue(value string, cfg *config.Config, me string, today time.Time) string {
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
		if (c.Kind == KindField || c.Kind == KindAuthor) && c.Value == "@me" {
			return true
		}
	}
	return false
}

// Matches reports whether issue satisfies every condition.
func Matches(issue gitbugio.Issue, conditions []Condition, cfg *config.Config, me string, today time.Time) bool {
	labels := make(map[string]bool, len(issue.Labels))
	for _, l := range issue.Labels {
		labels[l] = true
	}

	for _, c := range conditions {
		var ok bool
		switch c.Kind {
		case KindIs:
			ok = issue.Status == c.Value
		case KindTitle:
			// ponytail: path.Match's "*" doesn't cross "/" like Python's
			// fnmatch does; titles containing "/" may not glob-match as
			// expected. Switch to a manual glob if that bites.
			ok, _ = path.Match(strings.ToLower(c.Value), strings.ToLower(issue.Title))
		case KindText:
			if c.Hits != nil {
				ok = c.Hits[issue.ID]
			} else {
				ok = strings.Contains(strings.ToLower(issue.Title), strings.ToLower(c.Value))
			}
		case KindAuthor:
			want := c.Value
			if want == "@me" {
				want = me
			}
			for _, l := range issue.AuthorLogins {
				ok = ok || strings.EqualFold(l, want)
			}
		case KindLabel:
			for _, l := range issue.Labels {
				ok = ok || strings.EqualFold(l, c.Value)
			}
		case KindNo:
			ok = true
			for _, l := range issue.Labels {
				ns, _, hasNS := strings.Cut(l, ":")
				if (c.Value == "label" && !hasNS) || (hasNS && ns == c.Value) {
					ok = false
				}
			}
		case KindField:
			value := resolveValue(c.Value, cfg, me, today)
			ok = labels[c.Field+":"+value]
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
