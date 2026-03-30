package components

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
)

// Input is a small reusable single-line text input widget.
type Input struct {
	placeholder string
	value       string
	width       int
	focused     bool
}

func NewInput(placeholder string) Input {
	return Input{placeholder: placeholder, width: 20}
}

func (i *Input) SetSize(width int) {
	if width < 1 {
		width = 1
	}
	i.width = width
}

func (i *Input) SetFocused(focused bool) {
	i.focused = focused
}

func (i *Input) SetValue(value string) {
	i.value = value
}

func (i Input) Value() string {
	return i.value
}

func (i *Input) Update(msg tea.Msg) bool {
	keypress, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return false
	}

	switch keypress.String() {
	case "backspace":
		if i.value == "" {
			return false
		}
		runes := []rune(i.value)
		i.value = string(runes[:len(runes)-1])
		return true
	case "ctrl+w":
		trimmed := strings.TrimRight(i.value, " ")
		idx := strings.LastIndex(trimmed, " ")
		if idx == -1 {
			i.value = ""
		} else {
			i.value = strings.TrimRight(trimmed[:idx], " ")
		}
		return true
	case "enter", "tab", "shift+tab", "up", "down", "left", "right", "esc":
		return false
	}

	text := keypress.Key().Text
	if text == "" {
		return false
	}

	i.value += text
	return true
}

func (i Input) View() string {
	content := i.value
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	if content == "" {
		content = i.placeholder
		style = style.Faint(true)
	}

	cursor := ""
	if i.focused {
		cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Render("█")
	}

	prefix := lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Render("› ")
	textWidth := i.width - 2
	if textWidth < 1 {
		textWidth = 1
	}

	line := prefix + style.Width(textWidth).Render(trimToWidth(content, textWidth)) + cursor
	return lipgloss.NewStyle().Width(i.width).Render(line)
}

func trimToWidth(value string, width int) string {
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
