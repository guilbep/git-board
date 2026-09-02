// Package tui is an interactive bubbletea front-end over the same
// internal/gitbugio issue data the CLI's ls/view commands render as a flat
// table. It adds scrolling and search (both native to bubbles/list), a
// detail view that recognizes configured fields, inline field editing, and
// a quick category filter — all layered on bubbles/list rather than
// reinventing picker/filter UI.
package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/filter"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// item adapts a gitbugio.Issue to bubbles/list's DefaultItem interface.
type item struct {
	issue gitbugio.Issue
}

func (i item) Title() string { return i.issue.Title }

func (i item) Description() string {
	return fmt.Sprintf("%s  %s  %s", shortID(i.issue.ID), i.issue.Status, strings.Join(i.issue.Labels, ", "))
}

// FilterValue feeds bubbles/list's built-in filter, so "/" search matches
// title and labels.
func (i item) FilterValue() string {
	return i.issue.Title + " " + strings.Join(i.issue.Labels, " ")
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// ToItems converts issues to list.Items, preserving order. Pure — no
// bubbletea involved, so it's unit tested directly.
func ToItems(issues []gitbugio.Issue) []list.Item {
	items := make([]list.Item, len(issues))
	for i, issue := range issues {
		items[i] = item{issue: issue}
	}
	return items
}

// mode selects which of the model's sub-views is active.
type mode int

const (
	modeList mode = iota
	modeDetail
	modeFieldPicker
	modeValuePicker
	modeValueInput
	modeCatFilter
	modeFilterInput
	modeSortInput
)

// Model is the bubbletea model: the issue list, an optional detail view,
// and the edit-field/value, category-filter and structured-filter layers on
// top.
type Model struct {
	list     list.Model
	mode     mode
	selected gitbugio.Issue

	backend fields.Backend
	cfg     *config.Config
	me      string
	today   time.Time

	fieldPicker list.Model
	valuePicker list.Model
	catPicker   list.Model
	valueInput  textinput.Model
	editField   string

	// filterInput is the "f" structured-filter box (internal/filter's
	// field:value grammar — the same one ls/view/tui's launch argument
	// use), distinct from bubbles/list's own "/" fuzzy text search.
	// filterExpr is the currently-applied expression (empty = none);
	// defaultTitle is what the list title reverts to on clear.
	filterInput  textinput.Model
	filterExpr   string
	defaultTitle string

	// sortInput is the "s" sort box — same "field" / "field desc" spec
	// board.SortByField (and the CLI's --sort) already use. sortSpec
	// persists across filter/category-filter changes, so narrowing after
	// sorting doesn't lose the order — every place list items get
	// (re)built routes through sortedItems.
	sortInput textinput.Model
	sortSpec  string

	status string

	// allIssues is the unfiltered set, kept so the category filter and the
	// structured filter can both be cleared back to it (and so applying
	// one after the other narrows from the full set, not compounds off
	// whatever the other left in m.list).
	allIssues []gitbugio.Issue

	// baseIssues is the currently narrowed set *before* sorting — set by
	// applyCatFilter/applyStructuredFilter (== allIssues when nothing's
	// narrowed), read by applySort. Keeping this separate from m.list's
	// actual items means clearing the sort (empty spec) restores the true
	// natural order of whatever's currently narrowed, instead of "no-op
	// sorting" an already-sorted list — sortedItems copies before sorting,
	// it never mutates baseIssues itself.
	baseIssues []gitbugio.Issue
}

// New builds a Model over issues. title is shown as the list header (the
// filter expression the caller ran, e.g. "is:open"). backend/cfg/me/today
// are what field edits need to write through fields.Apply.
func New(issues []gitbugio.Issue, title string, backend fields.Backend, cfg *config.Config, me string, today time.Time) Model {
	l := list.New(ToItems(issues), list.NewDefaultDelegate(), 0, 0)
	l.Title = title
	l.SetFilteringEnabled(true)

	fp := list.New(fieldPickerItems(cfg), list.NewDefaultDelegate(), 0, 0)
	fp.Title = "Edit field"
	fp.SetFilteringEnabled(false)

	// valuePicker and catPicker are re-populated with real items when the
	// user actually opens them (updateFieldPicker, the "c" key), but they
	// still need to be real list.Models — not the zero value — from the
	// start: Update's WindowSizeMsg case calls SetSize on all four pickers
	// unconditionally, and bubbletea sends that message immediately on
	// startup, before the user has pressed anything. A zero-value
	// list.Model's internal delegate/paginator are nil, so SetSize on one
	// panics on every launch (confirmed: this exact panic was reported).
	vp := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	vp.SetFilteringEnabled(false)

	cp := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	cp.SetFilteringEnabled(false)

	vi := textinput.New()
	vi.Placeholder = "value"

	fi := textinput.New()
	fi.Placeholder = `assignee:guilbep -status:validated`
	fi.Prompt = "filter: "

	si := textinput.New()
	si.Placeholder = `prio asc`
	si.Prompt = "sort: "

	return Model{
		list:         l,
		backend:      backend,
		cfg:          cfg,
		me:           me,
		today:        today,
		fieldPicker:  fp,
		valuePicker:  vp,
		catPicker:    cp,
		valueInput:   vi,
		filterInput:  fi,
		sortInput:    si,
		defaultTitle: title,
		allIssues:    issues,
		baseIssues:   issues,
	}
}

func (m Model) Init() tea.Cmd { return nil }

// gitBugShowFinishedMsg is what tea.ExecProcess sends back once
// `git bug bug show` exits and the terminal is handed back to us.
type gitBugShowFinishedMsg struct{ err error }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case gitBugShowFinishedMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("git bug bug show failed: %v (is git-bug installed as a git subcommand?)", msg.err)
		} else {
			m.status = ""
		}
		return m, nil

	case tea.WindowSizeMsg:
		// Reserve one line for the status message View() appends below
		// the list; the pickers don't append one, so they get the full
		// height.
		m.list.SetSize(msg.Width, msg.Height-1)
		m.fieldPicker.SetSize(msg.Width, msg.Height)
		m.valuePicker.SetSize(msg.Width, msg.Height)
		m.catPicker.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeDetail:
			return m.updateDetail(msg)
		case modeFieldPicker:
			return m.updateFieldPicker(msg)
		case modeValuePicker:
			return m.updateValuePicker(msg)
		case modeValueInput:
			return m.updateValueInput(msg)
		case modeCatFilter:
			return m.updateCatFilter(msg)
		case modeFilterInput:
			return m.updateFilterInput(msg)
		case modeSortInput:
			return m.updateSortInput(msg)
		default:
			return m.updateList(msg)
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While the filter input is being typed into, every key (including
	// "q"/"esc"/"c"/"f"/"s") belongs to the list's own filter handling — don't
	// intercept.
	if m.list.FilterState() != list.Filtering {
		switch msg.String() {
		case "enter":
			if it, ok := m.list.SelectedItem().(item); ok {
				m.selected = it.issue
				m.status = ""
				m.mode = modeDetail
			}
			return m, nil
		case "c":
			if f, ok := catField(m.cfg); ok {
				cp := list.New(catFilterItems(f), list.NewDefaultDelegate(), m.list.Width(), m.list.Height())
				cp.Title = "Filter by " + displayFieldName(catFieldName)
				cp.SetFilteringEnabled(false)
				m.catPicker = cp
				m.mode = modeCatFilter
			} else {
				m.status = fmt.Sprintf("no %q field configured for the quick filter", catFieldName)
			}
			return m, nil
		case "f":
			// Always opens empty (not pre-filled with the active filter):
			// keeps "press f, enter" a predictable clear, and re-typing a
			// short filter expression isn't a real burden. The active
			// filter (if any) is still visible in the list title.
			m.filterInput.SetValue("")
			m.filterInput.Focus()
			m.mode = modeFilterInput
			return m, textinput.Blink
		case "s":
			m.sortInput.SetValue("")
			m.sortInput.Focus()
			m.mode = modeSortInput
			return m, textinput.Blink
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = modeList
		m.status = ""
	case "e":
		m.fieldPicker.SetItems(fieldPickerItems(m.cfg))
		m.mode = modeFieldPicker
	case "g":
		// Body/comments aren't git-board's data at all — they're
		// git-bug's. Shell out to git-bug's own `bug show` rather than
		// reimplementing a comment/body viewer: tea.ExecProcess suspends
		// this program, hands the terminal to `git bug bug show <id>`,
		// and resumes here (via gitBugShowFinishedMsg) when it exits.
		id := m.selected.ID
		return m, tea.ExecProcess(exec.Command("git", "bug", "bug", "show", id), func(err error) tea.Msg {
			return gitBugShowFinishedMsg{err: err}
		})
	}
	return m, nil
}

func (m Model) updateFieldPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = modeDetail
		return m, nil
	case "enter":
		fi, ok := m.fieldPicker.SelectedItem().(fieldItem)
		if !ok {
			return m, nil
		}
		m.editField = fi.name
		opts := valuePickerItems(m.cfg, fi.name)
		if len(opts) == 0 {
			m.valueInput.SetValue("")
			m.valueInput.Focus()
			m.mode = modeValueInput
			return m, textinput.Blink
		}
		vp := list.New(opts, list.NewDefaultDelegate(), m.fieldPicker.Width(), m.fieldPicker.Height())
		vp.Title = "Set " + displayFieldName(fi.name)
		vp.SetFilteringEnabled(false)
		m.valuePicker = vp
		m.mode = modeValuePicker
		return m, nil
	}
	var cmd tea.Cmd
	m.fieldPicker, cmd = m.fieldPicker.Update(msg)
	return m, cmd
}

func (m Model) updateValuePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = modeFieldPicker
		return m, nil
	case "enter":
		if vi, ok := m.valuePicker.SelectedItem().(valueItem); ok {
			m = m.applyEdit(vi.value)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.valuePicker, cmd = m.valuePicker.Update(msg)
	return m, cmd
}

func (m Model) updateValueInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeFieldPicker
		return m, nil
	case "enter":
		m = m.applyEdit(m.valueInput.Value())
		return m, nil
	}
	var cmd tea.Cmd
	m.valueInput, cmd = m.valueInput.Update(msg)
	return m, cmd
}

func (m Model) updateCatFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = modeList
		return m, nil
	case "enter":
		if vi, ok := m.catPicker.SelectedItem().(valueItem); ok {
			m = m.applyCatFilter(vi.value)
		}
		m.mode = modeList
		return m, nil
	}
	var cmd tea.Cmd
	m.catPicker, cmd = m.catPicker.Update(msg)
	return m, cmd
}

func (m Model) updateFilterInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		return m, nil
	case "enter":
		m = m.applyStructuredFilter(strings.TrimSpace(m.filterInput.Value()))
		m.mode = modeList
		return m, nil
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	return m, cmd
}

func (m Model) updateSortInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		return m, nil
	case "enter":
		m = m.applySort(strings.TrimSpace(m.sortInput.Value()))
		m.mode = modeList
		return m, nil
	}
	var cmd tea.Cmd
	m.sortInput, cmd = m.sortInput.Update(msg)
	return m, cmd
}

// applyEdit writes m.editField=value on the selected issue and returns to
// the detail view with a status line — a confirmation on success, the error
// message on failure. It never panics/crashes the program on a write error
// (e.g. a repo/network issue): the user sees it and can retry or back out.
func (m Model) applyEdit(value string) Model {
	labels, applied, err := ApplyField(m.backend, m.cfg, m.selected.ID, m.editField, value, m.me, m.today)
	if err != nil {
		m.status = fmt.Sprintf("edit failed: %v", err)
		m.mode = modeDetail
		return m
	}
	m.selected.Labels = labels
	m.syncIssueLabels(m.selected.ID, labels)
	m.status = fmt.Sprintf("updated %s", applied)
	m.mode = modeDetail
	return m
}

// syncIssueLabels writes labels into every copy of issueID this model
// holds — the (possibly filtered) visible list and the full backing set —
// by matching on ID rather than a captured index, since bubbles/list's own
// "/" filter and the category quick-filter both mean "index into the
// visible list" isn't a stable identity for an issue.
func (m *Model) syncIssueLabels(issueID string, labels []string) {
	for i, li := range m.list.Items() {
		if it, ok := li.(item); ok && it.issue.ID == issueID {
			it.issue.Labels = labels
			m.list.SetItem(i, item{issue: it.issue})
			break
		}
	}
	for i := range m.allIssues {
		if m.allIssues[i].ID == issueID {
			m.allIssues[i].Labels = labels
			break
		}
	}
}

// applyCatFilter narrows the visible list to issues carrying
// catFieldName:value, or clears the filter back to the full set for the
// "(all)" entry.
func (m Model) applyCatFilter(value string) Model {
	if value == "(all)" {
		m.baseIssues = m.allIssues
		m.list.SetItems(m.sortedItems(m.baseIssues))
		m.status = "cleared category filter"
		return m
	}
	m.baseIssues = FilterByField(m.allIssues, catFieldName, value)
	m.list.SetItems(m.sortedItems(m.baseIssues))
	m.status = fmt.Sprintf("filtered by %s:%s (%d issues)", catFieldName, value, len(m.baseIssues))
	return m
}

// sortedItems applies m.sortSpec (if any — a no-op on "") to a copy of
// issues and converts to list.Items. Routed through by every place that
// (re)builds the visible list (filter, category filter, sort itself) so a
// sort persists across a later filter/category-filter change instead of
// being silently lost.
func (m Model) sortedItems(issues []gitbugio.Issue) []list.Item {
	sorted := append([]gitbugio.Issue(nil), issues...)
	board.SortByField(sorted, m.sortSpec)
	return ToItems(sorted)
}

// applySort sorts the currently-visible issues by spec ("field" or "field
// desc", the same syntax board.SortByField/the CLI's --sort already use)
// and remembers it in m.sortSpec so it survives a later filter change.
// board.SortByField is a no-op on "", so an empty spec both clears and
// re-renders in whatever order the current filter naturally returned —
// same "empty clears" convention as the filter/category-filter inputs.
func (m Model) applySort(spec string) Model {
	m.sortSpec = spec
	m.list.SetItems(m.sortedItems(m.baseIssues))
	if spec == "" {
		m.status = "cleared sort"
	} else {
		m.status = fmt.Sprintf("sorted by %s", spec)
	}
	return m
}

// applyStructuredFilter re-filters the full issue set (m.allIssues, not
// whatever's currently visible) against expr, using the same
// internal/filter grammar as ls/view/tui's launch argument — field:value,
// negation, is:open/closed, title globs, free text, all AND'd. Distinct
// from bubbles/list's own "/" fuzzy search. An empty expr clears back to
// the full set, same as the category picker's "(all)".
//
// filter.Parse never fails syntactically (an unrecognized token just
// becomes a free-text condition), so the only failure mode is @me used
// with no current user configured — surfaced as a status line, not a
// crash, same as every other write/parse error in this model.
func (m Model) applyStructuredFilter(expr string) Model {
	if expr == "" {
		m.baseIssues = m.allIssues
		m.list.SetItems(m.sortedItems(m.baseIssues))
		m.list.Title = m.defaultTitle
		m.filterExpr = ""
		m.status = "cleared filter"
		return m
	}

	conditions := filter.Parse(expr)
	if filter.RequiresMe(conditions) && m.me == "" {
		m.status = fmt.Sprintf("filter %q uses @me but no current user configured (--me or GIT_BOARD_ME)", expr)
		return m
	}

	var matched []gitbugio.Issue
	for _, issue := range m.allIssues {
		if filter.Matches(issue, conditions, m.cfg, m.me, m.today) {
			matched = append(matched, issue)
		}
	}
	m.baseIssues = matched
	m.list.SetItems(m.sortedItems(m.baseIssues))
	m.list.Title = expr
	m.filterExpr = expr
	m.status = fmt.Sprintf("filtered: %s (%d issues)", expr, len(matched))
	return m
}

func (m Model) View() string {
	switch m.mode {
	case modeDetail:
		return renderDetail(m.selected, m.cfg, m.status)
	case modeFieldPicker:
		return m.fieldPicker.View()
	case modeValuePicker:
		return m.valuePicker.View()
	case modeValueInput:
		return fmt.Sprintf(
			"Set %s (free text — no fixed options; use +value/-value for multi-cardinality fields):\n\n%s\n\nenter: submit   esc: cancel\n",
			displayFieldName(m.editField), m.valueInput.View())
	case modeCatFilter:
		return m.catPicker.View()
	case modeFilterInput:
		current := "none"
		if m.filterExpr != "" {
			current = m.filterExpr
		}
		return fmt.Sprintf(
			"Current filter: %s\n\n%s\n\nfield:value  -field:value  is:open/closed  title:*glob*  free text  (AND'd)\nenter: apply   empty+enter: clear   esc: cancel\n",
			current, m.filterInput.View())
	case modeSortInput:
		current := "none"
		if m.sortSpec != "" {
			current = m.sortSpec
		}
		return fmt.Sprintf(
			"Current sort: %s\n\n%s\n\n\"field\" (ascending) or \"field desc\" — e.g. prio desc\nenter: apply   empty+enter: clear   esc: cancel\n",
			current, m.sortInput.View())
	default:
		view := m.list.View()
		if m.status != "" {
			view += "\n" + m.status + "\n"
		}
		return view
	}
}

// Run launches the interactive TUI over issues until the user quits.
// backend/cfg/me/today are threaded through to field edits (fields.Apply).
func Run(issues []gitbugio.Issue, title string, backend fields.Backend, cfg *config.Config, me string, today time.Time) error {
	_, err := tea.NewProgram(New(issues, title, backend, cfg, me, today), tea.WithAltScreen()).Run()
	return err
}
