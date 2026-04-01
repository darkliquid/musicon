package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/key"
)

func TestListViewRendersLeadingMarker(t *testing.T) {
	list := NewList()
	list.SetSize(20, 3)
	list.SetItems([]ListItem{{Leading: "●", Title: "Queued track"}})

	got := list.View()
	if !strings.Contains(got, "● Queued track") {
		t.Fatalf("expected leading marker in list view, got %q", got)
	}
}

func TestListUpdateUsesConfigurableKeyMap(t *testing.T) {
	list := NewList()
	list.SetItems([]ListItem{{Title: "First"}, {Title: "Second"}})
	list.SetKeyMap(ListKeyMap{
		Up:       key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "move up")),
		Down:     key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "move down")),
		Home:     key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "jump to top")),
		End:      key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "jump to bottom")),
		PageUp:   key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "page up")),
		PageDown: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "page down")),
	})

	if !list.Update(tea.KeyPressMsg(tea.Key{Text: "n"})) {
		t.Fatal("expected custom down key to move selection")
	}
	if list.SelectedIndex() != 1 {
		t.Fatalf("expected selection to move to 1, got %d", list.SelectedIndex())
	}
	if list.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})) {
		t.Fatal("expected default down key to stop matching after override")
	}
}
