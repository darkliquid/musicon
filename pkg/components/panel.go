package components

import "github.com/charmbracelet/lipgloss"

// PanelOptions controls the generic panel renderer.
type PanelOptions struct {
	Title    string
	Subtitle string
	Width    int
	Height   int
	Focused  bool
}

// RenderPanel draws a bordered panel with a title line and bounded content.
func RenderPanel(opts PanelOptions, body string) string {
	if opts.Width <= 0 || opts.Height <= 0 {
		return ""
	}

	innerWidth := opts.Width - 2
	innerHeight := opts.Height - 2
	if innerWidth < 1 {
		innerWidth = 1
	}
	if innerHeight < 1 {
		innerHeight = 1
	}

	borderColor := lipgloss.Color("240")
	titleColor := lipgloss.Color("252")
	if opts.Focused {
		borderColor = lipgloss.Color("63")
		titleColor = lipgloss.Color("230")
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(titleColor)
	subtitleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	title := titleStyle.Render(opts.Title)
	if opts.Subtitle != "" {
		title = lipgloss.JoinHorizontal(lipgloss.Left, title, "  ", subtitleStyle.Render(opts.Subtitle))
	}

	bodyHeight := innerHeight - 1
	if bodyHeight < 0 {
		bodyHeight = 0
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.NewStyle().Width(innerWidth).Render(title),
		lipgloss.NewStyle().Width(innerWidth).Height(bodyHeight).Render(body),
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(innerWidth).
		Height(innerHeight).
		Render(content)
}
