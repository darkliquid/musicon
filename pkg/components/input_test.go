package components

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestInputViewRespectsConfiguredWidthWhenFocused(t *testing.T) {
	input := NewInput("placeholder")
	input.SetSize(12)
	input.SetFocused(true)
	input.SetValue("abcdef")

	got := input.View()
	if width := lipgloss.Width(got); width != 12 {
		t.Fatalf("expected width 12, got %d from %q", width, got)
	}
}
