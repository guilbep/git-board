package filter

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/guilbep/git-board/core/index"
	"github.com/guilbep/git-board/core/model"
)

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
func dateOf(issue model.Issue, field string) (time.Time, bool) {
	switch field {
	case "created":
		return issue.CreatedAt, true
	case "updated":
		return issue.UpdatedAt, true
	}
	return issue.ClosedAt, !issue.ClosedAt.IsZero()
}

// In is where free text looks, from the in: conditions (index.Texts:
// title, body, comments): nil for everywhere. Several in: add up; -in:x
// is everywhere but x.
func In(conditions []Condition) []string {
	var in []string
	seen := false
	for _, c := range conditions {
		if c.Kind != KindIn {
			continue
		}
		seen = true
		names := strings.Split(c.Value, ",")
		for _, t := range index.Texts {
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
	slices.SortFunc(in, func(a, b string) int { return slices.Index(index.Texts, a) - slices.Index(index.Texts, b) })
	return in
}

// texts are an issue's texts in names, as index.MatchText takes them.
func texts(issue model.Issue, in []string) []string {
	if in == nil {
		in = index.Texts
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

// Check tells what in conditions can't be answered as written: a date, a
// count or an in: that isn't one. Matches finds nothing for them.
func Check(conditions []Condition, today time.Time) error {
	for _, c := range conditions {
		var err error
		switch c.Kind {
		case KindDate:
			_, err = DateRange(c.Value, today)
		case KindComments:
			_, err = CountRange(c.Value)
		case KindIn:
			for _, v := range strings.Split(c.Value, ",") {
				if !slices.Contains(index.Texts, v) {
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
