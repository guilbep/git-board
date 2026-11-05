package tui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/guilbep/git-board/next/app"
	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/hermetic"
)

func TestMain(m *testing.M) { hermetic.Run(m) }

// send drives Model.Update the way bubbletea's runtime would.
func send(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

// run drives Update and runs the command it answers, once.
func run(m Model, msg tea.Msg) Model {
	next, cmd := m.Update(msg)
	m = next.(Model)
	if cmd != nil {
		if out := cmd(); out != nil {
			m = send(m, out)
		}
	}
	return m
}

func key(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

var enter = tea.KeyMsg{Type: tea.KeyEnter}

func typed(m Model, s string) Model {
	for _, r := range s {
		m = send(m, key(r))
	}
	return m
}

const toml = `[fields.status]
single = true
options = ["backlog", "ready", "done"]

[fields.prio]
single = true
options = ["p0", "p1", "p2"]

[fields.cat]
single = true
options = ["bug", "feature"]
`

// fixture is a board with a person who writes, and its issues made with
// these labels, by title.
func fixture(t *testing.T, made map[string][]string) (*app.Board, []issues.Issue) {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := app.Open(t.Context(), dir, app.Options{Who: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	me, err := b.People.New(t.Context(), "dev", "dev@example.com")
	if err != nil {
		t.Fatal(err)
	}
	b.Issues.Me = me.ID
	for _, title := range slices.Sorted(func(yield func(string) bool) {
		for k := range made {
			if !yield(k) {
				return
			}
		}
	}) {
		if _, err := b.Issues.New(t.Context(), issues.NewReq{Title: title, Labels: made[title]}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := b.Issues.List(t.Context(), issues.ListReq{Filter: issues.DefaultFilter})
	if err != nil {
		t.Fatal(err)
	}
	return b, list
}

func model(t *testing.T, made map[string][]string) (Model, *app.Board) {
	b, list := fixture(t, made)
	return New(list, "is:open", &board{context.Background(), b, ""}, b.Config.Schema, nil), b
}

func titles(items []list.Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.(item).issue.Title)
	}
	return out
}

// e edits a field from the detail view, through the board: the write
// lands, and the list shows it.
func TestEditWritesThrough(t *testing.T) {
	t.Parallel()
	m, b := model(t, map[string][]string{"widget": {"status:backlog"}})
	m = send(m, enter)
	if m.mode != modeDetail {
		t.Fatalf("mode %v after enter", m.mode)
	}
	m = send(m, key('e'))
	names := FieldNames(m.cfg, m.allIssues)
	m.fieldPicker.Select(slices.Index(names, "status"))
	m = send(m, enter)
	if m.mode != modeValuePicker {
		t.Fatalf("mode %v after picking status", m.mode)
	}
	m.valuePicker.Select(2) // done
	m = send(m, enter)
	if !strings.HasPrefix(m.status, "updated") {
		t.Fatalf("status %q", m.status)
	}
	th, err := b.Issues.Show(t.Context(), string(m.selected.ID))
	if err != nil || !slices.Contains(th.Labels, "status:done") {
		t.Errorf("labels on the board %v, %v", th.Labels, err)
	}
	if !slices.Contains(m.list.Items()[0].(item).issue.Labels, "status:done") {
		t.Error("the list doesn't show the edit")
	}
}

// c narrows to a category, and (all) clears it.
func TestCategoryFilter(t *testing.T) {
	t.Parallel()
	m, _ := model(t, map[string][]string{"a bug": {"cat:bug"}, "a feature": {"cat:feature"}})
	m = send(m, key('c'))
	m.catPicker.Select(1) // bug
	m = send(m, enter)
	if got := titles(m.list.Items()); !slices.Equal(got, []string{"a bug"}) {
		t.Errorf("filtered %v", got)
	}
	m = send(m, key('c'))
	m = send(m, enter) // (all)
	if len(m.list.Items()) != 2 {
		t.Errorf("cleared to %v", titles(m.list.Items()))
	}
}

// f filters with ls's grammar, from every issue; empty clears.
func TestStructuredFilter(t *testing.T) {
	t.Parallel()
	m, _ := model(t, map[string][]string{"one": {"prio:p0"}, "two": {"prio:p1"}, "three": {"prio:p0"}})
	m = typed(send(m, key('f')), "prio:p0")
	m = send(m, enter)
	if got := titles(m.list.Items()); len(got) != 2 || slices.Contains(got, "two") || m.list.Title != "prio:p0" {
		t.Errorf("filtered %v, title %q", got, m.list.Title)
	}
	m = send(send(m, key('f')), enter)
	if len(m.list.Items()) != 3 || m.list.Title != "is:open" {
		t.Errorf("cleared %v", titles(m.list.Items()))
	}
}

// s sorts, the sort holds across a filter, and empty clears it.
func TestSort(t *testing.T) {
	t.Parallel()
	m, _ := model(t, map[string][]string{"a": {"prio:p2"}, "b": {"prio:p0"}, "c": {"prio:p1"}})
	m = send(typed(send(m, key('s')), "prio"), enter)
	if got := titles(m.list.Items()); !slices.Equal(got, []string{"b", "c", "a"}) {
		t.Errorf("sorted %v", got)
	}
	m = send(typed(send(m, key('f')), "-prio:p1"), enter)
	if got := titles(m.list.Items()); !slices.Equal(got, []string{"b", "a"}) {
		t.Errorf("filtered, sorted %v", got)
	}
}

type failing struct{ Backend }

func (failing) Set(string, string, string) ([]string, string, error) {
	return nil, "", errors.New("no network")
}

// A write that fails says so, and the program goes on.
func TestEditErrorShows(t *testing.T) {
	t.Parallel()
	b, list := fixture(t, map[string][]string{"widget": nil})
	m := New(list, "is:open", failing{&board{context.Background(), b, ""}}, b.Config.Schema, nil)
	m = send(send(m, enter), key('e'))
	m = send(m, enter)
	if m.mode == modeValueInput {
		m = send(typed(m, "x"), enter)
	} else {
		m = send(m, enter)
	}
	if m.mode != modeDetail || !strings.Contains(m.status, "edit failed: no network") {
		t.Errorf("mode %v, status %q", m.mode, m.status)
	}
}

// A size before anything else never panics; the list keeps a line for
// the status.
func TestWindowSize(t *testing.T) {
	t.Parallel()
	m, _ := model(t, map[string][]string{"widget": nil})
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.list.Height() != 23 {
		t.Errorf("height %d", m.list.Height())
	}
	_ = m.View()
}

type fakeBridge struct {
	read, applied int
	changed       []string
}

func (f *fakeBridge) Read() (string, error)    { f.read++; return "2 matched (1 changed)", nil }
func (f *fakeBridge) Apply() ([]string, error) { f.applied++; return f.changed, nil }

// i reads the Project and says the plan; o applies it, and the changed
// issues show their labels; o before i refuses; with no Project, i says
// how to configure one.
func TestBridgeKeys(t *testing.T) {
	t.Parallel()
	b, list := fixture(t, map[string][]string{"widget": {"prio:p2"}})
	m := New(list, "is:open", &board{context.Background(), b, ""}, b.Config.Schema, nil)
	if m = send(m, key('i')); !strings.Contains(m.status, "no GitHub project configured") {
		t.Errorf("no bridge: %q", m.status)
	}
	id := string(list[0].ID)
	fb := &fakeBridge{changed: []string{id}}
	m = New(list, "is:open", &board{context.Background(), b, ""}, b.Config.Schema, fb)
	if m = send(m, key('o')); !strings.Contains(m.status, "press i first") || fb.applied != 0 {
		t.Errorf("o before i: %q", m.status)
	}
	if m = run(m, key('i')); m.status != "2 matched (1 changed) — press o to apply" || fb.read != 1 {
		t.Errorf("i: %q", m.status)
	}
	if _, err := b.Issues.Set(t.Context(), issues.SetReq{Name: id, Assign: []string{"prio=p0"}}); err != nil {
		t.Fatal(err)
	}
	if m = send(m, key('o')); m.status != "bridge applied: 1 issue(s) updated" || fb.applied != 1 {
		t.Errorf("o: %q", m.status)
	}
	if !slices.Contains(m.list.Items()[0].(item).issue.Labels, "prio:p0") {
		t.Errorf("labels not read back: %v", m.list.Items()[0].(item).issue.Labels)
	}
}
