package fields

import (
	"testing"
	"time"
)

func TestKnownValueNumericTakesAnyNumber(t *testing.T) {
	f := Field{Single: true, Numeric: true, Options: []string{"1", "2"}}
	if !f.KnownValue("3.5") || !f.KnownValue("33") {
		t.Error("a number outside the options was refused")
	}
	if f.KnownValue("lots") {
		t.Error("a non-number was taken")
	}
}

// The last sprint ends: past its end there is no current sprint, and the
// previous is the one that ended. Its length without length_days is the
// one before it's, else the default file's 14 days, never none.
func TestTheLastSprintEnds(t *testing.T) {
	t.Parallel()
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	a := Sprint{Slug: "a", Start: day(2020, 1, 6)}
	b := Sprint{Slug: "b", Start: day(2020, 1, 13)}
	for _, tt := range []struct {
		sprints Sprints
		end     time.Time
	}{
		{Sprints{List: []Sprint{a}}, day(2020, 1, 20)},
		{Sprints{List: []Sprint{a}, LengthDays: 10}, day(2020, 1, 16)},
		{Sprints{List: []Sprint{a, b}}, day(2020, 1, 20)},
	} {
		w := tt.sprints.Windows()
		if got := w[len(w)-1].End; !got.Equal(tt.end) {
			t.Errorf("%+v: end %s, want %s", tt.sprints, got.Format("2006-01-02"), tt.end.Format("2006-01-02"))
		}
		last := tt.sprints.List[len(tt.sprints.List)-1].Slug
		if cur, _ := tt.sprints.Resolve("current", tt.end.AddDate(0, 0, -1)); cur == nil || cur.Slug != last {
			t.Errorf("%+v: current on its last day = %+v", tt.sprints, cur)
		}
		if cur, _ := tt.sprints.Resolve("current", day(2026, 11, 4)); cur != nil {
			t.Errorf("%+v: current six years on = %+v, want none", tt.sprints, cur)
		}
		if prev, _ := tt.sprints.Resolve("previous", day(2026, 11, 4)); prev == nil || prev.Slug != last {
			t.Errorf("%+v: previous six years on = %+v, want %s", tt.sprints, prev, last)
		}
	}
}
