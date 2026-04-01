package components

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestInputUpdateReportsValueChange(t *testing.T) {
	input := NewInput("placeholder")
	input.SetFocused(true)

	changed := input.Update(tea.KeyPressMsg{Text: "a", Code: 'a'})
	if !changed {
		t.Fatal("expected value change on text input")
	}
	if input.Value() != "a" {
		t.Fatalf("expected value %q, got %q", "a", input.Value())
	}
}

func TestInputUpdateReportsNoChangeForNonTextKeys(t *testing.T) {
	input := NewInput("placeholder")
	input.SetFocused(true)
	input.SetValue("hello")

	changed := input.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if changed {
		t.Fatal("expected no value change for up arrow")
	}
}

func TestInputSetValueAndValue(t *testing.T) {
	input := NewInput("placeholder")
	input.SetValue("test")
	if input.Value() != "test" {
		t.Fatalf("expected value %q, got %q", "test", input.Value())
	}
}
