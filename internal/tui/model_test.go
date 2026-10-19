package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghbridge"
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
	t.Parallel()
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

	m := New(issues, "is:open", b, cfg, "", today, noopReopen(b))

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
	t.Parallel()
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

	m := New(issues, "is:open", b, cfg, "", today, noopReopen(b))
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
	t.Parallel()
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

	m := New(issues, "is:open", b, cfg, "", today, noopReopen(b))

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
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	gitbugiotest.NewIssue(t, b, "widget")
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	m := New(issues, "is:open", b, cfg, "", today, noopReopen(b))
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
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	gitbugiotest.NewIssue(t, b, "widget")
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	m := New(issues, "is:open", b, cfg, "" /* no me */, today, noopReopen(b))
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

// f's words look in descriptions and comments too, as ls and the web page
// do: one grammar, one matcher.
func TestUpdateStructuredFilterSearchesComments(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "Train distances")
	if _, err := b.AddComment(id, "the zebra table is wrong"); err != nil {
		t.Fatal(err)
	}
	gitbugiotest.NewIssue(t, b, "Buildings")
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	m := New(issues, "is:open", b, cfg, "", today, noopReopen(b))
	m = send(m, key('f'))
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zebra")})
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := len(m.list.Items()); got != 1 {
		t.Errorf("list items = %d, want the issue whose comment says zebra (status %q)", got, m.status)
	}
}

// errBackend is a tui.Backend stub whose write always fails, for the
// write-error-must-not-crash case. It's the one place a stub replaces
// gitbugiotest: every other test drives a real repo.
type errBackend struct{}

func (errBackend) GetLabels(string) ([]string, error) { return []string{"status:backlog"}, nil }

func (errBackend) ApplyLabelChange(string, []string, []string) ([]string, error) {
	return nil, fmt.Errorf("boom: simulated write failure")
}

func (errBackend) Close() error { return nil }

// noopReopen is a reopen stub for tests that don't exercise "g" — it's
// never actually called, but New/Model need a non-nil func regardless.
func noopReopen(b Backend) func() (Backend, error) {
	return func() (Backend, error) { return b, nil }
}

func TestUpdateEditWriteErrorShowsStatusAndDoesNotCrash(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	issues := []gitbugio.Issue{
		{ID: "deadbeef", Title: "x", Status: "open", Labels: []string{"status:backlog"}},
	}
	m := New(issues, "is:open", errBackend{}, cfg, "", today, noopReopen(errBackend{}))

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
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	gitbugiotest.NewIssue(t, b, "widget")
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	m := New(issues, "is:open", b, cfg, "", today, noopReopen(b))

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
	t.Parallel()
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

	m := New(issues, "is:open", b, cfg, "", today, noopReopen(b))

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
	t.Parallel()
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

	m := New(issues, "is:open", b, cfg, "", today, noopReopen(b))
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
	t.Parallel()
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

	m := New(issues, "is:open", b, cfg, "", today, noopReopen(b))
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

// TestGKeyClosesBackendBeforeShellingOut is a regression test for a real
// bug: "g" shells out to `git bug bug show`, but git-board's own backend
// holds git-bug's repo-level lock open for the TUI's whole session —
// confirmed directly (outside this test suite, against the real
// co2-calculator repo) that the subprocess fails with "already locked by
// process <our own pid>" unless we release it first. This checks the
// release actually happens, using a real repo (no mock): after "g", the
// *original* backend reference must be genuinely closed.
func TestGKeyClosesBackendBeforeShellingOut(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "widget")
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	m := New(issues, "is:open", b, cfg, "", today, noopReopen(b))
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter}) // list -> detail
	if m.mode != modeDetail {
		t.Fatalf("mode = %v, want modeDetail", m.mode)
	}

	next, cmd := m.Update(key('g'))
	m = next.(Model)
	if cmd == nil {
		t.Error("Update after \"g\" returned a nil Cmd, want the ExecProcess command")
	}
	if m.mode != modeDetail {
		t.Errorf("mode = %v, want modeDetail (g doesn't change mode, the exec suspends the whole program)", m.mode)
	}

	// The real check: b (captured before New, same backend the Model
	// started with) must now be genuinely closed — a real operation on
	// it fails. If "g" forgot to Close(), this would still succeed and
	// the bug would still be present.
	if _, err := b.GetLabels(id); err == nil {
		t.Error("backend still usable after \"g\" — it should have been closed to release the repo lock before shelling out")
	}
}

// TestGitBugShowFinishedReopensBackend covers gitBugShowFinishedMsg's
// three outcomes: subprocess ok, subprocess failed (but reopen still
// must succeed so further edits keep working), and reopen itself
// failing.
func TestGitBugShowFinishedReopensBackend(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)

	t.Run("subprocess ok", func(t *testing.T) {
		stale := gitbugiotest.New(t) // simulates the backend "g" already closed
		_ = stale.Close()
		fresh := gitbugiotest.New(t)
		id := gitbugiotest.NewIssue(t, fresh, "widget")
		m := New(nil, "is:open", stale, cfg, "", today, func() (Backend, error) { return fresh, nil })

		m = send(m, gitBugShowFinishedMsg{err: nil})

		if m.status != "" {
			t.Errorf("status = %q, want empty after a clean git bug bug show", m.status)
		}
		if _, err := m.backend.GetLabels(id); err != nil {
			t.Errorf("reopened backend not usable: %v", err)
		}
	})

	t.Run("subprocess failed, reopen still succeeds", func(t *testing.T) {
		stale := gitbugiotest.New(t) // simulates the backend "g" already closed
		_ = stale.Close()
		fresh := gitbugiotest.New(t)
		id := gitbugiotest.NewIssue(t, fresh, "widget")
		m := New(nil, "is:open", stale, cfg, "", today, func() (Backend, error) { return fresh, nil })

		m = send(m, gitBugShowFinishedMsg{err: fmt.Errorf("git-bug not found")})

		if !strings.Contains(m.status, "git-bug not found") {
			t.Errorf("status = %q, want it to surface the subprocess error", m.status)
		}
		if _, err := m.backend.GetLabels(id); err != nil {
			t.Errorf("reopened backend not usable even though the subprocess itself failed: %v", err)
		}
	})

	t.Run("reopen itself fails", func(t *testing.T) {
		stale := gitbugiotest.New(t) // simulates the backend "g" already closed
		_ = stale.Close()
		m := New(nil, "is:open", stale, cfg, "", today, func() (Backend, error) {
			return nil, fmt.Errorf("repo vanished")
		})

		m = send(m, gitBugShowFinishedMsg{err: nil})

		if m.backend != nil {
			t.Error("backend should be nil after a failed reopen, not left pointing at a closed handle")
		}
		if !strings.Contains(m.status, "repo vanished") {
			t.Errorf("status = %q, want it to surface the reopen failure", m.status)
		}
	})
}

func TestIKeyRequiresGithubProjectConfigured(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t) // no [github-project] in the shared fixture
	b := gitbugiotest.New(t)
	m := New(nil, "is:open", b, cfg, "", today, noopReopen(b))

	m = send(m, key('i'))

	if m.bridgeBusy {
		t.Error("bridgeBusy = true, want false — nothing should start without a configured project")
	}
	if !strings.Contains(m.status, "bridge configure") {
		t.Errorf("status = %q, want it to point at `git-board bridge configure`", m.status)
	}
}

func TestIKeyStartsPullWhenConfigured(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	cfg.GithubProject = config.GithubProject{Owner: "acme", Number: 40}
	b := gitbugiotest.New(t)
	m := New(nil, "is:open", b, cfg, "", today, noopReopen(b))

	next, cmd := m.Update(key('i'))
	m = next.(Model)

	if !m.bridgeBusy {
		t.Error("bridgeBusy = false, want true once a pull has started")
	}
	if cmd == nil {
		t.Error("Update after \"i\" returned a nil Cmd, want the pull command")
	}
	if !strings.Contains(m.status, "acme/40") {
		t.Errorf("status = %q, want it to name the project being pulled", m.status)
	}

	// A second "i" while busy must not double-fire.
	next, cmd = m.Update(key('i'))
	m = next.(Model)
	if cmd != nil {
		t.Error("a second \"i\" while bridgeBusy should be a no-op, got a non-nil Cmd")
	}
}

func TestBridgePullFinishedStoresPlanForO(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	m := New(nil, "is:open", b, cfg, "", today, noopReopen(b))
	m.bridgeBusy = true

	t.Run("success", func(t *testing.T) {
		plan := ghbridge.Plan{Changes: []ghbridge.IssueChange{{BugID: "x", Added: []string{"prio:p1"}}}}
		got := send(m, bridgePullFinishedMsg{plan: plan})

		if got.bridgeBusy {
			t.Error("bridgeBusy still true after the pull finished")
		}
		if got.bridgePlan == nil {
			t.Fatal("bridgePlan not stored after a successful pull")
		}
		if !strings.Contains(got.status, "press o to apply") {
			t.Errorf("status = %q, want it to prompt for o", got.status)
		}
	})

	t.Run("error", func(t *testing.T) {
		got := send(m, bridgePullFinishedMsg{err: fmt.Errorf("rate limited")})

		if got.bridgeBusy {
			t.Error("bridgeBusy still true after a failed pull")
		}
		if got.bridgePlan != nil {
			t.Error("bridgePlan set despite the fetch failing")
		}
		if !strings.Contains(got.status, "rate limited") {
			t.Errorf("status = %q, want it to surface the fetch error", got.status)
		}
	})
}

func TestOKeyRequiresAPreviewFirst(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	m := New(nil, "is:open", b, cfg, "", today, noopReopen(b))

	m = send(m, key('o'))

	if !strings.Contains(m.status, "press i first") {
		t.Errorf("status = %q, want it to say to preview with i first", m.status)
	}
}

// TestOKeyAppliesThePreviewedPlan is the real end-to-end check: a plan
// naming a real seeded issue, applied via a real backend (no mocks) --
// the write must land for real (old label gone, new one present, same
// standard as every other write in this test file), the in-memory list
// must reflect it without a re-list, and the plan must be cleared so o
// can't be pressed twice on stale data.
func TestOKeyAppliesThePreviewedPlan(t *testing.T) {
	t.Parallel()
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

	m := New(issues, "is:open", b, cfg, "", today, noopReopen(b))
	m.bridgePlan = &ghbridge.Plan{
		Changes: []ghbridge.IssueChange{
			{BugID: id, Added: []string{"status:ready"}, Removed: []string{"status:backlog"}},
		},
	}

	m = send(m, key('o'))

	if m.bridgePlan != nil {
		t.Error("bridgePlan not cleared after applying — a second o would silently re-apply stale data")
	}
	if !strings.Contains(m.status, "1 issue(s) updated") {
		t.Errorf("status = %q, want an applied-count confirmation", m.status)
	}

	got, err := b.GetLabels(id)
	if err != nil {
		t.Fatalf("GetLabels: %v", err)
	}
	if !containsLabel(got, "status:ready") {
		t.Errorf("backend labels = %v, want status:ready (the real write)", got)
	}
	if containsLabel(got, "status:backlog") {
		t.Errorf("backend labels = %v, want status:backlog gone", got)
	}

	found := false
	for _, li := range m.list.Items() {
		it, ok := li.(item)
		if !ok || it.issue.ID != id {
			continue
		}
		found = true
		if !containsLabel(it.issue.Labels, "status:ready") || containsLabel(it.issue.Labels, "status:backlog") {
			t.Errorf("re-rendered list item labels = %v, want status:ready without a re-list from the backend", it.issue.Labels)
		}
	}
	if !found {
		t.Fatalf("issue %q not found in list items after applying the bridge plan", id)
	}
}
