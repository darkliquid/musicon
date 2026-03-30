package components

import (
	"strings"
	"testing"
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
