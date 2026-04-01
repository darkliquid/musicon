package components

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// Input is a single-line text input widget backed by the Bubbles textinput.
type Input struct {
	model textinput.Model
}

// NewInput constructs a single-line input with the supplied placeholder.
func NewInput(placeholder string) Input {
	m := textinput.New()
	m.Placeholder = placeholder
	m.Prompt = "› "

	// Disable suggestion-related bindings that conflict with host screen
	// navigation (tab, up/down arrows).
	km := m.KeyMap
	km.AcceptSuggestion = key.NewBinding()
	km.NextSuggestion = key.NewBinding()
	km.PrevSuggestion = key.NewBinding()
	m.KeyMap = km

	return Input{model: m}
}

// SetSize updates the rendered width of the input field.
func (i *Input) SetSize(width int) {
	if width < 1 {
		width = 1
	}
	i.model.SetWidth(width)
}

// SetFocused toggles the input cursor and focus styling.
func (i *Input) SetFocused(focused bool) {
	if focused {
		i.model.Focus()
	} else {
		i.model.Blur()
	}
}

// SetValue replaces the current input text.
func (i *Input) SetValue(value string) {
	i.model.SetValue(value)
}

// Value returns the current input text.
func (i Input) Value() string {
	return i.model.Value()
}

// Update handles text editing and reports whether the value changed.
func (i *Input) Update(msg tea.Msg) bool {
	old := i.model.Value()
	i.model, _ = i.model.Update(msg)
	return i.model.Value() != old
}

// View renders the input field.
func (i Input) View() string {
	return i.model.View()
}
