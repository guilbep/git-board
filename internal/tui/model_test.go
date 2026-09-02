package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// send drives Model.Update the way bubbletea's runtime would, unwrapping
// the tea.Model interface back to the concrete type these tests need.
func send(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func key(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestUpdateEditFieldWritesThroughAndRerendersList(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "widget")
	if _, err := b.ApplyLabelChange(id, []string{"status:backlog"}, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	m := New(issues, "is:open", b, cfg, "", today)

	m = send(m, tea.KeyMsg{Type: tea.KeyEnter}) // list -> detail
	if m.mode != modeDetail {
		t.Fatalf("mode = %v, want modeDetail", m.mode)
	}

	m = send(m, key('e')) // detail -> field picker
	if m.mode != modeFieldPicker {
		t.Fatalf("mode = %v, want modeFieldPicker", m.mode)
	}
	// Fields sort as: assignee, cat, est, prio, status.
	for range 4 {
		m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	if fi, ok := m.fieldPicker.SelectedItem().(fieldItem); !ok || fi.name != "status" {
		t.Fatalf("selected field = %+v, want status", m.fieldPicker.SelectedItem())
	}

	m = send(m, tea.KeyMsg{Type: tea.KeyEnter}) // field picker -> value picker
	if m.mode != modeValuePicker {
		t.Fatalf("mode = %v, want modeValuePicker", m.mode)
	}
	// status options: backlog, comite, ready, ...
	for range 2 {
		m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	if vi, ok := m.valuePicker.SelectedItem().(valueItem); !ok || vi.value != "ready" {
		t.Fatalf("selected value = %+v, want ready", m.valuePicker.SelectedItem())
	}

	m = send(m, tea.KeyMsg{Type: tea.KeyEnter}) // value picker -> write + back to detail

	if m.mode != modeDetail {
		t.Fatalf("mode = %v, want modeDetail after edit", m.mode)
	}
	if m.status == "" {
		t.Error("status empty after a successful edit, want a confirmation line")
	}
	if view := renderDetail(m.selected, cfg, m.status); !strings.Contains(view, "Status: ready") {
		t.Errorf("detail view = %q, want it to show the updated status", view)
	}

	got, err := b.GetLabels(id)
	if err != nil {
		t.Fatalf("GetLabels: %v", err)
	}
	if !containsLabel(got, "status:ready") {
		t.Errorf("backend labels = %v, want status:ready", got)
	}
	if containsLabel(got, "status:backlog") {
		t.Errorf("backend labels = %v, want status:backlog gone (single-cardinality swap)", got)
	}

	found := false
	for _, li := range m.list.Items() {
		it, ok := li.(item)
		if !ok || it.issue.ID != id {
			continue
		}
		found = true
		if !containsLabel(it.issue.Labels, "status:ready") {
			t.Errorf("re-rendered list item labels = %v, want status:ready", it.issue.Labels)
		}
	}
	if !found {
		t.Fatalf("edited issue %q not found in list items after edit", id)
	}
}

func TestUpdateCategoryFilterNarrowsList(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	bugID := gitbugiotest.NewIssue(t, b, "a bug")
	featureID := gitbugiotest.NewIssue(t, b, "a feature")
	if _, err := b.ApplyLabelChange(bugID, []string{"cat:bug"}, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := b.ApplyLabelChange(featureID, []string{"cat:feature"}, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	m := New(issues, "is:open", b, cfg, "", today)
	if got := len(m.list.Items()); got != 2 {
		t.Fatalf("initial list items = %d, want 2", got)
	}

	m = send(m, key('c'))
	if m.mode != modeCatFilter {
		t.Fatalf("mode = %v, want modeCatFilter", m.mode)
	}
	// catPicker items: (all), bug, feature, chore.
	m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	if vi, ok := m.catPicker.SelectedItem().(valueItem); !ok || vi.value != "bug" {
		t.Fatalf("selected category = %+v, want bug", m.catPicker.SelectedItem())
	}

	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeList {
		t.Fatalf("mode = %v, want modeList", m.mode)
	}
	if got := len(m.list.Items()); got != 1 {
		t.Fatalf("filtered list items = %d, want 1", got)
	}
	it, ok := m.list.Items()[0].(item)
	if !ok || it.issue.ID != bugID {
		t.Errorf("filtered item = %+v, want issue %q", m.list.Items()[0], bugID)
	}

	// "/" text search still works on top of the narrowed set — the list's
	// own filtering is unaffected by ours.
	if !m.list.FilteringEnabled() {
		t.Error("list filtering disabled after category filter, want it to stay available")
	}
}

func TestUpdateStructuredFilterNarrowsList(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	mine := gitbugiotest.NewIssue(t, b, "mine, not validated")
	other := gitbugiotest.NewIssue(t, b, "someone else's")
	validated := gitbugiotest.NewIssue(t, b, "mine, validated")
	for id, labels := range map[string][]string{
		mine:      {"assignee:guilbep"},
		other:     {"assignee:someone-else"},
		validated: {"assignee:guilbep", "status:validated"},
	} {
		if _, err := b.ApplyLabelChange(id, labels, nil); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	m := New(issues, "is:open", b, cfg, "", today)

	m = send(m, key('f'))
	if m.mode != modeFilterInput {
		t.Fatalf("mode = %v, want modeFilterInput", m.mode)
	}

	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("assignee:guilbep -status:validated")})
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != modeList {
		t.Fatalf("mode = %v, want modeList after applying the filter", m.mode)
	}
	if got := len(m.list.Items()); got != 1 {
		t.Fatalf("filtered list items = %d, want 1", got)
	}
	it, ok := m.list.Items()[0].(item)
	if !ok || it.issue.ID != mine {
		t.Errorf("filtered item = %+v, want issue %q", m.list.Items()[0], mine)
	}
	if m.list.Title != "assignee:guilbep -status:validated" {
		t.Errorf("list title = %q, want it to reflect the active filter", m.list.Title)
	}

	// Empty input clears back to the full set and the original title.
	m = send(m, key('f'))
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := len(m.list.Items()); got != 3 {
		t.Fatalf("list items after clearing = %d, want 3 (back to the full set)", got)
	}
	if m.list.Title != "is:open" {
		t.Errorf("list title after clearing = %q, want the original launch title back", m.list.Title)
	}
}

func TestUpdateStructuredFilterEscCancelsWithoutApplying(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	gitbugiotest.NewIssue(t, b, "widget")
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	m := New(issues, "is:open", b, cfg, "", today)
	m = send(m, key('f'))
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("title:*nomatch*")})
	m = send(m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.mode != modeList {
		t.Fatalf("mode = %v, want modeList after esc", m.mode)
	}
	if got := len(m.list.Items()); got != 1 {
		t.Errorf("list items after esc = %d, want 1 (unfiltered, esc must not apply)", got)
	}
}

// TestUpdateStructuredFilterRequiresMeDoesNotCrash mirrors runLs's CLI-side
// check (filter.RequiresMe): @me with no --me/GIT_BOARD_ME configured must
// surface as a status line, not panic or silently match nothing.
func TestUpdateStructuredFilterRequiresMeDoesNotCrash(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	gitbugiotest.NewIssue(t, b, "widget")
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	m := New(issues, "is:open", b, cfg, "" /* no me */, today)
	m = send(m, key('f'))
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("assignee:@me")})
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != modeList {
		t.Fatalf("mode = %v, want modeList", m.mode)
	}
	if !strings.Contains(m.status, "@me") {
		t.Errorf("status = %q, want it to explain the missing --me", m.status)
	}
	if got := len(m.list.Items()); got != 1 {
		t.Errorf("list items = %d, want the list untouched (filter never applied)", got)
	}
}

// errBackend is a fields.Backend stub whose write always fails, for the
// write-error-must-not-crash case. It's the one place a stub replaces
// gitbugiotest: every other test drives a real repo.
type errBackend struct{}

func (errBackend) GetLabels(string) ([]string, error) { return []string{"status:backlog"}, nil }

func (errBackend) ApplyLabelChange(string, []string, []string) ([]string, error) {
	return nil, fmt.Errorf("boom: simulated write failure")
}

func TestUpdateEditWriteErrorShowsStatusAndDoesNotCrash(t *testing.T) {
	cfg := mustLoadConfig(t)
	issues := []gitbugio.Issue{
		{ID: "deadbeef", Title: "x", Status: "open", Labels: []string{"status:backlog"}},
	}
	m := New(issues, "is:open", errBackend{}, cfg, "", today)

	m = send(m, tea.KeyMsg{Type: tea.KeyEnter}) // detail
	m = send(m, key('e'))                       // field picker
	for range 4 {
		m = send(m, tea.KeyMsg{Type: tea.KeyDown}) // -> status
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter}) // value picker
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter}) // pick first option -> write fails

	if m.mode != modeDetail {
		t.Fatalf("mode = %v, want modeDetail (a write error must not crash the TUI)", m.mode)
	}
	if !strings.Contains(m.status, "boom") {
		t.Errorf("status = %q, want it to surface the write error", m.status)
	}
	if v := m.View(); v == "" {
		t.Error("View() empty after a write error")
	}
}

func containsLabel(labels []string, label string) bool {
	for _, l := range labels {
		if l == label {
			return true
		}
	}
	return false
}

// TestWindowSizeMsgOnFreshModelDoesNotPanic is a regression test for a real
// crash: bubbletea sends a WindowSizeMsg immediately on startup, before the
// user has pressed anything, and Update's WindowSizeMsg case calls SetSize
// on all four pickers (list, fieldPicker, valuePicker, catPicker)
// unconditionally. valuePicker/catPicker used to be left as the zero-value
// list.Model until the user actually opened them (updateFieldPicker, the
// "c" key) — SetSize on a zero-value list.Model panics (nil delegate),
// so every `git-board tui` launch crashed before this was fixed.
func TestWindowSizeMsgOnFreshModelDoesNotPanic(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	gitbugiotest.NewIssue(t, b, "widget")
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	m := New(issues, "is:open", b, cfg, "", today)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Update(WindowSizeMsg) on a fresh Model panicked: %v", r)
		}
	}()
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	if v := m.View(); v == "" {
		t.Error("View() empty after the initial resize")
	}
}

func titles(items []list.Item) []string {
	out := make([]string, len(items))
	for i, li := range items {
		out[i] = li.(item).issue.Title
	}
	return out
}

func TestUpdateSortOrdersList(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	p2 := gitbugiotest.NewIssue(t, b, "p2 issue")
	p0 := gitbugiotest.NewIssue(t, b, "p0 issue")
	p1 := gitbugiotest.NewIssue(t, b, "p1 issue")
	for id, prio := range map[string]string{p2: "prio:p2", p0: "prio:p0", p1: "prio:p1"} {
		if _, err := b.ApplyLabelChange(id, []string{prio}, nil); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	m := New(issues, "is:open", b, cfg, "", today)

	m = send(m, key('s'))
	if m.mode != modeSortInput {
		t.Fatalf("mode = %v, want modeSortInput", m.mode)
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("prio")})
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != modeList {
		t.Fatalf("mode = %v, want modeList after applying the sort", m.mode)
	}
	want := []string{"p0 issue", "p1 issue", "p2 issue"}
	if got := titles(m.list.Items()); !slices.Equal(got, want) {
		t.Errorf("order after sort by prio = %v, want %v", got, want)
	}

	// "prio desc" reverses it.
	m = send(m, key('s'))
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("prio desc")})
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})
	wantDesc := []string{"p2 issue", "p1 issue", "p0 issue"}
	if got := titles(m.list.Items()); !slices.Equal(got, wantDesc) {
		t.Errorf("order after sort by prio desc = %v, want %v", got, wantDesc)
	}
}

func TestUpdateSortPersistsAcrossFilter(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	p2 := gitbugiotest.NewIssue(t, b, "p2 issue")
	p0 := gitbugiotest.NewIssue(t, b, "p0 issue")
	closed := gitbugiotest.NewIssue(t, b, "p1 issue, will be excluded")
	for id, prio := range map[string]string{p2: "prio:p2", p0: "prio:p0", closed: "prio:p1"} {
		if _, err := b.ApplyLabelChange(id, []string{prio}, nil); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	m := New(issues, "is:open", b, cfg, "", today)
	m = send(m, key('s'))
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("prio")})
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})

	// Narrowing afterward must not lose the sort.
	m = send(m, key('f'))
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-title:*excluded*")})
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})

	want := []string{"p0 issue", "p2 issue"}
	if got := titles(m.list.Items()); !slices.Equal(got, want) {
		t.Errorf("order after filtering a sorted list = %v, want %v (sort should persist)", got, want)
	}
}

func TestUpdateSortClearRestoresNaturalOrder(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	p2 := gitbugiotest.NewIssue(t, b, "p2 issue")
	p0 := gitbugiotest.NewIssue(t, b, "p0 issue")
	p1 := gitbugiotest.NewIssue(t, b, "p1 issue")
	for id, prio := range map[string]string{p2: "prio:p2", p0: "prio:p0", p1: "prio:p1"} {
		if _, err := b.ApplyLabelChange(id, []string{prio}, nil); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	issues, err := b.ListIssues() // natural (unsorted) order
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	natural := titles(ToItems(issues))

	m := New(issues, "is:open", b, cfg, "", today)
	m = send(m, key('s'))
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("prio")})
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})
	// Regression check for the sort itself, independent of what "natural"
	// happens to be (with 3 distinct prio values this is an unambiguous
	// assertion, not a coin-flip like a 2-item natural-order comparison
	// would be).
	wantSorted := []string{"p0 issue", "p1 issue", "p2 issue"}
	if got := titles(m.list.Items()); !slices.Equal(got, wantSorted) {
		t.Fatalf("order after sort by prio = %v, want %v", got, wantSorted)
	}

	// Empty input clears the sort — this must recompute from the
	// still-narrowed base set in its natural order, not just re-run a
	// no-op sort over the already-sorted visible items (which would
	// silently leave the stale sorted order in place).
	m = send(m, key('s'))
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := titles(m.list.Items()); !slices.Equal(got, natural) {
		t.Errorf("order after clearing sort = %v, want the natural order %v back", got, natural)
	}
}
