package components

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
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

func TestMarqueeLeavesShortStringsUntouched(t *testing.T) {
	got := marquee("short", 10, 3)
	if got != "short" {
		t.Fatalf("expected short string unchanged, got %q", got)
	}
}

func TestMarqueeScrollsLongStringsAcrossSteps(t *testing.T) {
	first := marquee("abcdefgh", 5, 0)
	next := marquee("abcdefgh", 5, 1)
	if first == next {
		t.Fatalf("expected marquee output to change across steps, got %q", first)
	}
	if !strings.Contains(first, "abcde") {
		t.Fatalf("expected first marquee frame to start at the beginning, got %q", first)
	}
	if !strings.Contains(next, "bcdef") {
		t.Fatalf("expected later marquee frame to advance, got %q", next)
	}
}

func TestAnchoredMarqueeKeepsPrefixStatic(t *testing.T) {
	first := anchoredMarquee("local: ", "abcdefgh", 10, 0)
	next := anchoredMarquee("local: ", "abcdefgh", 10, 1)
	if !strings.Contains(first, "local: ") {
		t.Fatalf("expected prefix in first frame, got %q", first)
	}
	if !strings.Contains(next, "local: ") {
		t.Fatalf("expected prefix in later frame, got %q", next)
	}
	if first == next {
		t.Fatalf("expected scrolling suffix to change while prefix stays fixed, got %q", first)
	}
}

func TestListViewKeepsTrailingMetaVisible(t *testing.T) {
	list := NewList()
	list.SetSize(24, 3)
	list.SetFocused(true)
	list.SetItems([]ListItem{{Title: "A very long queue entry title", Meta: "3:45"}})

	got := list.View()
	if !strings.Contains(got, "3:45") {
		t.Fatalf("expected meta to remain visible, got %q", got)
	}
}

func TestListViewKeepsLeadingPrefixAnchoredDuringMarquee(t *testing.T) {
	list := NewList()
	list.SetSize(20, 3)
	list.SetFocused(true)
	list.SetItems([]ListItem{{Leading: "local:", Title: "A very long queue entry title"}})

	got := list.View()
	if !strings.Contains(got, "local: ") {
		t.Fatalf("expected anchored leading prefix in marquee view, got %q", got)
	}
}
