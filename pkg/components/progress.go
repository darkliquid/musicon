package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderProgress draws a simple progress bar with an optional label.
func RenderProgress(width int, ratio float64, label string) string {
	if width <= 0 {
		return ""
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}

	labelWidth := lipgloss.Width(label)
	barWidth := width
	if labelWidth > 0 {
		barWidth = width - labelWidth - 1
	}
	if barWidth < 4 {
		barWidth = width
		label = ""
		labelWidth = 0
	}

	filled := int(ratio * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Render(strings.Repeat("█", filled))
	empty := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(strings.Repeat("░", barWidth-filled))

	if label == "" {
		return lipgloss.NewStyle().Width(width).Render(bar + empty)
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, bar+empty, " ", lipgloss.NewStyle().Width(labelWidth).Align(lipgloss.Right).Render(label))
}
