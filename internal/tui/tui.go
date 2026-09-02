// Package tui is an interactive bubbletea front-end over the same
// internal/gitbugio issue data the CLI's ls/view commands render as a flat
// table. It adds scrolling and search (both native to bubbles/list) plus a
// detail view, without duplicating any filter/config/gitbug logic.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

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

// Model is the bubbletea model: a list of issues, with an optional detail
// view (full label list) pushed on top when an item is selected.
type Model struct {
	list     list.Model
	detail   bool
	selected gitbugio.Issue
}

// New builds a Model over issues. title is shown as the list header (the
// filter expression the caller ran, e.g. "is:open").
func New(issues []gitbugio.Issue, title string) Model {
	l := list.New(ToItems(issues), list.NewDefaultDelegate(), 0, 0)
	l.Title = title
	l.SetFilteringEnabled(true)
	return Model{list: l}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		if m.detail {
			switch msg.String() {
			case "esc", "q":
				m.detail = false
			}
			return m, nil
		}
		// While the filter input is being typed into, every key (including
		// "q"/"esc") belongs to the list's own filter handling — don't
		// intercept.
		if m.list.FilterState() != list.Filtering {
			switch msg.String() {
			case "enter":
				if it, ok := m.list.SelectedItem().(item); ok {
					m.selected = it.issue
					m.detail = true
				}
				return m, nil
			case "q", "ctrl+c":
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.detail {
		return renderDetail(m.selected)
	}
	return m.list.View()
}

func renderDetail(issue gitbugio.Issue) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n%s  (%s)\n\nLabels:\n", issue.Title, shortID(issue.ID), issue.Status)
	if len(issue.Labels) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, l := range issue.Labels {
		fmt.Fprintf(&b, "  - %s\n", l)
	}
	b.WriteString("\nesc/q: back\n")
	return b.String()
}

// Run launches the interactive TUI over issues until the user quits.
func Run(issues []gitbugio.Issue, title string) error {
	_, err := tea.NewProgram(New(issues, title), tea.WithAltScreen()).Run()
	return err
}
