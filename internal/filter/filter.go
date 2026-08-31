// Package filter implements the git-board filter grammar:
// field:value, -field:value, field:@current|@next|@previous|@me,
// is:open|closed, title:*glob*, free text. Conditions are AND'd. Pure logic.
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
	KindField Kind = "field"
	KindIs    Kind = "is"
	KindTitle Kind = "title"
	KindText  Kind = "text"
)

// Condition is one AND'd term of a parsed filter expression.
type Condition struct {
	Kind    Kind
	Field   string // set for KindField
	Value   string
	Negated bool
}

// Parse splits a filter string into whitespace-separated AND'd conditions.
func Parse(text string) []Condition {
	var conditions []Condition
	for _, token := range strings.Fields(text) {
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
		default:
			conditions = append(conditions, Condition{Kind: KindField, Field: fieldName, Value: value, Negated: negated})
		}
	}
	return conditions
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
		if c.Kind == KindField && c.Value == "@me" {
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
			ok = strings.Contains(strings.ToLower(issue.Title), strings.ToLower(c.Value))
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
