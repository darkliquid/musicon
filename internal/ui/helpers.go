package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
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

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSeconds := int(d.Seconds())
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func pill(label string, active bool) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Padding(0, 1)
	if active {
		style = style.Foreground(lipgloss.Color("230")).Background(lipgloss.Color("63")).Bold(true)
	}
	return style.Render(label)
}

func joinLines(lines ...string) string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func overlayRows(base, overlay string, width, height int) string {
	baseLines := strings.Split(lipgloss.NewStyle().Width(width).Height(height).Render(base), "\n")
	overlayLines := strings.Split(lipgloss.NewStyle().Width(width).Height(height).Render(overlay), "\n")
	limit := min(len(baseLines), len(overlayLines))
	for index := 0; index < limit; index++ {
		if strings.TrimSpace(overlayLines[index]) == "" {
			continue
		}
		baseLines[index] = overlayLines[index]
	}
	return strings.Join(baseLines, "\n")
}

func centeredOverlay(body, overlay string, width, height int) string {
	return overlayRows(body, lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, overlay), width, height)
}

func topOverlay(body, overlay string, width, height int) string {
	return overlayRows(body, lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, overlay), width, height)
}

func bottomOverlay(body, overlay string, width, height int) string {
	return overlayRows(body, lipgloss.Place(width, height, lipgloss.Center, lipgloss.Bottom, overlay), width, height)
}
