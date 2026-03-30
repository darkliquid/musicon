package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/darkliquid/musicon/pkg/components"
)

type queueFocus int

const (
	focusSearch queueFocus = iota
	focusBrowser
)

func (f queueFocus) String() string {
	switch f {
	case focusBrowser:
		return "browser focus"
	default:
		return "search focus"
	}
}

type queueBrowserRowKind int

const (
	queueRowQueued queueBrowserRowKind = iota
	queueRowSearchResult
)

type queueBrowserRow struct {
	kind   queueBrowserRowKind
	queue  QueueEntry
	result SearchResult
}

type queueScreen struct {
	services     Services
	width        int
	height       int
	sources      []SourceDescriptor
	sourceIndex  int
	filters      SearchFilters
	searchInput  components.Input
	browser      components.List
	browserData  []queueBrowserRow
	resultData   []SearchResult
	queueData    []QueueEntry
	status       string
	lastQuery    string
	lastSourceID string
}

func newQueueScreen(services Services) *queueScreen {
	searchInput := components.NewInput("type to search the active source")
	searchInput.SetFocused(true)

	browser := components.NewList()
	browser.SetEmptyState("Queue is empty", "Queued items stay at the top. Type to append matching search results below them.")
	browser.SetFocused(false)

	screen := &queueScreen{
		services:    services,
		filters:     DefaultSearchFilters(),
		searchInput: searchInput,
		browser:     browser,
		sources:     configuredSources(services),
		status:      "Queue mode ready. Type to search and use tab to switch modes.",
	}

	screen.syncQueue()
	return screen
}

func configuredSources(services Services) []SourceDescriptor {
	if services.Search == nil {
		return []SourceDescriptor{{ID: "all", Name: "All sources", Description: "Search is available when a backend is connected."}}
	}
	sources := services.Search.Sources()
	if len(sources) == 0 {
		return []SourceDescriptor{{ID: "all", Name: "All sources", Description: "The backend did not expose source choices yet."}}
	}
	return sources
}

func (q *queueScreen) SetSize(width, height int) {
	q.width = max(1, width)
	q.height = max(1, height)
	q.searchInput.SetSize(max(8, q.width-8))
	q.resizeBrowser()
}

func (q *queueScreen) resizeBrowser() {
	listHeight := max(3, q.height-7)
	q.browser.SetSize(max(6, q.width-4), listHeight)
}

func (q *queueScreen) Update(msg tea.Msg) string {
	keypress, ok := msg.(tea.KeyPressMsg)
	if ok {
		if shouldEditSearch(keypress) && q.searchInput.Update(msg) {
			q.refreshResults()
			if strings.TrimSpace(q.searchInput.Value()) == "" {
				return "Search cleared."
			}
			return fmt.Sprintf("Searching %s for %q.", q.activeSource().Name, q.searchInput.Value())
		}

		switch keypress.String() {
		case "ctrl+l":
			return "Search and browser stay active together."
		case "ctrl+h":
			return "Search and browser stay active together."
		case "[":
			q.sourceIndex--
			if q.sourceIndex < 0 {
				q.sourceIndex = len(q.sources) - 1
			}
			q.refreshResults()
			return fmt.Sprintf("Active source: %s", q.activeSource().Name)
		case "]":
			q.sourceIndex = (q.sourceIndex + 1) % len(q.sources)
			q.refreshResults()
			return fmt.Sprintf("Active source: %s", q.activeSource().Name)
		case "1":
			q.filters.Toggle(MediaTrack)
			q.refreshResults()
			return q.filterStatus()
		case "2":
			q.filters.Toggle(MediaStream)
			q.refreshResults()
			return q.filterStatus()
		case "3":
			q.filters.Toggle(MediaPlaylist)
			q.refreshResults()
			return q.filterStatus()
		case "enter":
			return q.activateSelectedRow()
		case "ctrl+x":
			return q.clearQueue()
		case "x":
			return q.removeSelectedQueueItem()
		}
	}

	if ok {
		q.browser.Update(msg)
	}

	return ""
}

func shouldEditSearch(keypress tea.KeyPressMsg) bool {
	switch keypress.String() {
	case "backspace", "ctrl+w":
		return true
	}
	return keypress.Key().Text != ""
}

func (q *queueScreen) View() string {
	if q.width <= 0 || q.height <= 0 {
		return ""
	}
	q.resizeBrowser()

	body := joinLines(
		renderSourceChips(q.sources, q.sourceIndex),
		renderFilterChips(q.filters),
		q.searchInput.View(),
		q.browser.View(),
	)

	return components.RenderPanel(components.PanelOptions{
		Title:    "Queue browser",
		Subtitle: q.browserSubtitle(),
		Width:    q.width,
		Height:   q.height,
		Focused:  true,
	}, body)
}

func (q *queueScreen) HelpView() string {
	return components.RenderPanel(components.PanelOptions{
		Title:    "Queue help",
		Subtitle: "dedicated queue-management mode",
		Width:    q.width,
		Height:   q.height,
		Focused:  true,
	}, strings.Join([]string{
		"ctrl+l / ctrl+h  search and browser stay active together",
		"[ / ]            switch active source",
		"1 / 2 / 3        toggle Track / Stream / Playlist filters",
		"type text         update the active search query while keeping browser selection live",
		"up / down         move through queued items and current search results",
		"enter             toggle the selected item between enqueued and not enqueued",
		"x / ctrl+x        remove selected queued item / clear the entire queue",
		"tab               switch to playback mode",
		"?                 toggle this help view",
	}, "\n"))
}

func (q *queueScreen) syncFocus() {
	q.searchInput.SetFocused(true)
	q.browser.SetFocused(true)
}

func (q *queueScreen) activeSource() SourceDescriptor {
	if len(q.sources) == 0 {
		return SourceDescriptor{ID: "all", Name: "All sources"}
	}
	if q.sourceIndex < 0 || q.sourceIndex >= len(q.sources) {
		q.sourceIndex = 0
	}
	return q.sources[q.sourceIndex]
}

func (q *queueScreen) refreshResults() {
	query := strings.TrimSpace(q.searchInput.Value())
	sourceID := q.activeSource().ID
	if query == "" {
		q.resultData = nil
		q.lastQuery = ""
		q.lastSourceID = sourceID
		q.rebuildBrowser()
		return
	}
	if q.services.Search == nil {
		q.resultData = nil
		q.lastQuery = query
		q.lastSourceID = sourceID
		q.rebuildBrowser()
		return
	}

	results, err := q.services.Search.Search(SearchRequest{SourceID: sourceID, Query: query, Filters: q.filters})
	if err != nil {
		q.resultData = nil
		q.browser.SetEmptyState("Search failed", err.Error())
		q.status = err.Error()
		q.lastQuery = query
		q.lastSourceID = sourceID
		q.rebuildBrowser()
		return
	}

	filtered := make([]SearchResult, 0, len(results))
	for _, result := range results {
		if q.filters.Matches(result.Kind) {
			filtered = append(filtered, result)
		}
	}
	q.resultData = filtered
	q.lastQuery = query
	q.lastSourceID = sourceID
	q.rebuildBrowser()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (q *queueScreen) activateSelectedRow() string {
	index := q.browser.SelectedIndex()
	if index < 0 || index >= len(q.browserData) {
		return "Select a row to act on it."
	}

	row := q.browserData[index]
	switch row.kind {
	case queueRowQueued:
		return q.removeQueueEntry(row.queue)
	case queueRowSearchResult:
		if entry, ok := q.findQueuedEntryByID(row.result.ID); ok {
			return q.removeQueueEntry(entry)
		}
		return q.addSearchResult(row.result)
	default:
		return "Select a row to act on it."
	}
}

func (q *queueScreen) addSearchResult(result SearchResult) string {
	if q.services.Queue != nil {
		if err := q.services.Queue.Add(result); err != nil {
			return err.Error()
		}
		q.syncQueue()
	} else {
		q.queueData = append(q.queueData, QueueEntry{
			ID:       result.ID,
			Title:    result.Title,
			Subtitle: result.Subtitle,
			Source:   result.Source,
			Kind:     result.Kind,
			Duration: result.Duration,
			Artwork:  result.Artwork,
		})
		q.rebuildBrowser()
	}
	return fmt.Sprintf("Added %q to the queue.", result.Title)
}

func (q *queueScreen) removeSelectedQueueItem() string {
	index := q.browser.SelectedIndex()
	if index < 0 || index >= len(q.browserData) {
		return "Select a queued item to remove it."
	}
	row := q.browserData[index]
	switch row.kind {
	case queueRowQueued:
		return q.removeQueueEntry(row.queue)
	case queueRowSearchResult:
		entry, ok := q.findQueuedEntryByID(row.result.ID)
		if !ok {
			return "Selected item is not currently queued."
		}
		return q.removeQueueEntry(entry)
	default:
		return "Select a queued item to remove it."
	}
}

func (q *queueScreen) removeQueueEntry(entry QueueEntry) string {
	if q.services.Queue != nil {
		if err := q.services.Queue.Remove(entry.ID); err != nil {
			return err.Error()
		}
		q.syncQueue()
	} else {
		for index, queued := range q.queueData {
			if queued.ID != entry.ID {
				continue
			}
			q.queueData = append(q.queueData[:index], q.queueData[index+1:]...)
			break
		}
		q.rebuildBrowser()
	}
	return fmt.Sprintf("Removed %q from the queue.", entry.Title)
}

func (q *queueScreen) clearQueue() string {
	if len(q.queueData) == 0 {
		return "Queue is already empty."
	}
	if q.services.Queue != nil {
		if err := q.services.Queue.Clear(); err != nil {
			return err.Error()
		}
		q.syncQueue()
	} else {
		q.queueData = nil
		q.rebuildBrowser()
	}
	return "Cleared the queue."
}

func (q *queueScreen) syncQueue() {
	if q.services.Queue != nil {
		q.queueData = q.services.Queue.Snapshot()
	}
	q.rebuildBrowser()
}

func (q *queueScreen) rebuildBrowser() {
	rows := make([]queueBrowserRow, 0, len(q.queueData)+len(q.resultData))
	items := make([]components.ListItem, 0, len(q.queueData)+len(q.resultData))

	for index, entry := range q.queueData {
		rows = append(rows, queueBrowserRow{kind: queueRowQueued, queue: entry})
		meta := fmt.Sprintf("%d", index+1)
		if entry.Duration > 0 {
			meta += " · " + formatDuration(entry.Duration)
		}
		items = append(items, components.ListItem{
			Leading:  "●",
			Title:    entry.Title,
			Subtitle: firstNonEmpty(entry.Subtitle, entry.Source),
			Meta:     meta,
		})
	}

	for _, result := range q.resultData {
		rows = append(rows, queueBrowserRow{kind: queueRowSearchResult, result: result})
		meta := result.Kind.String()
		if result.Duration > 0 {
			meta += " · " + formatDuration(result.Duration)
		}
		items = append(items, components.ListItem{
			Title:    result.Title,
			Subtitle: firstNonEmpty(result.Subtitle, result.Source),
			Meta:     meta,
		})
	}

	q.browserData = rows
	q.browser.SetItems(items)
	switch {
	case len(q.queueData) == 0 && strings.TrimSpace(q.searchInput.Value()) == "":
		q.browser.SetEmptyState("Queue is empty", "Type to search. Queued items will stay pinned above search results.")
	case len(q.queueData) > 0 && len(q.resultData) == 0:
		q.browser.SetEmptyState("Queued items only", "Type to append matching search results below the queued items.")
	default:
		q.browser.SetEmptyState("No matching music", "Queued items stay visible, but the current search returned no additional results.")
	}
}

func (q *queueScreen) browserSubtitle() string {
	return fmt.Sprintf("%d queued · %d results · type to search, arrows to browse, enter to toggle", len(q.queueData), len(q.resultData))
}

func (q *queueScreen) findQueuedEntryByID(id string) (QueueEntry, bool) {
	for _, entry := range q.queueData {
		if entry.ID == id {
			return entry, true
		}
	}
	return QueueEntry{}, false
}

func (q *queueScreen) filterStatus() string {
	parts := make([]string, 0, 3)
	if q.filters.Tracks {
		parts = append(parts, "tracks")
	}
	if q.filters.Streams {
		parts = append(parts, "streams")
	}
	if q.filters.Playlists {
		parts = append(parts, "playlists")
	}
	return fmt.Sprintf("Filters: %s", strings.Join(parts, ", "))
}

func renderSourceChips(sources []SourceDescriptor, active int) string {
	chips := make([]string, 0, len(sources))
	for idx, source := range sources {
		chips = append(chips, pill(source.Name, idx == active))
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, chips...)
}

func renderFilterChips(filters SearchFilters) string {
	return lipgloss.JoinHorizontal(lipgloss.Left,
		pill("1 Track", filters.Tracks),
		pill("2 Stream", filters.Streams),
		pill("3 Playlist", filters.Playlists),
	)
}
