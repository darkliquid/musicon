package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	bubblekey "github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/darkliquid/musicon/pkg/components"
)

const defaultQueueSearchDebounce = 300 * time.Millisecond

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
	services      Services
	width         int
	height        int
	keymap        QueueKeyMap
	sources       []SourceDescriptor
	sourceIndex   int
	filters       SearchFilters
	searchInput   components.Input
	browser       components.List
	searchFocused bool
	browserData   []queueBrowserRow
	resultData    []SearchResult
	queueData     []QueueEntry
	status        string
	lastQuery     string
	lastSourceID  string
	searchSeq     uint64
	searching     bool
	searchDelay   time.Duration
	cancelSearch  context.CancelFunc
}

type queueStartSearchMsg struct {
	seq      uint64
	query    string
	sourceID string
	filters  SearchFilters
}

type queueSearchResultsMsg struct {
	seq      uint64
	query    string
	sourceID string
	filters  SearchFilters
	results  []SearchResult
	err      error
}

func newQueueScreen(services Services) *queueScreen {
	return newQueueScreenWithKeyMap(services, normalizedKeyMap(KeybindOptions{}).Queue)
}

func newQueueScreenWithKeyMap(services Services, keymap QueueKeyMap) *queueScreen {
	searchInput := components.NewInput("type to search the active source")
	searchInput.SetFocused(true)

	browser := components.NewList()
	browser.SetEmptyState("Queue is empty", "Queued items stay at the top. Type to append matching search results below them.")
	browser.SetKeyMap(keymap.Browser)
	browser.SetFocused(true)

	screen := &queueScreen{
		services:      services,
		keymap:        keymap,
		filters:       DefaultSearchFilters(),
		searchInput:   searchInput,
		browser:       browser,
		searchFocused: true,
		sources:       configuredSources(services),
		status:        "Queue mode ready. Focus search to type, unfocus it to use queue shortcuts.",
		searchDelay:   defaultQueueSearchDebounce,
	}

	screen.syncFocus()
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

// SetSize updates the queue screen's render bounds.
func (q *queueScreen) SetSize(width, height int) {
	q.width = max(1, width)
	q.height = max(1, height)
	q.searchInput.SetSize(max(8, q.width))
	q.resizeBrowser()
}

func (q *queueScreen) resizeBrowser() {
	listHeight := max(3, q.height-5)
	q.browser.SetSize(max(6, q.width), listHeight)
}

// Update handles queue-screen input, debounced searches, and async search results.
func (q *queueScreen) Update(msg tea.Msg) (string, tea.Cmd) {
	switch typed := msg.(type) {
	case queueStartSearchMsg:
		if typed.seq != q.searchSeq || typed.query != q.lastQuery || typed.sourceID != q.lastSourceID || typed.filters != q.filters {
			return "", nil
		}
		search := q.services.Search
		ctx, cancel := context.WithCancel(context.Background())
		q.cancelSearch = cancel
		return "", func() tea.Msg {
			defer cancel()
			results, err := search.Search(ctx, SearchRequest{
				SourceID: typed.sourceID,
				Query:    typed.query,
				Filters:  typed.filters,
			})
			if err != nil && errors.Is(err, context.Canceled) {
				return nil
			}
			return queueSearchResultsMsg{
				seq:      typed.seq,
				query:    typed.query,
				sourceID: typed.sourceID,
				filters:  typed.filters,
				results:  results,
				err:      err,
			}
		}
	case queueSearchResultsMsg:
		if typed.seq != q.searchSeq || typed.query != q.lastQuery || typed.sourceID != q.lastSourceID || typed.filters != q.filters {
			return "", nil
		}
		q.cancelSearch = nil
		q.searching = false
		if typed.err != nil {
			q.resultData = nil
			q.browser.SetEmptyState("Search failed", typed.err.Error())
			q.rebuildBrowser()
			return typed.err.Error(), nil
		}

		filtered := make([]SearchResult, 0, len(typed.results))
		for _, result := range typed.results {
			if q.filters.Matches(result.Kind) {
				filtered = append(filtered, result)
			}
		}
		q.resultData = filtered
		q.rebuildBrowser()
		return "", nil
	}

	keypress, ok := msg.(tea.KeyPressMsg)
	if ok {
		if bubblekey.Matches(keypress, q.keymap.ToggleSearchFocus) {
			q.searchFocused = !q.searchFocused
			q.syncFocus()
			if q.searchFocused {
				return fmt.Sprintf("Search focused. Type freely; %s returns to queue shortcuts.", bindingLabel(q.keymap.ToggleSearchFocus)), nil
			}
			return "Search unfocused. Queue shortcuts are active again.", nil
		}

		if q.searchFocused && shouldEditSearch(keypress) && q.searchInput.Update(msg) {
			cmd := q.refreshResultsCmd()
			if strings.TrimSpace(q.searchInput.Value()) == "" {
				return "Search cleared.", cmd
			}
			return fmt.Sprintf("Searching %s for %q.", q.activeSource().Name, q.searchInput.Value()), cmd
		}

		if !q.searchFocused {
			switch {
			case bubblekey.Matches(keypress, q.keymap.SourcePrev):
				q.sourceIndex--
				if q.sourceIndex < 0 {
					q.sourceIndex = len(q.sources) - 1
				}
				return fmt.Sprintf("Active source: %s", q.activeSource().Name), q.refreshResultsCmd()
			case bubblekey.Matches(keypress, q.keymap.SourceNext):
				q.sourceIndex = (q.sourceIndex + 1) % len(q.sources)
				return fmt.Sprintf("Active source: %s", q.activeSource().Name), q.refreshResultsCmd()
			case bubblekey.Matches(keypress, q.keymap.FilterTracks):
				q.filters.Toggle(MediaTrack)
				return q.filterStatus(), q.refreshResultsCmd()
			case bubblekey.Matches(keypress, q.keymap.FilterStreams):
				q.filters.Toggle(MediaStream)
				return q.filterStatus(), q.refreshResultsCmd()
			case bubblekey.Matches(keypress, q.keymap.FilterPlaylists):
				q.filters.Toggle(MediaPlaylist)
				return q.filterStatus(), q.refreshResultsCmd()
			case bubblekey.Matches(keypress, q.keymap.MoveSelectedUp):
				return q.moveSelectedQueueEntry(-1), nil
			case bubblekey.Matches(keypress, q.keymap.MoveSelectedDown):
				return q.moveSelectedQueueEntry(1), nil
			case bubblekey.Matches(keypress, q.keymap.ClearQueue):
				return q.clearQueue(), nil
			case bubblekey.Matches(keypress, q.keymap.RemoveSelected):
				return q.removeSelectedQueueItem(), nil
			}
		}

		if bubblekey.Matches(keypress, q.keymap.ActivateSelected) {
			return q.activateSelectedRow(), nil
		}
	}

	if ok && !(q.searchFocused && keypress.Key().Text != "") {
		q.browser.Update(msg)
	}

	return "", nil
}

func shouldEditSearch(keypress tea.KeyPressMsg) bool {
	switch keypress.String() {
	case "backspace", "ctrl+w":
		return true
	}
	return keypress.Key().Text != ""
}

// View renders the queue screen inside the active square viewport.
func (q *queueScreen) View() string {
	if q.width <= 0 || q.height <= 0 {
		return ""
	}
	q.syncPlaybackState()
	q.resizeBrowser()

	body := joinLines(
		renderSourceChips(q.sources, q.sourceIndex),
		renderFilterChips(q.filters),
		q.searchInput.View(),
		q.browser.View(),
	)

	return lipgloss.NewStyle().Width(q.width).Height(q.height).Render(body)
}

// HelpView renders the queue-screen help overlay.
func (q *queueScreen) HelpView() string {
	width := min(q.width, 64)
	height := min(q.height, 14)
	return components.RenderPanel(components.PanelOptions{
		Title:    "Queue help",
		Subtitle: "focus search to type, unfocus it for queue shortcuts",
		Width:    width,
		Height:   height,
		Focused:  true,
	}, strings.Join([]string{
		helpLine(q.keymap.ToggleSearchFocus, "focus or unfocus the search input"),
		"type text          update the search query while search is focused",
		helpLinePair(q.keymap.SourcePrev, q.keymap.SourceNext, "switch active source when search is unfocused"),
		helpLinePair(q.keymap.FilterTracks, q.keymap.FilterStreams, "toggle Track and Stream filters when unfocused"),
		helpLine(q.keymap.FilterPlaylists, "toggle Playlist filter when unfocused"),
		helpLinePair(q.keymap.Browser.Up, q.keymap.Browser.Down, "move through queued items and search results"),
		helpLinePair(q.keymap.MoveSelectedUp, q.keymap.MoveSelectedDown, "move the selected queued item up or down"),
		helpLine(q.keymap.ActivateSelected, "toggle the selected item between enqueued and not enqueued"),
		helpLinePair(q.keymap.RemoveSelected, q.keymap.ClearQueue, "remove selected queued item or clear the queue"),
	}, "\n"))
}

func (q *queueScreen) syncFocus() {
	q.searchInput.SetFocused(q.searchFocused)
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

func (q *queueScreen) refreshResultsCmd() tea.Cmd {
	query := strings.TrimSpace(q.searchInput.Value())
	sourceID := q.activeSource().ID
	seq := atomic.AddUint64(&q.searchSeq, 1)
	q.lastQuery = query
	q.lastSourceID = sourceID
	q.cancelRunningSearch()

	if query == "" {
		q.resultData = nil
		q.searching = false
		q.rebuildBrowser()
		return nil
	}
	if q.services.Search == nil {
		q.resultData = nil
		q.searching = false
		q.rebuildBrowser()
		return nil
	}

	q.searching = true
	q.rebuildBrowser()
	filters := q.filters
	if q.searchDelay <= 0 {
		return func() tea.Msg {
			return queueStartSearchMsg{
				seq:      seq,
				query:    query,
				sourceID: sourceID,
				filters:  filters,
			}
		}
	}
	return tea.Tick(q.searchDelay, func(time.Time) tea.Msg {
		return queueStartSearchMsg{
			seq:      seq,
			query:    query,
			sourceID: sourceID,
			filters:  filters,
		}
	})
}

func (q *queueScreen) cancelRunningSearch() {
	if q.cancelSearch != nil {
		q.cancelSearch()
		q.cancelSearch = nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func queueSourceLabel(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "", "queue":
		return ""
	case "local", "local files":
		return "local"
	case "youtube", "youtube music", "youtube-music":
		return "youtube"
	default:
		return strings.ReplaceAll(normalized, " ", "-")
	}
}

func queueRowTitle(source, title string) string {
	if label := queueSourceLabel(source); label != "" {
		return label + ": " + title
	}
	return title
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

func (q *queueScreen) moveSelectedQueueEntry(delta int) string {
	index := q.browser.SelectedIndex()
	if index < 0 || index >= len(q.browserData) {
		return "Select a queued item to move it."
	}

	entry, ok := q.selectedQueueEntry(q.browserData[index])
	if !ok {
		return "Selected item is not currently queued."
	}
	newIndex, moved := q.moveQueueEntry(entry, delta)
	if !moved {
		return fmt.Sprintf("%q is already at the edge of the queue.", entry.Title)
	}
	return fmt.Sprintf("Moved %q to queue position %d.", entry.Title, newIndex+1)
}

func (q *queueScreen) moveQueueEntry(entry QueueEntry, delta int) (int, bool) {
	currentIndex := -1
	for index, queued := range q.queueData {
		if queued.ID == entry.ID {
			currentIndex = index
			break
		}
	}
	if currentIndex == -1 {
		return -1, false
	}

	target := clamp(currentIndex+delta, 0, len(q.queueData)-1)
	if target == currentIndex {
		return currentIndex, false
	}

	if q.services.Queue != nil {
		if err := q.services.Queue.Move(entry.ID, delta); err != nil {
			q.status = err.Error()
			return currentIndex, false
		}
		q.syncQueue()
	} else {
		queued := q.queueData[currentIndex]
		q.queueData = append(q.queueData[:currentIndex], q.queueData[currentIndex+1:]...)
		head := append([]QueueEntry(nil), q.queueData[:target]...)
		head = append(head, queued)
		q.queueData = append(head, q.queueData[target:]...)
		q.rebuildBrowser()
	}

	for rowIndex, row := range q.browserData {
		if row.kind == queueRowQueued && row.queue.ID == entry.ID {
			q.browser.SetSelectedIndex(rowIndex)
			break
		}
	}

	return target, true
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

func (q *queueScreen) syncPlaybackState() {
	if q.services.Playback == nil {
		return
	}
	q.rebuildBrowser()
}

func (q *queueScreen) rebuildBrowser() {
	selectedKey := q.selectedRowKey()
	nowPlayingID := q.nowPlayingID()
	rows := make([]queueBrowserRow, 0, len(q.queueData)+len(q.resultData))
	items := make([]components.ListItem, 0, len(q.queueData)+len(q.resultData))

	for index, entry := range q.queueData {
		rows = append(rows, queueBrowserRow{kind: queueRowQueued, queue: entry})
		meta := fmt.Sprintf("%d", index+1)
		leading := "●"
		if entry.ID == nowPlayingID {
			leading = "▶"
			meta = "playing · " + meta
		}
		if entry.Duration > 0 {
			meta += " · " + formatDuration(entry.Duration)
		}
		items = append(items, components.ListItem{
			Leading:  leading,
			Title:    queueRowTitle(entry.Source, entry.Title),
			Subtitle: entry.Subtitle,
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
			Title:    queueRowTitle(result.Source, result.Title),
			Subtitle: result.Subtitle,
			Meta:     meta,
		})
	}

	q.browserData = rows
	q.browser.SetItems(items)
	if selectedKey != "" {
		for index, row := range q.browserData {
			if row.key() == selectedKey {
				q.browser.SetSelectedIndex(index)
				break
			}
		}
	}
	switch {
	case q.searching:
		q.browser.SetEmptyState("Searching…", "The source is working in the background. Input and quit keys stay responsive.")
	case len(q.queueData) == 0 && strings.TrimSpace(q.searchInput.Value()) == "":
		q.browser.SetEmptyState("Queue is empty", "Type to search. Queued items will stay pinned above search results.")
	case len(q.queueData) > 0 && len(q.resultData) == 0:
		q.browser.SetEmptyState("Queued items only", "Type to append matching search results below the queued items.")
	default:
		q.browser.SetEmptyState("No matching music", "Queued items stay visible, but the current search returned no additional results.")
	}
}

func (q *queueScreen) browserSubtitle() string {
	return fmt.Sprintf("%d queued · %d results · type to search, ctrl+k/j to reorder, enter to toggle", len(q.queueData), len(q.resultData))
}

func (q *queueScreen) findQueuedEntryByID(id string) (QueueEntry, bool) {
	for _, entry := range q.queueData {
		if entry.ID == id {
			return entry, true
		}
	}
	return QueueEntry{}, false
}

func (q *queueScreen) nowPlayingID() string {
	if q.services.Playback == nil {
		return ""
	}
	snapshot := q.services.Playback.Snapshot()
	if snapshot.Track != nil && strings.TrimSpace(snapshot.Track.ID) != "" {
		return snapshot.Track.ID
	}
	return ""
}

func (q *queueScreen) selectedRowKey() string {
	index := q.browser.SelectedIndex()
	if index < 0 || index >= len(q.browserData) {
		return ""
	}
	return q.browserData[index].key()
}

func (q *queueScreen) selectedQueueEntry(row queueBrowserRow) (QueueEntry, bool) {
	switch row.kind {
	case queueRowQueued:
		return row.queue, true
	case queueRowSearchResult:
		return q.findQueuedEntryByID(row.result.ID)
	default:
		return QueueEntry{}, false
	}
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

func (r queueBrowserRow) key() string {
	switch r.kind {
	case queueRowQueued:
		return "queue:" + r.queue.ID
	case queueRowSearchResult:
		return "result:" + r.result.ID
	default:
		return ""
	}
}
