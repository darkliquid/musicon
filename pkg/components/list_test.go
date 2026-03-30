package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	bubblekey "github.com/charmbracelet/bubbles/key"
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
		Up:       bubblekey.NewBinding(bubblekey.WithKeys("p"), bubblekey.WithHelp("p", "move up")),
		Down:     bubblekey.NewBinding(bubblekey.WithKeys("n"), bubblekey.WithHelp("n", "move down")),
		Home:     bubblekey.NewBinding(bubblekey.WithKeys("h"), bubblekey.WithHelp("h", "jump to top")),
		End:      bubblekey.NewBinding(bubblekey.WithKeys("e"), bubblekey.WithHelp("e", "jump to bottom")),
		PageUp:   bubblekey.NewBinding(bubblekey.WithKeys("u"), bubblekey.WithHelp("u", "page up")),
		PageDown: bubblekey.NewBinding(bubblekey.WithKeys("d"), bubblekey.WithHelp("d", "page down")),
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
