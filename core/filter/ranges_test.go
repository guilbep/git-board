package filter

import (
	"slices"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/model"
)

func TestParseQualifiers(t *testing.T) {
	t.Parallel()
	for text, want := range map[string]Condition{
		"created:>2026-10-01": {Kind: KindDate, Field: "created", Value: ">2026-10-01"},
		"-updated:@today":     {Kind: KindDate, Field: "updated", Value: "@today", Negated: true},
		"closed:a..b":         {Kind: KindDate, Field: "closed", Value: "a..b"},
		"comments:>5":         {Kind: KindComments, Field: "comments", Value: ">5"},
		"in:title,body":       {Kind: KindIn, Field: "in", Value: "title,body"},
	} {
		if c := Parse(text); len(c) != 1 || c[0].Kind != want.Kind || c[0].Field != want.Field || c[0].Value != want.Value || c[0].Negated != want.Negated {
			t.Errorf("Parse(%q) = %+v, want %+v", text, c, want)
		}
	}
}

// A date is a day in today's time zone: 23:30 in Zurich is that day there,
// the day before in UTC.
func TestDateRangeIsTodaysZone(t *testing.T) {
	t.Parallel()
	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Skip(err)
	}
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, zurich)
	late := time.Date(2026, 10, 1, 23, 30, 0, 0, zurich).Unix()
	for v, want := range map[string]bool{
		"2026-10-01": true, "2026-10-02": false, ">2026-09-30": true, "<=2026-10-01": true, "<2026-10-01": false,
		"2026-10-01T23:30": true, "2026-10-01T23:31": false, "2026-10-01T21:30:00Z": true,
		"@today-14d": true, "@today-2w": true, "@today-13d": false, ">=@today-1w": false, ">=@today-1m": true, "<@today-0d": true, "2026-09-01..2026-10-01": true,
		"*..*": true, "@today+1d..*": false,
	} {
		r, err := DateRange(v, now)
		if err != nil {
			t.Errorf("%q: %v", v, err)
			continue
		}
		if r.Has(late) != want {
			t.Errorf("%q has 2026-10-01 23:30 Zurich: %v, want %v", v, !want, want)
		}
	}
	for _, v := range []string{"", ">", "2026-10", "10/01/2026", "@today-7", "@yesterday", "2026-10-01..", "2026-10-01T25:00"} {
		if _, err := DateRange(v, now); err == nil {
			t.Errorf("%q: no error", v)
		}
	}
}

func TestCountRange(t *testing.T) {
	t.Parallel()
	for v, want := range map[string][]int64{
		"0": {0}, "5": {5}, ">5": {6, 7}, ">=5": {5, 6, 7}, "<2": {0, 1}, "<=2": {0, 1, 2}, "2..4": {2, 3, 4}, "3..*": {3, 4, 5, 6, 7}, "*..1": {0, 1},
	} {
		r, err := CountRange(v)
		if err != nil {
			t.Fatalf("%q: %v", v, err)
		}
		var got []int64
		for n := range int64(8) {
			if r.Has(n) {
				got = append(got, n)
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%q: %v, want %v", v, got, want)
		}
	}
	for _, v := range []string{"", "x", "-1", ">=", "1.5", "1..x"} {
		if _, err := CountRange(v); err == nil {
			t.Errorf("%q: no error", v)
		}
	}
}

func TestInNarrowsFreeText(t *testing.T) {
	t.Parallel()
	for text, want := range map[string][]string{
		"crash":                   nil,
		"in:title":                {"title"},
		"in:comments in:title":    {"title", "comments"},
		"in:body,title":           {"title", "body"},
		"-in:title":               {"body", "comments"},
		"-in:title,body,comments": {},
		"in:title -in:title":      {"title", "body", "comments"},
	} {
		if got := In(Parse(text)); !slices.Equal(got, want) || (got == nil) != (want == nil) {
			t.Errorf("In(%q) = %#v, want %#v", text, got, want)
		}
	}
	is := model.Issue{Title: "Docs", Body: "a café", CommentBodies: []string{"crash here", "and there"}}
	for text, want := range map[string]bool{
		"cafe": true, "cafe in:title": false, "cafe in:body": true, "crash-here": true, "here and": true, "there-crash": false,
		"cras in:comments": true, "rash": false, "docs -in:title": false,
	} {
		if got := Matches(is, Parse(text), nil, "", today); got != want {
			t.Errorf("%q: %v, want %v", text, got, want)
		}
	}
}
