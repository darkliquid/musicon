package components

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	bubblekey "github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

type ListKeyMap struct {
	Up       bubblekey.Binding
	Down     bubblekey.Binding
	Home     bubblekey.Binding
	End      bubblekey.Binding
	PageUp   bubblekey.Binding
	PageDown bubblekey.Binding
}

func DefaultListKeyMap() ListKeyMap {
	return ListKeyMap{
		Up:       bubblekey.NewBinding(bubblekey.WithKeys("up", "k"), bubblekey.WithHelp("up / k", "move up")),
		Down:     bubblekey.NewBinding(bubblekey.WithKeys("down", "j"), bubblekey.WithHelp("down / j", "move down")),
		Home:     bubblekey.NewBinding(bubblekey.WithKeys("home"), bubblekey.WithHelp("home", "jump to top")),
		End:      bubblekey.NewBinding(bubblekey.WithKeys("end"), bubblekey.WithHelp("end", "jump to bottom")),
		PageUp:   bubblekey.NewBinding(bubblekey.WithKeys("pgup"), bubblekey.WithHelp("pgup", "page up")),
		PageDown: bubblekey.NewBinding(bubblekey.WithKeys("pgdown"), bubblekey.WithHelp("pgdown", "page down")),
	}
}

// ListItem is a generic single-row list entry.
type ListItem struct {
	Leading  string
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
	keymap     ListKeyMap
	emptyTitle string
	emptyBody  string
}

func NewList() List {
	return List{
		width:      20,
		height:     5,
		keymap:     DefaultListKeyMap(),
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

func (l *List) SetKeyMap(keymap ListKeyMap) {
	l.keymap = keymap
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

	switch {
	case bubblekey.Matches(keypress, l.keymap.Up):
		l.Move(-1)
		return true
	case bubblekey.Matches(keypress, l.keymap.Down):
		l.Move(1)
		return true
	case bubblekey.Matches(keypress, l.keymap.Home):
		l.selected = 0
		return true
	case bubblekey.Matches(keypress, l.keymap.End):
		l.selected = len(l.items) - 1
		return true
	case bubblekey.Matches(keypress, l.keymap.PageUp):
		l.Move(-max(1, l.height-1))
		return true
	case bubblekey.Matches(keypress, l.keymap.PageDown):
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

func (l *List) SetSelectedIndex(index int) {
	if len(l.items) == 0 {
		l.selected = 0
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(l.items) {
		index = len(l.items) - 1
	}
	l.selected = index
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
		if item.Leading != "" {
			label = item.Leading + " " + label
		}
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
