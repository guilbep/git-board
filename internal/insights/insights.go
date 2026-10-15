// Package insights replays every issue's label and status changes, which
// git-bug keeps with their time, into what a project manager asks: how
// the work flowed each day (a cumulative flow), how many issues close a
// week, how long an issue takes once started, how the current sprint is
// burning up, and the board as it was on any day. Pure logic: the server
// feeds it the history (gitbugio.History).
package insights

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// None is the band of issues without a value for the field.
const None = "(none)"

// Change is one operation: labels added and removed, or the status set.
type Change struct {
	At      time.Time
	Added   []string
	Removed []string
	Status  string // "open" or "closed" when the change sets it
}

// Issue is an issue's history, its changes in time order.
type Issue struct {
	ID      string
	Created time.Time
	Changes []Change
}

// Sprint is the iteration a burn-up follows: issues whose Field is Slug.
type Sprint struct {
	Field, Slug, Title string
	Start, End         time.Time
}

// Options say what to compute.
type Options struct {
	Field    string   // the field the flow is drawn for: status
	Workflow []string // its values in order (the field's options)
	From, To time.Time
	Sprint   *Sprint // nil: no burn-up
}

// Report is everything the Insights page draws.
type Report struct {
	Field     string            `json:"field"`
	Values    []string          `json:"values"` // bands, in workflow order, None last
	Days      []string          `json:"days"`   // YYYY-MM-DD
	All       map[string][]int  `json:"all"`    // per value and day: every issue then (the flow)
	Open      map[string][]int  `json:"open"`   // the open ones (the board that day)
	Weeks     []Week            `json:"weeks"`
	Cycle     Cycle             `json:"cycle"`
	Burnup    *Burnup           `json:"burnup,omitempty"`
	Headlines map[string]string `json:"headlines"`
}

// Week counts the issues opened and closed in the week from Start.
type Week struct {
	Start  string `json:"start"`
	Opened int    `json:"opened"`
	Closed int    `json:"closed"`
}

// Cycle is how long issues closed in the period took from Start (the
// first value counted as started) to closing, in days.
type Cycle struct {
	From   string    `json:"from"` // "started" (Start's value), or "opened" when no issue went through a started value
	Start  string    `json:"start"`
	Days   []float64 `json:"days"`
	Median float64   `json:"median"`
	P85    float64   `json:"p85"`
}

// Burnup follows the sprint: per day, its issues and those closed.
type Burnup struct {
	Sprint string   `json:"sprint"`
	Title  string   `json:"title"`
	Start  string   `json:"start"`
	End    string   `json:"end"`
	Days   []string `json:"days"`
	Scope  []int    `json:"scope"`
	Done   []int    `json:"done"`
	// Status is where the sprint's issues stand on its last day (or today):
	// how many have each value of the report's field, None for none.
	Status map[string]int `json:"status"`
}

// state is an issue at a moment: its labels and whether it is open.
type state struct {
	labels map[string]bool
	open   bool
}

func (is Issue) at(t time.Time) (state, bool) {
	if is.Created.After(t) {
		return state{}, false
	}
	s := state{labels: map[string]bool{}, open: true}
	for _, c := range is.Changes {
		if c.At.After(t) {
			break
		}
		for _, l := range c.Removed {
			delete(s.labels, l)
		}
		for _, l := range c.Added {
			s.labels[l] = true
		}
		switch c.Status {
		case "open":
			s.open = true
		case "closed":
			s.open = false
		}
	}
	return s, true
}

// value is the field's value in a state: the first in sorted order when
// several, None when none.
func (s state) value(field string) string {
	var vs []string
	for l := range s.labels {
		if v, ok := strings.CutPrefix(l, field+":"); ok {
			vs = append(vs, v)
		}
	}
	if len(vs) == 0 {
		return None
	}
	sort.Strings(vs)
	return vs[0]
}

const dayFmt = "2006-01-02"

// Compute replays the issues over [From, To], day by day.
func Compute(issues []Issue, o Options) Report {
	r := Report{Field: o.Field, All: map[string][]int{}, Open: map[string][]int{}, Headlines: map[string]string{}}
	from := truncDay(o.From)
	var ends []time.Time // the end of each day
	for d := from; !d.After(o.To); d = d.AddDate(0, 0, 1) {
		r.Days = append(r.Days, d.Format(dayFmt))
		ends = append(ends, d.AddDate(0, 0, 1).Add(-time.Nanosecond))
	}
	seen := map[string]bool{}
	for i, end := range ends {
		for _, is := range issues {
			s, ok := is.at(end)
			if !ok {
				continue
			}
			v := s.value(o.Field)
			if r.All[v] == nil {
				r.All[v], r.Open[v] = make([]int, len(ends)), make([]int, len(ends))
			}
			seen[v] = true
			r.All[v][i]++
			if s.open {
				r.Open[v][i]++
			}
		}
	}
	r.Values = order(seen, o.Workflow)
	r.Weeks = weeks(issues, from, o.To)
	r.Cycle = cycle(issues, o)
	if o.Sprint != nil {
		r.Burnup = burnup(issues, *o.Sprint, o.Field, o.To)
	}
	r.headlines()
	return r
}

func truncDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// order puts the values seen in workflow order, the others after it
// (sorted), None last.
func order(seen map[string]bool, workflow []string) []string {
	var res, rest []string
	for _, v := range workflow {
		if seen[v] {
			res = append(res, v)
		}
	}
	for v := range seen {
		if v != None && !slices.Contains(workflow, v) {
			rest = append(rest, v)
		}
	}
	sort.Strings(rest)
	res = append(res, rest...)
	if seen[None] {
		res = append(res, None)
	}
	return res
}

// weeks counts openings and closings per week, weeks starting Monday.
func weeks(issues []Issue, from, to time.Time) []Week {
	start := from.AddDate(0, 0, -((int(from.Weekday()) + 6) % 7))
	var res []Week
	for w := start; !w.After(to); w = w.AddDate(0, 0, 7) {
		res = append(res, Week{Start: w.Format(dayFmt)})
	}
	idx := func(t time.Time) int {
		if t.Before(start) || t.After(to) {
			return -1
		}
		return int(t.Sub(start).Hours() / 24 / 7)
	}
	for _, is := range issues {
		if i := idx(is.Created); i >= 0 && i < len(res) {
			res[i].Opened++
		}
		for _, c := range is.Changes {
			if i := idx(c.At); c.Status == "closed" && i >= 0 && i < len(res) {
				res[i].Closed++
			}
		}
	}
	return res
}

var startedWord = regexp.MustCompile(`progress|doing|wip|started|dev|active`)

// cycle measures, for issues closed in the period, the days from their
// start to the last closing. Started: the first workflow value naming
// work in progress, and every value after it, reached from a value before
// it. When no issue did that (no such value, or statuses imported in one
// go), the days from opened to closed.
func cycle(issues []Issue, o Options) Cycle {
	if c := cycleFrom(issues, o, true); len(c.Days) > 0 {
		return c
	}
	// statuses imported in one go have no history: opened → closed
	return cycleFrom(issues, o, false)
}

func cycleFrom(issues []Issue, o Options, fromStarted bool) Cycle {
	c := Cycle{Days: []float64{}, From: "started"}
	if !fromStarted {
		c.From = "opened"
	}
	threshold := -1
	for i, v := range o.Workflow {
		if startedWord.MatchString(v) {
			threshold, c.Start = i, v
			break
		}
	}
	started := func(label string) bool {
		v, ok := strings.CutPrefix(label, o.Field+":")
		if !ok {
			return false
		}
		if threshold < 0 {
			return true
		}
		i := slices.Index(o.Workflow, v)
		return i >= threshold
	}
	for _, is := range issues {
		var begin, closed time.Time
		if !fromStarted {
			begin = is.Created
		}
		// a start is a move through the workflow, from a value not started
		// (backlog, ready) to a started one: never from no value, which is
		// how an import or a first labelling looks
		labels := map[string]bool{}
		for _, ch := range is.Changes {
			prev := state{labels: labels}.value(o.Field)
			for _, l := range ch.Removed {
				delete(labels, l)
			}
			for _, l := range ch.Added {
				labels[l] = true
			}
			if begin.IsZero() && prev != None && !started(o.Field+":"+prev) && slices.ContainsFunc(ch.Added, started) {
				begin = ch.At
			}
			switch ch.Status {
			case "closed":
				closed = ch.At
			case "open":
				closed = time.Time{}
			}
		}
		if begin.IsZero() || closed.IsZero() || closed.Before(o.From) || closed.After(o.To) || closed.Before(begin) {
			continue
		}
		c.Days = append(c.Days, closed.Sub(begin).Hours()/24)
	}
	sorted := slices.Clone(c.Days)
	sort.Float64s(sorted)
	c.Median, c.P85 = quantile(sorted, .5), quantile(sorted, .85)
	return c
}

func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[int(math.Ceil(q*float64(len(sorted))))-1]
}

func burnup(issues []Issue, sp Sprint, field string, to time.Time) *Burnup {
	b := &Burnup{Sprint: sp.Slug, Title: sp.Title, Start: sp.Start.Format(dayFmt), End: sp.End.Format(dayFmt), Status: map[string]int{}}
	last := sp.End.Add(-time.Nanosecond)
	if to.Before(last) {
		last = to
	}
	label := sp.Field + ":" + sp.Slug
	for d := truncDay(sp.Start); !d.After(last); d = d.AddDate(0, 0, 1) {
		end := d.AddDate(0, 0, 1).Add(-time.Nanosecond)
		scope, done := 0, 0
		for _, is := range issues {
			if s, ok := is.at(end); ok && s.labels[label] {
				scope++
				if !s.open {
					done++
				}
			}
		}
		b.Days, b.Scope, b.Done = append(b.Days, d.Format(dayFmt)), append(b.Scope, scope), append(b.Done, done)
	}
	for _, is := range issues {
		if s, ok := is.at(last); ok && s.labels[label] {
			b.Status[s.value(field)]++
		}
	}
	return b
}

// headlines say in one sentence what each chart shows.
func (r *Report) headlines() {
	n := len(r.Days)
	if n > 0 {
		open := func(i int) (t int) {
			for _, v := range r.Values {
				t += r.Open[v][i]
			}
			return t
		}
		grew, from, to := "", 0, 0
		for _, v := range r.Values {
			if d := r.Open[v][n-1] - r.Open[v][0]; v != None && d > to-from {
				grew, from, to = v, r.Open[v][0], r.Open[v][n-1]
			}
		}
		h := fmt.Sprintf("%d open issues today, %s %d days ago.", open(n-1), signed(open(n-1)-open(0)), n-1)
		if grew != "" {
			h += fmt.Sprintf(" %s grew the most, from %d to %d.", grew, from, to)
		}
		r.Headlines["flow"] = h
	}
	if w := r.Weeks; len(w) > 0 {
		recent, before := avgClosed(w, 0, 4), avgClosed(w, 4, 8)
		h := fmt.Sprintf("%s issues closed a week over the last 4 weeks", num(recent))
		switch {
		case recent == 0 && before == 0:
			h = "No issue closed in the last 8 weeks."
		case recent == 0:
			h = fmt.Sprintf("No issue closed in the last 4 weeks, against %s a week the 4 before.", num(before))
		case before == 0:
			h += "."
		case recent >= 1.8*before:
			h += fmt.Sprintf(", %s the pace of the 4 before (%s).", times(recent/before), num(before))
		case recent > before:
			h += fmt.Sprintf(", up from %s the 4 weeks before.", num(before))
		case recent < before:
			h += fmt.Sprintf(", down from %s the 4 weeks before.", num(before))
		default:
			h += ", as many as the 4 weeks before."
		}
		r.Headlines["throughput"] = h
	}
	if c := r.Cycle; len(c.Days) > 0 {
		from := "starting"
		if c.From == "opened" {
			from = "being opened"
		}
		r.Headlines["cycle"] = fmt.Sprintf("Half the issues close within %s of %s, 85%% within %s (%d issues).", duration(c.Median), from, duration(c.P85), len(c.Days))
	} else {
		r.Headlines["cycle"] = "No issue went from started to closed in this period."
	}
	if b := r.Burnup; b != nil && len(b.Days) > 0 {
		last := len(b.Days) - 1
		end, _ := time.Parse(dayFmt, b.End)
		today, _ := time.Parse(dayFmt, b.Days[last])
		left := int(end.Sub(today).Hours()/24) - 1
		h := fmt.Sprintf("%s: %d of %d done", b.Title, b.Done[last], b.Scope[last])
		if left > 0 {
			h += fmt.Sprintf(", %d day%s left.", left, plural(left))
		} else {
			h += ", sprint over."
		}
		r.Headlines["burnup"] = h
	}
}

// avgClosed is the mean closings of the weeks [a, b) counted from the last.
func avgClosed(w []Week, a, b int) float64 {
	sum, n := 0, 0
	for i := len(w) - 1 - a; i > len(w)-1-b && i >= 0; i-- {
		sum += w[i].Closed
		n++
	}
	if n == 0 {
		return 0
	}
	return float64(sum) / float64(n)
}

func signed(d int) string {
	switch {
	case d > 0:
		return fmt.Sprintf("%d more than", d)
	case d < 0:
		return fmt.Sprintf("%d fewer than", -d)
	}
	return "as many as"
}

func num(f float64) string {
	if f == math.Trunc(f) {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%.1f", f)
}

func times(f float64) string {
	if f < 2.5 {
		return "twice"
	}
	return fmt.Sprintf("%.0f times", f)
}

func duration(days float64) string {
	if days < 1 {
		h := int(math.Round(days * 24))
		return fmt.Sprintf("%d hour%s", h, plural(h))
	}
	d := int(math.Round(days))
	return fmt.Sprintf("%d day%s", d, plural(d))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
