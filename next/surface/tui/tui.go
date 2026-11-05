// Package tui is an interactive bubbletea front-end over the board, as
// the application gives it (v1's internal/tui, ported over next/app): the
// issues the CLI's ls/view commands render as a flat table. It adds scrolling and search (both native to bubbles/list), a
// detail view that recognizes configured fields, inline field editing, and
// a quick category filter — all layered on bubbles/list rather than
// reinventing picker/filter UI.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/guilbep/git-board/next/domain/issues"
)

// Backend is what the TUI needs from the board: the filter's matches
// among issues, a field's write, and an issue's labels as they are; Close
// is called when the program exits. The application's (Open).
type Backend interface {
	// Match is the issues among among that the filter matches: ls's
	// grammar.
	Match(filter string, among []issues.Issue) ([]issues.Issue, error)
	// Set writes field=value on the issue (set's grammar: +v, -v), and
	// answers its labels after and what was applied.
	Set(id, field, value string) (labels []string, applied string, err error)
	Labels(id string) ([]string, error)
	Close() error
}

// Bridge is the GitHub Project's pull, as "i" previews it and "o" applies
// it (git board project pull): nil when no Project is configured.
type Bridge interface {
	// Read reads the Project (network) and plans its pull into the
	// labels: a line saying the plan.
	Read() (summary string, err error)
	// Apply decides the plan read again, from the issues as they are
	// now, and applies it: the issues whose labels changed.
	Apply() (changed []string, err error)
}

// item adapts a issues.Issue to bubbles/list's DefaultItem interface.
type item struct {
	issue issues.Issue
}

func (i item) Title() string { return i.issue.ShownTitle() }

func (i item) Description() string {
	return fmt.Sprintf("%s  %s  %s", ref(i.issue), i.issue.Status, strings.Join(i.issue.Labels, ", "))
}

// ref is the issue's number (CO2-3075, else #2518), which its shown title
// leaves out, and its short id.
func ref(is issues.Issue) string {
	if is.Number != "" {
		return is.Number + "  " + shortID(string(is.ID))
	}
	for _, f := range is.Forges {
		if f.Number != "" {
			return "#" + f.Number + "  " + shortID(string(is.ID))
		}
	}
	return shortID(string(is.ID))
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
func ToItems(issues []issues.Issue) []list.Item {
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
	selected issues.Issue

	backend Backend
	cfg     *issues.Schema

	fieldPicker list.Model
	valuePicker list.Model
	catPicker   list.Model
	valueInput  textinput.Model
	editField   string

	// filterInput is the "f" structured-filter box (core/filter's
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
	allIssues []issues.Issue

	// baseIssues is the currently narrowed set *before* sorting — set by
	// applyCatFilter/applyStructuredFilter (== allIssues when nothing's
	// narrowed), read by applySort. Keeping this separate from m.list's
	// actual items means clearing the sort (empty spec) restores the true
	// natural order of whatever's currently narrowed, instead of "no-op
	// sorting" an already-sorted list — sortedItems copies before sorting,
	// it never mutates baseIssues itself.
	baseIssues []issues.Issue

	// bridgeRemote is the Project as the last "i" read it, which "o"
	// pulls into the labels: git board project pull, both bound the way
	// git-bug's own termui binds i=Pull/o=Push. nil means nothing read
	// yet (o refuses to apply blind: dry-run first, as every other write
	// path here). "o" decides the plan again from the issues as they are
	// then, so an edit made between the two isn't undone. bridgeBusy
	// guards against a second "i" while a read is in flight.
	bridgeRead bool
	bridgeBusy bool
	// bridge is how "i" reads the Project and "o" applies it; nil: none
	// configured.
	bridge Bridge
}

// New builds a Model over issues. title is shown as the list header (the
// filter expression the caller ran, e.g. "is:open"). backend/cfg/me/today
// are what field edits need to write through fields.Apply.
func New(list_ []issues.Issue, title string, backend Backend, cfg *issues.Schema, bridge Bridge) Model {
	l := list.New(ToItems(list_), list.NewDefaultDelegate(), 0, 0)
	l.Title = title
	l.SetFilteringEnabled(true)

	fp := newPicker(fieldPickerItems(cfg, list_), "Edit field")

	// valuePicker and catPicker are re-populated with real items (and
	// resized to fit them, via newPicker) when the user actually opens
	// them (updateFieldPicker, the "c" key) — but they still need to be
	// real list.Models — not the zero value — from construction: a
	// zero-value list.Model's internal delegate/paginator are nil, and
	// calling SetSize on one panics (confirmed: this exact panic was
	// reported). newPicker(nil, "") builds a real, empty one.
	vp := newPicker(nil, "")
	cp := newPicker(nil, "")

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
		fieldPicker:  fp,
		valuePicker:  vp,
		catPicker:    cp,
		valueInput:   vi,
		filterInput:  fi,
		sortInput:    si,
		defaultTitle: title,
		allIssues:    list_,
		baseIssues:   list_,
		bridge:       bridge,
	}
}

func (m Model) Init() tea.Cmd { return nil }

// bridgePullFinishedMsg is what bridgePullCmd sends back once the GitHub
// Project is read and its pull planned.
type bridgePullFinishedMsg struct {
	summary string
	err     error
}

// bridgePullCmd reads the GitHub Project (network): an ordinary async
// tea.Cmd, so the TUI stays responsive while it runs.
func bridgePullCmd(b Bridge) tea.Cmd {
	return func() tea.Msg {
		s, err := b.Read()
		return bridgePullFinishedMsg{summary: s, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case bridgePullFinishedMsg:
		m.bridgeBusy = false
		if msg.err != nil {
			m.status = fmt.Sprintf("bridge pull failed: %v", msg.err)
			return m, nil
		}
		m.bridgeRead = true
		m.status = msg.summary + " — press o to apply"
		return m, nil

	case tea.WindowSizeMsg:
		// Reserve one line for the status message View() appends below
		// the list. The pickers are deliberately *not* resized to the
		// terminal here — newPicker already sized them to their own
		// content (a handful of short options, not a full screen); we
		// want them to keep looking like a compact dropdown, not stretch
		// to fill whatever terminal they're shown in.
		m.list.SetSize(msg.Width, msg.Height-1)
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
				m.catPicker = newPicker(catFilterItems(f), "Filter by "+DisplayFieldName(catFieldName))
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
		case "i":
			// Mirrors git-bug's own termui: i=Pull, o=Push (bug_table.go).
			// Here: preview git board project pull (a dry run, as the
			// command defaults to), "o" applies it.
			if m.bridgeBusy {
				return m, nil
			}
			if m.bridge == nil {
				m.status = "no GitHub project configured: run `git board project configure` first"
				return m, nil
			}
			m.bridgeBusy = true
			m.status = "pulling the GitHub Project…"
			return m, bridgePullCmd(m.bridge)
		case "o":
			if !m.bridgeRead || m.bridge == nil {
				m.status = "no bridge preview loaded — press i first, then o to apply it"
				return m, nil
			}
			changed, err := m.bridge.Apply()
			if err != nil {
				m.status = fmt.Sprintf("bridge apply failed: %v", err)
				return m, nil
			}
			for _, id := range changed {
				if labels, err := m.backend.Labels(id); err == nil {
					m.syncIssueLabels(id, labels)
				}
			}
			m.bridgeRead = false
			m.status = fmt.Sprintf("bridge applied: %d issue(s) updated", len(changed))
			return m, nil
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
		m.fieldPicker.SetItems(fieldPickerItems(m.cfg, m.allIssues))
		m.mode = modeFieldPicker
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
		m.valuePicker = newPicker(opts, "Set "+DisplayFieldName(fi.name))
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
	labels, applied, err := m.backend.Set(string(m.selected.ID), m.editField, value)
	if err != nil {
		m.status = fmt.Sprintf("edit failed: %v", err)
		m.mode = modeDetail
		return m
	}
	m.selected.Labels = labels
	m.syncIssueLabels(string(m.selected.ID), labels)
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
		if it, ok := li.(item); ok && string(it.issue.ID) == issueID {
			it.issue.Labels = labels
			m.list.SetItem(i, item{issue: it.issue})
			break
		}
	}
	for i := range m.allIssues {
		if string(m.allIssues[i].ID) == issueID {
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
func (m Model) sortedItems(list_ []issues.Issue) []list.Item {
	sorted := append([]issues.Issue(nil), list_...)
	_ = m.cfg.Sort(sorted, m.sortSpec) // a spec that isn't one leaves the order
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
// whatever's currently visible) against expr through board.Match, the
// same grammar and matcher as ls/view/the web page — field:value,
// negation, is:open/closed, title globs, words in titles, descriptions and
// comments, all AND'd. Distinct from bubbles/list's own "/" fuzzy search.
// An empty expr clears back to the full set, same as the category
// picker's "(all)".
//
// The grammar never fails syntactically (an unrecognized token just
// becomes a free-text condition), so the failures are @me used with no
// current user configured, or the full-text index — surfaced as a status
// line, not a crash, same as every other write/parse error in this model.
func (m Model) applyStructuredFilter(expr string) Model {
	if expr == "" {
		m.baseIssues = m.allIssues
		m.list.SetItems(m.sortedItems(m.baseIssues))
		m.list.Title = m.defaultTitle
		m.filterExpr = ""
		m.status = "cleared filter"
		return m
	}

	matched, err := m.backend.Match(expr, m.allIssues)
	if err != nil {
		m.status = fmt.Sprintf("filter %q: %v", expr, err)
		return m
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
			DisplayFieldName(m.editField), m.valueInput.View())
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

// Run launches the interactive TUI over the issues until the user quits.
// It owns backend for the run, and closes it when it returns.
func Run(list_ []issues.Issue, title string, backend Backend, cfg *issues.Schema, bridge Bridge) error {
	final, err := tea.NewProgram(New(list_, title, backend, cfg, bridge), tea.WithAltScreen()).Run()
	if fm, ok := final.(Model); ok && fm.backend != nil {
		_ = fm.backend.Close()
	}
	return err
}
