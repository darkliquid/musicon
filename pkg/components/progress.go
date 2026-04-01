package components

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// RenderProgress draws a simple progress bar with an optional label.
func RenderProgress(width int, ratio float64, label string, theme Theme) string {
	if width <= 0 {
		return ""
	}
	theme = theme.Normalize()
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
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Primary)).Render(strings.Repeat("█", filled))
	empty := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.SurfaceVariant)).Render(strings.Repeat("░", barWidth-filled))

	if label == "" {
		return lipgloss.NewStyle().Width(width).Render(bar + empty)
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, bar+empty, " ", lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Text)).Width(labelWidth).Align(lipgloss.Right).Render(label))
}
