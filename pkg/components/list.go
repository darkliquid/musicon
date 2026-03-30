package components

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
)

// ListItem is a generic single-row list entry.
type ListItem struct {
	Title    string
	Subtitle string
	Meta     string
}

// List is a reusable selectable list widget.
type List struct {
	items      []ListItem
	selected   int
	width      int
	height     int
	focused    bool
	emptyTitle string
	emptyBody  string
}

func NewList() List {
	return List{
		width:      20,
		height:     5,
		emptyTitle: "Nothing here",
		emptyBody:  "No items are available in this panel yet.",
	}
}

func (l *List) SetItems(items []ListItem) {
	l.items = append([]ListItem(nil), items...)
	if len(l.items) == 0 {
		l.selected = 0
		return
	}
	if l.selected >= len(l.items) {
		l.selected = len(l.items) - 1
	}
	if l.selected < 0 {
		l.selected = 0
	}
}

func (l *List) SetSize(width, height int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	l.width = width
	l.height = height
}

func (l *List) SetFocused(focused bool) {
	l.focused = focused
}

func (l *List) SetEmptyState(title, body string) {
	l.emptyTitle = title
	l.emptyBody = body
}

func (l *List) Update(msg tea.Msg) bool {
	keypress, ok := msg.(tea.KeyPressMsg)
	if !ok || len(l.items) == 0 {
		return false
	}

	switch keypress.String() {
	case "up", "k":
		l.Move(-1)
		return true
	case "down", "j":
		l.Move(1)
		return true
	case "home":
		l.selected = 0
		return true
	case "end":
		l.selected = len(l.items) - 1
		return true
	case "pgup":
		l.Move(-max(1, l.height-1))
		return true
	case "pgdown":
		l.Move(max(1, l.height-1))
		return true
	default:
		return false
	}
}

func (l *List) Move(delta int) {
	if len(l.items) == 0 {
		return
	}
	l.selected += delta
	if l.selected < 0 {
		l.selected = 0
	}
	if l.selected >= len(l.items) {
		l.selected = len(l.items) - 1
	}
}

func (l List) SelectedIndex() int {
	return l.selected
}

func (l List) View() string {
	if l.width <= 0 || l.height <= 0 {
		return ""
	}
	if len(l.items) == 0 {
		return RenderEmptyState(l.width, l.height, l.emptyTitle, l.emptyBody)
	}

	start := 0
	if l.selected >= l.height {
		start = l.selected - l.height + 1
	}
	end := start + l.height
	if end > len(l.items) {
		end = len(l.items)
	}

	lines := make([]string, 0, end-start)
	for idx := start; idx < end; idx++ {
		item := l.items[idx]
		label := item.Title
		if item.Subtitle != "" {
			label += " — " + item.Subtitle
		}

		metaWidth := lipgloss.Width(item.Meta)
		leftWidth := l.width
		if metaWidth > 0 {
			leftWidth = l.width - metaWidth - 1
		}
		if leftWidth < 1 {
			leftWidth = 1
		}

		left := lipgloss.NewStyle().Width(leftWidth).Render(truncate(label, leftWidth))
		row := left
		if metaWidth > 0 {
			meta := lipgloss.NewStyle().Width(metaWidth).Align(lipgloss.Right).Foreground(lipgloss.Color("246")).Render(item.Meta)
			row = lipgloss.JoinHorizontal(lipgloss.Left, left, " ", meta)
		}

		prefix := "  "
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
		if idx == l.selected {
			prefix = "▸ "
			style = style.Bold(true)
			if l.focused {
				style = style.Foreground(lipgloss.Color("230")).Background(lipgloss.Color("63"))
			} else {
				style = style.Foreground(lipgloss.Color("255")).Background(lipgloss.Color("238"))
			}
		} else if !l.focused {
			style = style.Faint(true)
		}

		lines = append(lines, style.Width(l.width).Render(truncate(prefix+row, l.width)))
	}

	return lipgloss.NewStyle().Width(l.width).Height(l.height).Render(strings.Join(lines, "\n"))
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	runes := []rune(value)
	if width == 1 {
		return "…"
	}
	if len(runes) >= width {
		return string(runes[:width-1]) + "…"
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
