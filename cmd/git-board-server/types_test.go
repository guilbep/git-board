package main

import (
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// The page gets each field's type, guessed or configured, and dates
// colored against today.
func TestFieldsSayTheirType(t *testing.T) {
	ts, s := newIssueServer(t)
	s.cfg.Fields = map[string]config.Field{"est": {Single: true, Numeric: true}, "status": {Options: []string{"todo", "done"}}}
	day := func(d int) string { return time.Now().AddDate(0, 0, d).Format("2006-01-02") }
	a := gitbugiotest.NewIssue(t, s.b, "late")
	b := gitbugiotest.NewIssue(t, s.b, "soon")
	c := gitbugiotest.NewIssue(t, s.b, "later")
	for id, labels := range map[string][]string{
		a: {"due:" + day(-2), "parent:#3", "assignee:guilbep", "cat:feat"},
		b: {"due:" + day(3), "blocked-by:#4"},
		c: {"due:" + day(40)},
	} {
		if _, err := s.b.ApplyLabelChange(id, labels, nil); err != nil {
			t.Fatal(err)
		}
	}

	var fields map[string]struct {
		Type, Unit string
		Single     bool
	}
	call(t, "GET", ts.URL+"/api/config/fields", nil, &fields)
	for name, want := range map[string]struct {
		typ, unit string
		single    bool
	}{
		"est":        {config.TypeNumber, "h", true},
		"status":     {config.TypeSelect, "", false},
		"due":        {config.TypeDate, "", true},
		"parent":     {config.TypeIssue, "", true},
		"blocked-by": {config.TypeIssue, "", false},
		"assignee":   {config.TypePerson, "", false},
	} {
		got := fields[name]
		if got.Type != want.typ || got.Unit != want.unit || got.Single != want.single {
			t.Errorf("%s: %+v, want %+v", name, got, want)
		}
	}
	if _, listed := fields["cat"]; listed {
		t.Error("an undeclared select is listed: the page guesses those itself")
	}

	var colors map[string]map[string]string
	call(t, "GET", ts.URL+"/api/colors", nil, &colors)
	if colors["due"][day(-2)] != "red" || colors["due"][day(3)] != "amber" || colors["due"][day(40)] != "" {
		t.Errorf("due colors %v", colors["due"])
	}
}

// An issue field links issues both ways: what an issue points to, and
// what points to it (its sub-issues, what it blocks).
func TestIssueRefsBothWays(t *testing.T) {
	ts, s := newIssueServer(t)
	epic := gitbugiotest.NewImportedIssue(t, s.b, "Hosted git-board", "https://github.com/guilbep/git-board/issues/12")
	child := gitbugiotest.NewIssue(t, s.b, "OIDC login")
	other := gitbugiotest.NewIssue(t, s.b, "Webhooks")
	if _, err := s.b.ApplyLabelChange(child, []string{"parent:#12"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.b.ApplyLabelChange(other, []string{"parent:" + epic[:8], "blocked-by:" + child[:7]}, nil); err != nil {
		t.Fatal(err)
	}

	type ref struct {
		ID, Title, Value string
	}
	var refs struct {
		Out map[string][]ref `json:"out"`
		In  map[string][]ref `json:"in"`
	}
	call(t, "GET", ts.URL+"/api/issues/"+epic+"/refs", nil, &refs)
	if len(refs.In["parent"]) != 2 || len(refs.Out) != 0 {
		t.Errorf("the epic: %+v", refs)
	}
	call(t, "GET", ts.URL+"/api/issues/"+other+"/refs", nil, &refs)
	if p := refs.Out["parent"]; len(p) != 1 || p[0].ID != epic || p[0].Title != "Hosted git-board" {
		t.Errorf("other's parent: %+v", refs.Out)
	}
	if b := refs.Out["blocked-by"]; len(b) != 1 || b[0].ID != child || b[0].Value != child[:7] {
		t.Errorf("other's blocked-by: %+v", refs.Out)
	}
	call(t, "GET", ts.URL+"/api/issues/"+child+"/refs", nil, &refs)
	if b := refs.In["blocked-by"]; len(b) != 1 || b[0].ID != other {
		t.Errorf("what child blocks: %+v", refs.In)
	}
	if p := refs.Out["parent"]; len(p) != 1 || p[0].ID != epic {
		t.Errorf("child's parent by #12: %+v", refs.Out)
	}
}
