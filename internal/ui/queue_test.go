package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestQueueBrowserShowsQueuedItemsBeforeSearchResults(t *testing.T) {
	screen := newQueueScreen(Services{})
	screen.queueData = []QueueEntry{{ID: "queued-1", Title: "Queued track", Source: "Queue"}}
	screen.resultData = []SearchResult{{ID: "result-1", Title: "Search result", Source: "Local files", Kind: MediaTrack}}
	screen.rebuildBrowser()

	if len(screen.browserData) != 2 {
		t.Fatalf("expected 2 browser rows, got %d", len(screen.browserData))
	}
	if screen.browserData[0].kind != queueRowQueued {
		t.Fatalf("expected queued row first, got %#v", screen.browserData[0])
	}
	if screen.browserData[1].kind != queueRowSearchResult {
		t.Fatalf("expected search result second, got %#v", screen.browserData[1])
	}
}

func TestQueueBrowserAddsSearchResultToQueue(t *testing.T) {
	screen := newQueueScreen(Services{})
	screen.resultData = []SearchResult{{ID: "result-1", Title: "Search result", Source: "Local files", Kind: MediaTrack}}
	screen.rebuildBrowser()

	got := screen.activateSelectedRow()
	if !strings.Contains(got, `Added "Search result"`) {
		t.Fatalf("expected add status, got %q", got)
	}
	if len(screen.queueData) != 1 || screen.queueData[0].Title != "Search result" {
		t.Fatalf("expected added queue entry, got %#v", screen.queueData)
	}
	if len(screen.browserData) == 0 || screen.browserData[0].kind != queueRowQueued {
		t.Fatalf("expected queued row at top after add, got %#v", screen.browserData)
	}
}

func TestQueueBrowserRemovesQueuedItemFromMergedList(t *testing.T) {
	screen := newQueueScreen(Services{})
	screen.queueData = []QueueEntry{{ID: "queued-1", Title: "Queued track", Source: "Queue"}}
	screen.resultData = []SearchResult{{ID: "result-1", Title: "Search result", Source: "Local files", Kind: MediaTrack}}
	screen.rebuildBrowser()

	got := screen.activateSelectedRow()
	if !strings.Contains(got, `Removed "Queued track"`) {
		t.Fatalf("expected remove status, got %q", got)
	}
	if len(screen.queueData) != 0 {
		t.Fatalf("expected queue to be empty, got %#v", screen.queueData)
	}
	if len(screen.browserData) != 1 || screen.browserData[0].kind != queueRowSearchResult {
		t.Fatalf("expected remaining search result row, got %#v", screen.browserData)
	}
}

func TestQueueBrowserBackspaceReturnsFocusToSearchAndClearsQuery(t *testing.T) {
	screen := newQueueScreen(Services{})
	screen.searchInput.SetValue("a")
	screen.resultData = []SearchResult{{ID: "result-1", Title: "Search result", Source: "Local files", Kind: MediaTrack}}
	screen.rebuildBrowser()

	got := screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if got != "Search cleared." {
		t.Fatalf("expected search cleared status, got %q", got)
	}
	if screen.searchInput.Value() != "" {
		t.Fatalf("expected cleared search input, got %q", screen.searchInput.Value())
	}
	if len(screen.resultData) != 0 {
		t.Fatalf("expected cleared results, got %#v", screen.resultData)
	}
}

func TestQueueBrowserArrowKeysBrowseWhileSearchRemainsActive(t *testing.T) {
	screen := newQueueScreen(Services{})
	screen.resultData = []SearchResult{
		{ID: "result-1", Title: "First", Source: "Local files", Kind: MediaTrack},
		{ID: "result-2", Title: "Second", Source: "Local files", Kind: MediaTrack},
	}
	screen.rebuildBrowser()

	screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	got := screen.browser.SelectedIndex()
	if got != 1 {
		t.Fatalf("expected browser selection to move down, got %d", got)
	}
	if !screen.browserData[got].resultMatchesID("result-2") {
		t.Fatalf("expected second result selected, got %#v", screen.browserData[got])
	}
	if screen.searchInput.Value() != "" {
		t.Fatalf("expected search input to remain editable, got %q", screen.searchInput.Value())
	}
}

func TestQueueBrowserEnterTogglesSearchResultQueueMembership(t *testing.T) {
	screen := newQueueScreen(Services{})
	screen.resultData = []SearchResult{{ID: "result-1", Title: "Search result", Source: "Local files", Kind: MediaTrack}}
	screen.rebuildBrowser()

	got := screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !strings.Contains(got, `Added "Search result"`) {
		t.Fatalf("expected add status, got %q", got)
	}
	if len(screen.queueData) != 1 {
		t.Fatalf("expected queued item after first enter, got %#v", screen.queueData)
	}

	got = screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !strings.Contains(got, `Removed "Search result"`) {
		t.Fatalf("expected remove status, got %q", got)
	}
	if len(screen.queueData) != 0 {
		t.Fatalf("expected queue emptied after second enter, got %#v", screen.queueData)
	}
}

func (r queueBrowserRow) resultMatchesID(id string) bool {
	return r.kind == queueRowSearchResult && r.result.ID == id
}
