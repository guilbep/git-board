package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// compactDelegate is a one-line-per-item bubbles/list delegate for the
// field/value/category pickers. bubbles/list's own NewDefaultDelegate is
// built for browsing a big issue list (title + description, two lines
// per row) — using it for "pick one of 4 status values" renders a full
// second screen of two-line rows for what's really a short dropdown.
// This is the "reuse bubbles/list, don't add a dropdown dependency"
// answer: same library, a ~20-line delegate, one line per item.
type compactDelegate struct{}

func (compactDelegate) Height() int                         { return 1 }
func (compactDelegate) Spacing() int                        { return 0 }
func (compactDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

var (
	pickerNormalStyle   = lipgloss.NewStyle().PaddingLeft(2)
	pickerSelectedStyle = lipgloss.NewStyle().PaddingLeft(0).Bold(true).Foreground(lipgloss.Color("212"))
)

func (compactDelegate) Render(w io.Writer, m list.Model, index int, it list.Item) {
	titler, ok := it.(interface{ Title() string })
	if !ok {
		return
	}
	title := titler.Title()
	if index == m.Index() {
		fmt.Fprint(w, pickerSelectedStyle.Render("> "+title))
		return
	}
	fmt.Fprint(w, pickerNormalStyle.Render(title))
}

// newPicker builds a compact, chrome-light bubbles/list for the
// field/value/category pickers: one line per item, no status bar or
// pagination dots (noise for a handful of options), sized to its own
// content rather than the full terminal — the list renders at the top
// of the screen with blank space below instead of stretching two-line
// DefaultDelegate rows to fill it.
func newPicker(items []list.Item, title string) list.Model {
	l := list.New(items, compactDelegate{}, pickerWidth(items), len(items)+2)
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	return l
}

// pickerWidth sizes to the longest item's title (plus room for the
// selection marker/padding), capped at a sane minimum/maximum so a very
// short or very long option list still looks like a tidy box rather than
// a sliver or an overflow.
func pickerWidth(items []list.Item) int {
	const minWidth, maxWidth = 24, 60
	width := minWidth
	for _, it := range items {
		if titler, ok := it.(interface{ Title() string }); ok {
			if w := len(titler.Title()) + 4; w > width {
				width = w
			}
		}
	}
	if width > maxWidth {
		width = maxWidth
	}
	return width
}
