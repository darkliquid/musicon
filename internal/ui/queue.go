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
	focusResults
	focusQueue
)

func (f queueFocus) String() string {
	switch f {
	case focusResults:
		return "results focus"
	case focusQueue:
		return "queue focus"
	default:
		return "search focus"
	}
}

type queueScreen struct {
	services     Services
	width        int
	height       int
	focus        queueFocus
	sources      []SourceDescriptor
	sourceIndex  int
	filters      SearchFilters
	searchInput  components.Input
	results      components.List
	queue        components.List
	resultData   []SearchResult
	queueData    []QueueEntry
	status       string
	lastQuery    string
	lastSourceID string
}

func newQueueScreen(services Services) *queueScreen {
	searchInput := components.NewInput("type to search the active source")
	searchInput.SetFocused(true)

	results := components.NewList()
	results.SetEmptyState("No results yet", "Type a query to ask the active source for matching music.")
	results.SetFocused(false)

	queue := components.NewList()
	queue.SetEmptyState("Queue is empty", "Add search results to build the active listening queue.")

	screen := &queueScreen{
		services:    services,
		focus:       focusSearch,
		filters:     DefaultSearchFilters(),
		searchInput: searchInput,
		results:     results,
		queue:       queue,
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
	q.resizeLists()
}

func (q *queueScreen) resizeLists() {
	topHeight := max(8, (q.height*3)/5)
	if topHeight >= q.height {
		topHeight = max(4, q.height-4)
	}
	bottomHeight := max(4, q.height-topHeight-1)

	resultsHeight := max(3, topHeight-7)
	q.results.SetSize(max(6, q.width-4), resultsHeight)
	q.queue.SetSize(max(6, q.width-4), max(2, bottomHeight-2))
}

func (q *queueScreen) Update(msg tea.Msg) string {
	keypress, ok := msg.(tea.KeyPressMsg)
	if ok {
		switch keypress.String() {
		case "ctrl+l":
			q.focus = (q.focus + 1) % 3
			q.syncFocus()
			return fmt.Sprintf("Focused %s.", q.focus.String())
		case "ctrl+h":
			q.focus = (q.focus + 2) % 3
			q.syncFocus()
			return fmt.Sprintf("Focused %s.", q.focus.String())
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
			switch q.focus {
			case focusSearch:
				if strings.TrimSpace(q.searchInput.Value()) == "" {
					return "Enter a search query first."
				}
				q.focus = focusResults
				q.syncFocus()
				q.refreshResults()
				return fmt.Sprintf("Showing results for %q.", q.searchInput.Value())
			case focusResults:
				return q.addSelectedResult()
			case focusQueue:
				return q.removeSelectedQueueItem()
			}
		case "ctrl+x":
			return q.clearQueue()
		case "x":
			if q.focus == focusQueue {
				return q.removeSelectedQueueItem()
			}
		}
	}

	if q.focus == focusSearch && q.searchInput.Update(msg) {
		q.refreshResults()
		if strings.TrimSpace(q.searchInput.Value()) == "" {
			return "Search cleared."
		}
		return fmt.Sprintf("Searching %s for %q.", q.activeSource().Name, q.searchInput.Value())
	}

	switch q.focus {
	case focusResults:
		q.results.Update(msg)
	case focusQueue:
		q.queue.Update(msg)
	}

	return ""
}

func (q *queueScreen) View() string {
	if q.width <= 0 || q.height <= 0 {
		return ""
	}
	q.resizeLists()

	topHeight := max(8, (q.height*3)/5)
	if topHeight >= q.height {
		topHeight = max(4, q.height-4)
	}
	bottomHeight := max(4, q.height-topHeight-1)
	searchBody := joinLines(
		renderSourceChips(q.sources, q.sourceIndex),
		renderFilterChips(q.filters),
		q.searchInput.View(),
		q.results.View(),
	)

	queueBody := q.queue.View()
	if len(q.queueData) > 0 {
		queueBody = lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(fmt.Sprintf("%d item(s) queued", len(q.queueData))),
			q.queue.View(),
		)
	}

	top := components.RenderPanel(components.PanelOptions{
		Title:    "Discover",
		Subtitle: q.focus.String(),
		Width:    q.width,
		Height:   topHeight,
		Focused:  q.focus != focusQueue,
	}, searchBody)

	bottom := components.RenderPanel(components.PanelOptions{
		Title:    "Queue",
		Subtitle: "enter/x acts on selected row",
		Width:    q.width,
		Height:   bottomHeight,
		Focused:  q.focus == focusQueue,
	}, queueBody)

	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}

func (q *queueScreen) HelpView() string {
	return components.RenderPanel(components.PanelOptions{
		Title:    "Queue help",
		Subtitle: "dedicated queue-management mode",
		Width:    q.width,
		Height:   q.height,
		Focused:  true,
	}, strings.Join([]string{
		"ctrl+l / ctrl+h  change focus between search, results, and queue",
		"[ / ]            switch active source",
		"1 / 2 / 3        toggle Track / Stream / Playlist filters",
		"type text         update the active search query",
		"enter             search, add selected result, or remove selected queue item",
		"x / ctrl+x        remove selected queue item / clear the entire queue",
		"tab               switch to playback mode",
		"?                 toggle this help view",
	}, "\n"))
}

func (q *queueScreen) syncFocus() {
	q.searchInput.SetFocused(q.focus == focusSearch)
	q.results.SetFocused(q.focus == focusResults)
	q.queue.SetFocused(q.focus == focusQueue)
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
		q.results.SetItems(nil)
		q.lastQuery = ""
		q.lastSourceID = sourceID
		return
	}
	if q.services.Search == nil {
		q.resultData = nil
		q.results.SetItems(nil)
		q.results.SetEmptyState("Search backend unavailable", "Connect a source-search implementation to populate results for this query.")
		q.lastQuery = query
		q.lastSourceID = sourceID
		return
	}

	results, err := q.services.Search.Search(SearchRequest{SourceID: sourceID, Query: query, Filters: q.filters})
	if err != nil {
		q.resultData = nil
		q.results.SetItems(nil)
		q.results.SetEmptyState("Search failed", err.Error())
		q.status = err.Error()
		q.lastQuery = query
		q.lastSourceID = sourceID
		return
	}

	filtered := make([]SearchResult, 0, len(results))
	for _, result := range results {
		if q.filters.Matches(result.Kind) {
			filtered = append(filtered, result)
		}
	}
	q.resultData = filtered
	q.results.SetItems(searchListItems(filtered))
	q.results.SetEmptyState("No matching music", "The current backend returned no results for this query and filter combination.")
	q.lastQuery = query
	q.lastSourceID = sourceID
}

func searchListItems(results []SearchResult) []components.ListItem {
	items := make([]components.ListItem, 0, len(results))
	for _, result := range results {
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
	return items
}

func queueListItems(entries []QueueEntry) []components.ListItem {
	items := make([]components.ListItem, 0, len(entries))
	for index, entry := range entries {
		meta := fmt.Sprintf("%d", index+1)
		if entry.Duration > 0 {
			meta += " · " + formatDuration(entry.Duration)
		}
		items = append(items, components.ListItem{
			Title:    entry.Title,
			Subtitle: firstNonEmpty(entry.Subtitle, entry.Source),
			Meta:     meta,
		})
	}
	return items
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (q *queueScreen) addSelectedResult() string {
	index := q.results.SelectedIndex()
	if index < 0 || index >= len(q.resultData) {
		return "Select a result to add it to the queue."
	}
	result := q.resultData[index]
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
		})
		q.queue.SetItems(queueListItems(q.queueData))
	}
	return fmt.Sprintf("Added %q to the queue.", result.Title)
}

func (q *queueScreen) removeSelectedQueueItem() string {
	index := q.queue.SelectedIndex()
	if index < 0 || index >= len(q.queueData) {
		return "Select a queued item to remove it."
	}
	entry := q.queueData[index]
	if q.services.Queue != nil {
		if err := q.services.Queue.Remove(entry.ID); err != nil {
			return err.Error()
		}
		q.syncQueue()
	} else {
		q.queueData = append(q.queueData[:index], q.queueData[index+1:]...)
		q.queue.SetItems(queueListItems(q.queueData))
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
		q.queue.SetItems(nil)
	}
	return "Cleared the queue."
}

func (q *queueScreen) syncQueue() {
	if q.services.Queue != nil {
		q.queueData = q.services.Queue.Snapshot()
	}
	q.queue.SetItems(queueListItems(q.queueData))
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
