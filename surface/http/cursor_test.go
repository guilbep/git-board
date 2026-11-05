package httpapi

import (
	"fmt"
	"net/url"
	"slices"
	"testing"
)

// pages lists q a page of limit at a time, from cursor, calling between
// after each page; it answers the titles, in order.
func pages(t *testing.T, a *api, q string, limit int, between func(page int, last string)) []string {
	t.Helper()
	var titles []string
	next := ""
	for i := 0; ; i++ {
		p := want[issuePage](t, a.do("GET", fmt.Sprintf("/api/issues?%s&limit=%d&cursor=%s", q, limit, url.QueryEscape(next)), nil), 200)
		for _, it := range p.Items {
			titles = append(titles, it.Title)
		}
		if p.Next == "" || i > 20 {
			return titles
		}
		if between != nil {
			between(i, p.Items[len(p.Items)-1].ID)
		}
		next = p.Next
	}
}

// A page's cursor is a place in the order, not an index (review finding
// 2): the issue it names leaving the list, issues leaving before it,
// issues added before it, and the issue moving all skip nothing that was
// after it, and repeat nothing before it.
func TestCursorIsAPlace(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	ids := map[string]string{}
	for i := range 6 {
		title := fmt.Sprintf("n%d", i)
		th := want[threadBody](t, a.do("POST", "/api/issues", map[string]any{"title": title, "labels": []string{"batch"}, "set": []string{fmt.Sprintf("prio=p%d", i%3)}}), 201)
		ids[title] = th.ID
	}
	close := func(title string) {
		want[threadBody](t, a.do("PATCH", "/api/issues/"+ids[title], map[string]any{"status": "closed"}), 200)
	}
	byID := func(id string) string {
		for k, v := range ids {
			if v == id {
				return k
			}
		}
		return ""
	}

	all := func(q string) []string { return pages(t, a, q, 0, nil) }
	// every issue listed once, in the list's order, but those gone
	check := func(what string, got, before []string, gone ...string) {
		t.Helper()
		want := slices.DeleteFunc(slices.Clone(before), func(s string) bool { return slices.Contains(gone, s) })
		got = slices.DeleteFunc(got, func(s string) bool { return slices.Contains(gone, s) })
		if !slices.Equal(got, want) {
			t.Fatalf("%s: %v, want %v", what, got, want)
		}
	}

	// the cursor's own issue closed after the first page
	q := "filter=label:batch+is:open"
	before := all(q)
	var gone []string
	got := pages(t, a, q, 2, func(page int, last string) {
		if page == 0 {
			gone = append(gone, byID(last))
			close(byID(last))
		}
	})
	check("the cursor's issue closed", got, before)
	// two closed up to the cursor, one of them its own
	before, gone = all(q), nil
	got = pages(t, a, q, 2, func(page int, last string) {
		if page == 0 {
			gone = append(gone, before[0], byID(last))
			close(before[0])
			close(byID(last))
		}
	})
	check("issues closed up to the cursor", got, before)
	if len(got) != len(before) {
		t.Fatalf("closed after they were listed, they stay listed: %v, %v", got, before)
	}
	// added before the cursor, by its sort: not listed again, nothing skipped
	q = "filter=label:batch+is:open&sort=title+desc"
	before = all(q)
	got = pages(t, a, q, 1, func(page int, last string) {
		if page == 0 {
			th := want[threadBody](t, a.do("POST", "/api/issues", map[string]any{"title": "n9", "labels": []string{"batch"}}), 201)
			ids["n9"] = th.ID
		}
	})
	check("an issue added before the cursor", got, before)
	// the cursor's issue moves to the front (updated desc): the rest still comes
	q = "filter=label:batch+is:open&sort=updated+desc"
	before = all(q)
	var moved string
	got = pages(t, a, q, 2, func(page int, last string) {
		if page == 0 {
			moved = byID(last)
			delete(ids, moved)
			ids[moved+"x"] = last
			want[threadBody](t, a.do("PATCH", "/api/issues/"+last, map[string]any{"title": moved + "x"}), 200)
		}
	})
	check("the cursor's issue moved", got, before, moved, moved+"x")
	// a field's sort: the cursor's issue changes value
	q = "filter=label:batch+is:open&sort=prio"
	before = all(q)
	got = pages(t, a, q, 1, func(page int, last string) {
		if page == 1 {
			moved = byID(last)
			want[threadBody](t, a.do("PATCH", "/api/issues/"+last, map[string]any{"set": []string{"prio=p2"}}), 200)
		}
	})
	check("by prio, the cursor's issue moved", got, before, moved)
	// a cursor of another sort is refused
	p := want[issuePage](t, a.do("GET", "/api/issues?filter=label:batch&limit=1&sort=title", nil), 200)
	if e := want[apiError](t, a.do("GET", "/api/issues?filter=label:batch&limit=1&sort=updated&cursor="+p.Next, nil), 400); e.Field != "cursor" {
		t.Fatalf("a cursor of another sort = %+v", e)
	}
}
