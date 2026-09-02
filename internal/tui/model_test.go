package tui

import (
	"fmt"
	"strings"
	"testing"

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
