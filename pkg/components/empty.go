package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderEmptyState renders a centered title/body pair for panels without data.
func RenderEmptyState(width, height int, title, body string) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	bodyWidth := width
	if bodyWidth > 4 {
		bodyWidth -= 4
	}

	parts := []string{
		lipgloss.NewStyle().Bold(true).Align(lipgloss.Center).Width(width).Render(title),
	}
	if strings.TrimSpace(body) != "" {
		parts = append(parts, lipgloss.NewStyle().Faint(true).Align(lipgloss.Center).Width(bodyWidth).Render(body))
	}

	content := lipgloss.JoinVertical(lipgloss.Center, parts...)
	return lipgloss.NewStyle().Width(width).Height(height).Align(lipgloss.Center, lipgloss.Center).Render(content)
}
