package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/darkliquid/musicon/pkg/components"
)

const defaultQueueSearchDebounce = 300 * time.Millisecond

type queueBrowserRowKind int

const (
	queueRowQueued queueBrowserRowKind = iota
	queueRowQueueGroup
	queueRowSearchResult
	queueRowExpandedCollectionTrack
)

type queueBrowserRow struct {
	kind    queueBrowserRowKind
	queue   QueueEntry
	result  SearchResult
	groupID string
	childOf string
}

type queueFocus int

const (
	focusSources queueFocus = iota
	focusModes
	focusSearch
	focusBrowser
)

type queueScreen struct {
	services              Services
	width                 int
	height                int
	keymap                QueueKeyMap
	sources               []SourceDescriptor
	sourceIndex           int
	searchMode            SearchMode
	artistFilter          SearchArtistFilter
	searchInput           components.Input
	browser               components.List
	focus                 queueFocus
	browserData           []queueBrowserRow
	resultData            []SearchResult
	queueData             []QueueEntry
	status                string
	lastQuery             string
	lastSourceID          string
	searchSeq             uint64
	searching             bool
	searchDelay           time.Duration
	cancelSearch          context.CancelFunc
	expandedCollections   map[string][]SearchResult
	expandedCollectionIDs map[string]bool
	expandingCollectionID string
	cancelExpand          context.CancelFunc
}

type queueStartSearchMsg struct {
	seq      uint64
	query    string
	sourceID string
	request  SearchRequest
}

type queueSearchResultsMsg struct {
	seq      uint64
	query    string
	sourceID string
	request  SearchRequest
	results  []SearchResult
	err      error
}

type queueExpandResultsMsg struct {
	resultID string
	results  []SearchResult
	err      error
}

type queuePlaybackStartedMsg struct {
	status string
	err    error
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
		services:              services,
		keymap:                keymap,
		searchInput:           searchInput,
		browser:               browser,
		focus:                 focusSearch,
		sources:               configuredSources(services),
		status:                "Queue mode ready. Use arrow keys to navigate between zones.",
		searchDelay:           defaultQueueSearchDebounce,
		expandedCollections:   make(map[string][]SearchResult),
		expandedCollectionIDs: make(map[string]bool),
	}

	screen.searchMode = screen.defaultSearchModeForSource(screen.activeSource())
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
	listHeight := max(3, q.height-6)
	q.browser.SetSize(max(6, q.width), listHeight)
}

// Update handles queue-screen input, debounced searches, collection expansion, and async search results.
func (q *queueScreen) Update(msg tea.Msg) (string, tea.Cmd) {
	switch typed := msg.(type) {
	case queueStartSearchMsg:
		if typed.seq != q.searchSeq || typed.query != q.lastQuery || typed.sourceID != q.lastSourceID || typed.request != q.activeSearchRequest() {
			return "", nil
		}
		search := q.services.Search
		ctx, cancel := context.WithCancel(context.Background())
		q.cancelSearch = cancel
		return "", func() tea.Msg {
			defer cancel()
			results, err := search.Search(ctx, typed.request)
			if err != nil && errors.Is(err, context.Canceled) {
				return nil
			}
			return queueSearchResultsMsg{seq: typed.seq, query: typed.query, sourceID: typed.sourceID, request: typed.request, results: results, err: err}
		}
	case queueSearchResultsMsg:
		if typed.seq != q.searchSeq || typed.query != q.lastQuery || typed.sourceID != q.lastSourceID || typed.request != q.activeSearchRequest() {
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
			if !q.matchesActiveSearchKind(result) {
				continue
			}
			filtered = append(filtered, result)
		}
		q.resultData = filtered
		q.rebuildBrowser()
		return "", nil
	case queueExpandResultsMsg:
		if typed.resultID != q.expandingCollectionID {
			return "", nil
		}
		q.expandingCollectionID = ""
		q.cancelExpand = nil
		if typed.err != nil {
			return typed.err.Error(), nil
		}
		q.expandedCollections[typed.resultID] = typed.results
		q.expandedCollectionIDs[typed.resultID] = true
		q.rebuildBrowser()
		return "", nil
	case queuePlaybackStartedMsg:
		if typed.err != nil {
			return typed.err.Error(), nil
		}
		return typed.status, nil
	}

	keypress, ok := msg.(tea.KeyPressMsg)
	if ok {
		if key.Matches(keypress, q.keymap.ToggleSearchFocus) {
			if q.focus == focusSearch {
				q.setFocus(focusBrowser)
				return "Browser focused. Use up/down to return to search.", nil
			}
			q.setFocus(focusSearch)
			return fmt.Sprintf("Search focused. Type freely; %s jumps back to the browser.", bindingLabel(q.keymap.ToggleSearchFocus)), nil
		}

		switch q.focus {
		case focusSources:
			switch keypress.String() {
			case "left":
				return q.changeSource(-1), q.refreshResultsCmd()
			case "right":
				return q.changeSource(1), q.refreshResultsCmd()
			case "down":
				if q.hasFocusedSearchModes() {
					q.setFocus(focusModes)
				} else {
					q.setFocus(focusSearch)
				}
				return "", nil
			}
		case focusModes:
			switch keypress.String() {
			case "left":
				return q.cycleSearchModeBackward(), q.refreshResultsCmd()
			case "right":
				return q.cycleSearchMode(), q.refreshResultsCmd()
			case "up":
				q.setFocus(focusSources)
				return "", nil
			case "down":
				q.setFocus(focusSearch)
				return "", nil
			}
		case focusSearch:
			switch keypress.String() {
			case "up":
				if q.hasFocusedSearchModes() {
					q.setFocus(focusModes)
				} else {
					q.setFocus(focusSources)
				}
				return "", nil
			case "down":
				q.setFocus(focusBrowser)
				return "", nil
			}

			if q.searchInput.Update(msg) {
				cmd := q.refreshResultsCmd()
				if strings.TrimSpace(q.searchInput.Value()) == "" {
					return "Search cleared.", cmd
				}
				return fmt.Sprintf("Searching %s for %q.", q.activeSource().Name, q.searchInput.Value()), cmd
			}
			// If the textinput consumed the key (text editing or cursor
			// movement), stop here so the browser doesn't also act on it.
			if isTextInputKey(keypress) {
				return "", nil
			}
		case focusBrowser:
			if keypress.String() == "up" && q.browser.SelectedIndex() == 0 {
				q.setFocus(focusSearch)
				return "", nil
			}
		}

		if q.focus != focusSearch {
			switch {
			case key.Matches(keypress, q.keymap.SourcePrev):
				return q.changeSource(-1), q.refreshResultsCmd()
			case key.Matches(keypress, q.keymap.SourceNext):
				return q.changeSource(1), q.refreshResultsCmd()
			case q.hasFocusedSearchModes() && key.Matches(keypress, q.keymap.ModeSongs):
				return q.selectVisibleSearchMode(0), q.refreshResultsCmd()
			case q.hasFocusedSearchModes() && key.Matches(keypress, q.keymap.ModeArtists):
				return q.selectVisibleSearchMode(1), q.refreshResultsCmd()
			case q.hasFocusedSearchModes() && key.Matches(keypress, q.keymap.ModeAlbums):
				return q.selectVisibleSearchMode(2), q.refreshResultsCmd()
			case q.hasFocusedSearchModes() && key.Matches(keypress, q.keymap.ModePlaylists):
				return q.selectVisibleSearchMode(3), q.refreshResultsCmd()
			case q.hasFocusedSearchModes() && key.Matches(keypress, q.keymap.CycleSearchMode):
				return q.cycleSearchMode(), q.refreshResultsCmd()
			case key.Matches(keypress, q.keymap.MoveSelectedUp):
				return q.moveSelectedQueueEntry(-1), nil
			case key.Matches(keypress, q.keymap.MoveSelectedDown):
				return q.moveSelectedQueueEntry(1), nil
			case key.Matches(keypress, q.keymap.ClearQueue):
				return q.clearQueue(), nil
			case key.Matches(keypress, q.keymap.RemoveSelected):
				return q.removeSelectedQueueItem(), nil
			case key.Matches(keypress, q.keymap.ExpandSelected):
				return q.expandSelectedCollection()
			}
		}

		if key.Matches(keypress, q.keymap.ActivateSelected) {
			return q.activateSelectedRow()
		}
	}

	if ok && q.focus == focusBrowser {
		q.browser.Update(msg)
	}

	return "", nil
}

// isTextInputKey reports whether a keypress is handled by the textinput
// widget (text entry, deletion, or cursor movement) so the queue screen
// can prevent it from also reaching the browser list.
func isTextInputKey(keypress tea.KeyPressMsg) bool {
	switch keypress.String() {
	case "backspace", "ctrl+h", "delete", "ctrl+d",
		"ctrl+w", "alt+backspace", "alt+delete", "alt+d",
		"ctrl+k", "ctrl+u",
		"left", "right", "home", "end",
		"ctrl+f", "ctrl+b", "ctrl+a", "ctrl+e",
		"alt+left", "alt+right", "ctrl+left", "ctrl+right",
		"alt+f", "alt+b",
		"ctrl+v":
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
		renderSourceChips(q.sources, q.sourceIndex, q.focus == focusSources),
		renderSearchModeChips(q.visibleSearchModes(), q.activeSearchMode(), q.keymap, q.focus == focusModes),
		renderArtistFilterChip(q.artistFilter),
		q.searchInput.View(),
		q.browser.View(),
	)

	return lipgloss.NewStyle().Width(q.width).Height(q.height).Render(body)
}

// HelpView renders the queue-screen help overlay.
func (q *queueScreen) HelpView() string {
	width := min(q.width, 72)
	height := min(q.height, 16)
	lines := []string{
		helpLine(q.keymap.ToggleSearchFocus, "jump between search and browser"),
		"up/down            move focus between sources, modes, search, and results",
		"left/right         switch source or search mode when those chips are focused",
		"type text          update the search query while search is focused",
		helpLinePair(q.keymap.SourcePrev, q.keymap.SourceNext, "switch active source from any non-search focus"),
		helpLine(q.keymap.CycleSearchMode, "cycle the visible search-kind chips from any non-search focus"),
		helpLinePair(q.keymap.ModeSongs, q.keymap.ModeArtists, "select the first or second visible search kind"),
		helpLinePair(q.keymap.ModeAlbums, q.keymap.ModePlaylists, "select the third or fourth visible search kind"),
		helpLine(q.keymap.ExpandSelected, "expand or collapse the selected album or playlist"),
		helpLinePair(q.keymap.Browser.Up, q.keymap.Browser.Down, "move through queued items and search results when the browser is focused"),
		helpLinePair(q.keymap.MoveSelectedUp, q.keymap.MoveSelectedDown, "move the selected queued item up or down"),
		helpLine(q.keymap.ActivateSelected, "queue, unqueue, filter by artist, or add a whole collection"),
		helpLinePair(q.keymap.RemoveSelected, q.keymap.ClearQueue, "remove selected queued item or clear the queue"),
	}
	return components.RenderPanel(components.PanelOptions{Title: "Queue help", Subtitle: "arrow keys move focus; search stays ready without a required shortcut", Width: width, Height: height, Focused: true}, strings.Join(lines, "\n"))
}

func (q *queueScreen) syncFocus() {
	q.searchInput.SetFocused(q.focus == focusSearch)
	q.browser.SetFocused(q.focus == focusBrowser)
}

func (q *queueScreen) setFocus(focus queueFocus) {
	if focus == focusModes && !q.hasFocusedSearchModes() {
		focus = focusSources
	}
	q.focus = focus
	q.syncFocus()
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

func (q *queueScreen) activeSearchMode() SearchMode {
	mode := q.searchMode
	if mode == SearchModeDefault {
		mode = q.defaultSearchModeForSource(q.activeSource())
	}
	return mode
}

func (q *queueScreen) defaultSearchModeForSource(source SourceDescriptor) SearchMode {
	if len(source.SearchModes) == 0 {
		return SearchModeDefault
	}
	if source.DefaultMode != "" {
		return source.DefaultMode
	}
	return source.SearchModes[0].ID
}

func (q *queueScreen) filtersForActiveSearchMode() SearchFilters {
	switch q.activeSearchMode() {
	case SearchModeAll:
		return SearchFilters{Tracks: true, Streams: true, Playlists: true}
	case SearchModeStreams:
		return SearchFilters{Tracks: false, Streams: true, Playlists: false}
	case SearchModePlaylists:
		return SearchFilters{Tracks: false, Streams: false, Playlists: true}
	default:
		return SearchFilters{Tracks: true, Streams: false, Playlists: false}
	}
}

func (q *queueScreen) visibleSearchModes() []SearchModeDescriptor {
	return append([]SearchModeDescriptor(nil), q.activeSource().SearchModes...)
}

func (q *queueScreen) activeSearchRequest() SearchRequest {
	request := SearchRequest{
		SourceID: q.activeSource().ID,
		Query:    strings.TrimSpace(q.searchInput.Value()),
		Filters:  q.filtersForActiveSearchMode(),
		Mode:     q.activeSearchMode(),
	}
	if q.activeSearchMode() == SearchModeSongs {
		request.ArtistFilter = q.artistFilter
	}
	return request
}

func (q *queueScreen) refreshResultsCmd() tea.Cmd {
	query := strings.TrimSpace(q.searchInput.Value())
	sourceID := q.activeSource().ID
	request := q.activeSearchRequest()
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
	if q.searchDelay <= 0 {
		return func() tea.Msg {
			return queueStartSearchMsg{seq: seq, query: query, sourceID: sourceID, request: request}
		}
	}
	return tea.Tick(q.searchDelay, func(time.Time) tea.Msg {
		return queueStartSearchMsg{seq: seq, query: query, sourceID: sourceID, request: request}
	})
}

func (q *queueScreen) cancelRunningSearch() {
	if q.cancelSearch != nil {
		q.cancelSearch()
		q.cancelSearch = nil
	}
}

func (q *queueScreen) cancelRunningExpand() {
	if q.cancelExpand != nil {
		q.cancelExpand()
		q.cancelExpand = nil
	}
	q.expandingCollectionID = ""
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
	return title
}

func queueRowLeading(source string, leading ...string) string {
	parts := make([]string, 0, len(leading)+1)
	for _, value := range leading {
		if strings.TrimSpace(value) == "" {
			continue
		}
		parts = append(parts, value)
	}
	if label := queueSourceLabel(source); label != "" {
		parts = append(parts, label+":")
	}
	return strings.Join(parts, " ")
}

func (q *queueScreen) activateSelectedRow() (string, tea.Cmd) {
	index := q.browser.SelectedIndex()
	if index < 0 || index >= len(q.browserData) {
		return "Select a row to act on it.", nil
	}

	row := q.browserData[index]
	switch row.kind {
	case queueRowQueued:
		return q.removeQueueEntry(row.queue), nil
	case queueRowQueueGroup:
		return q.removeQueueGroup(row.groupID), nil
	case queueRowSearchResult, queueRowExpandedCollectionTrack:
		return q.activateSearchResult(row.result)
	default:
		return "Select a row to act on it.", nil
	}
}

func (q *queueScreen) activateSearchResult(result SearchResult) (string, tea.Cmd) {
	switch result.Kind {
	case MediaArtist:
		return q.applyArtistFilter(result.ArtistFilter), q.refreshResultsCmd()
	case MediaAlbum, MediaPlaylist:
		return q.addSearchResult(result)
	default:
		if entry, ok := q.findQueuedEntryByID(result.ID); ok {
			return q.removeQueueEntry(entry), nil
		}
		return q.addSearchResult(result)
	}
}

func (q *queueScreen) applyArtistFilter(filter SearchArtistFilter) string {
	q.artistFilter = filter
	q.searchMode = SearchModeSongs
	return fmt.Sprintf("Artist filter: %s", filter.Name)
}

func (q *queueScreen) addSearchResult(result SearchResult) (string, tea.Cmd) {
	wasEmpty := len(q.queueData) == 0
	if isCollectionResult(result) {
		children := q.expandedCollections[result.ID]
		if len(children) > 0 {
			result.CollectionItems = make([]QueueEntry, 0, len(children))
			for _, child := range children {
				result.CollectionItems = append(result.CollectionItems, QueueEntry{ID: child.ID, Title: child.Title, Subtitle: child.Subtitle, Source: child.Source, Kind: child.Kind, Duration: child.Duration, Artwork: child.Artwork})
			}
		}
	}

	if q.services.Queue != nil {
		if err := q.services.Queue.Add(result); err != nil {
			return err.Error(), nil
		}
		q.syncQueue()
	} else {
		entries := []QueueEntry{{ID: result.ID, Title: result.Title, Subtitle: result.Subtitle, Source: result.Source, Kind: result.Kind, Duration: result.Duration, Artwork: result.Artwork}}
		if isCollectionResult(result) {
			entries = append([]QueueEntry(nil), result.CollectionItems...)
			for index := range entries {
				entries[index].GroupID = result.ID
				entries[index].GroupTitle = result.Title
				entries[index].GroupKind = result.Kind
				entries[index].GroupIndex = index
				entries[index].GroupSize = len(entries)
			}
		}
		q.queueData = append(q.queueData, entries...)
		q.rebuildBrowser()
	}
	if wasEmpty && result.Kind == MediaStream && q.services.Playback != nil {
		return fmt.Sprintf("Added %q to the queue. Starting playback.", result.Title), q.startQueuedPlaybackCmd(result.Title)
	}
	if isCollectionResult(result) {
		return fmt.Sprintf("Added %q to the queue.", result.Title), nil
	}
	return fmt.Sprintf("Added %q to the queue.", result.Title), nil
}

func (q *queueScreen) startQueuedPlaybackCmd(title string) tea.Cmd {
	playback := q.services.Playback
	if playback == nil {
		return nil
	}
	return func() tea.Msg {
		if err := playback.TogglePause(); err != nil {
			return queuePlaybackStartedMsg{err: fmt.Errorf("start playback for %q: %w", title, err)}
		}
		return queuePlaybackStartedMsg{status: fmt.Sprintf("Added %q to the queue and started playback.", title)}
	}
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
	case queueRowQueueGroup:
		return q.removeQueueGroup(row.groupID)
	case queueRowSearchResult, queueRowExpandedCollectionTrack:
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

func (q *queueScreen) removeQueueGroup(groupID string) string {
	groupTitle := q.groupTitle(groupID)
	if groupTitle == "" {
		groupTitle = groupID
	}
	if q.services.Queue != nil {
		if err := q.services.Queue.RemoveGroup(groupID); err != nil {
			return err.Error()
		}
		q.syncQueue()
	} else {
		filtered := q.queueData[:0]
		for _, entry := range q.queueData {
			if entry.GroupID != groupID {
				filtered = append(filtered, entry)
			}
		}
		q.queueData = append([]QueueEntry(nil), filtered...)
		q.rebuildBrowser()
	}
	return fmt.Sprintf("Removed %q from the queue.", groupTitle)
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
	seenGroups := make(map[string]struct{})

	for index, entry := range q.queueData {
		if entry.GroupID != "" {
			if _, seen := seenGroups[entry.GroupID]; !seen {
				seenGroups[entry.GroupID] = struct{}{}
				rows = append(rows, queueBrowserRow{kind: queueRowQueueGroup, groupID: entry.GroupID})
				items = append(items, components.ListItem{
					Leading:  queueRowLeading(entry.Source, "◆"),
					Title:    queueRowTitle(entry.Source, entry.GroupTitle),
					Subtitle: strings.ToLower(entry.GroupKind.String()) + " collection",
					Meta:     fmt.Sprintf("%d songs", entry.GroupSize),
				})
			}
		}

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
		if entry.GroupID != "" {
			meta = fmt.Sprintf("%d/%d · %s", entry.GroupIndex+1, entry.GroupSize, meta)
		}
		items = append(items, components.ListItem{
			Leading:  queueRowLeading(entry.Source, leading),
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
		if isCollectionResult(result) {
			meta = collectionMeta(result, q.expandingCollectionID == result.ID, q.expandedCollectionIDs[result.ID], q.keymap)
		}
		items = append(items, components.ListItem{
			Leading:  queueRowLeading(result.Source),
			Title:    queueRowTitle(result.Source, result.Title),
			Subtitle: result.Subtitle,
			Meta:     meta,
		})

		if q.expandedCollectionIDs[result.ID] {
			for _, child := range q.expandedCollections[result.ID] {
				rows = append(rows, queueBrowserRow{kind: queueRowExpandedCollectionTrack, result: child, childOf: result.ID})
				meta := child.Kind.String()
				if child.Duration > 0 {
					meta += " · " + formatDuration(child.Duration)
				}
				items = append(items, components.ListItem{
					Leading:  queueRowLeading(child.Source, "↳"),
					Title:    queueRowTitle(child.Source, child.Title),
					Subtitle: child.Subtitle,
					Meta:     meta,
				})
			}
		}
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

func collectionMeta(result SearchResult, expanding, expanded bool, keymap QueueKeyMap) string {
	parts := []string{result.Kind.String()}
	if result.CollectionCount > 0 {
		parts = append(parts, fmt.Sprintf("%d songs", result.CollectionCount))
	}
	parts = append(parts, bindingLabel(keymap.ActivateSelected)+" adds all")
	if expanding {
		parts = append(parts, "loading…")
	} else if expanded {
		parts = append(parts, bindingLabel(keymap.ExpandSelected)+" collapses")
	} else {
		parts = append(parts, bindingLabel(keymap.ExpandSelected)+" expands")
	}
	return strings.Join(parts, " · ")
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
	case queueRowSearchResult, queueRowExpandedCollectionTrack:
		return q.findQueuedEntryByID(row.result.ID)
	default:
		return QueueEntry{}, false
	}
}

func renderSourceChips(sources []SourceDescriptor, active int, focused bool) string {
	chips := make([]string, 0, len(sources))
	for idx, source := range sources {
		chips = append(chips, pill(source.Name, idx == active))
	}
	indicator := "  "
	if focused {
		indicator = "▸ "
	}
	return indicator + lipgloss.JoinHorizontal(lipgloss.Left, chips...)
}

func renderSearchModeChips(modes []SearchModeDescriptor, active SearchMode, keymap QueueKeyMap, focused bool) string {
	if len(modes) == 0 {
		return ""
	}
	chips := make([]string, 0, len(modes))
	for index, mode := range modes {
		chips = append(chips, pill(searchModeChipLabel(index, mode.Name, keymap), mode.ID == active))
	}
	indicator := "  "
	if focused {
		indicator = "▸ "
	}
	return indicator + lipgloss.JoinHorizontal(lipgloss.Left, chips...)
}

func renderArtistFilterChip(filter SearchArtistFilter) string {
	if strings.TrimSpace(filter.Name) == "" {
		return ""
	}
	return pill("artist: "+filter.Name, true)
}

func searchModeChipLabel(index int, name string, keymap QueueKeyMap) string {
	switch index {
	case 0:
		return bindingLabel(keymap.ModeSongs) + " " + name
	case 1:
		return bindingLabel(keymap.ModeArtists) + " " + name
	case 2:
		return bindingLabel(keymap.ModeAlbums) + " " + name
	case 3:
		return bindingLabel(keymap.ModePlaylists) + " " + name
	default:
		return name
	}
}

func (r queueBrowserRow) key() string {
	switch r.kind {
	case queueRowQueued:
		return "queue:" + r.queue.ID
	case queueRowQueueGroup:
		return "group:" + r.groupID
	case queueRowExpandedCollectionTrack:
		return "child:" + r.childOf + ":" + r.result.ID
	case queueRowSearchResult:
		return "result:" + r.result.ID
	default:
		return ""
	}
}

func (q *queueScreen) cycleSearchMode() string {
	return q.cycleSearchModeBy(1)
}

func (q *queueScreen) cycleSearchModeBackward() string {
	return q.cycleSearchModeBy(-1)
}

func (q *queueScreen) cycleSearchModeBy(delta int) string {
	modes := q.visibleSearchModes()
	if len(modes) == 0 {
		return "Active source uses a single search mode."
	}
	current := q.activeSearchMode()
	for index, mode := range modes {
		if mode.ID == current {
			next := modes[(index+delta+len(modes))%len(modes)]
			return q.setSearchMode(next.ID)
		}
	}
	return q.setSearchMode(modes[0].ID)
}

func (q *queueScreen) changeSource(delta int) string {
	if len(q.sources) == 0 {
		return "No sources are available."
	}
	q.sourceIndex = (q.sourceIndex + delta + len(q.sources)) % len(q.sources)
	q.resetSourceScopedState()
	return fmt.Sprintf("Active source: %s", q.activeSource().Name)
}

func (q *queueScreen) selectVisibleSearchMode(index int) string {
	modes := q.visibleSearchModes()
	if index < 0 || index >= len(modes) {
		return "That search kind is not available for the active source."
	}
	return q.setSearchMode(modes[index].ID)
}

func (q *queueScreen) setSearchMode(mode SearchMode) string {
	if !q.sourceSupportsMode(mode) {
		return "Active source does not expose that search mode."
	}
	q.searchMode = mode
	if mode != SearchModeSongs {
		q.artistFilter = SearchArtistFilter{}
	}
	return fmt.Sprintf("Search mode: %s", mode.String())
}

func (q *queueScreen) sourceSupportsMode(mode SearchMode) bool {
	for _, descriptor := range q.visibleSearchModes() {
		if descriptor.ID == mode {
			return true
		}
	}
	return false
}

func (q *queueScreen) hasFocusedSearchModes() bool {
	return len(q.visibleSearchModes()) > 0
}

func (q *queueScreen) matchesActiveSearchKind(result SearchResult) bool {
	switch q.activeSearchMode() {
	case SearchModeAll:
		return result.Kind == MediaTrack || result.Kind == MediaStream || result.Kind == MediaPlaylist
	case SearchModeTracks, SearchModeSongs:
		return result.Kind == MediaTrack
	case SearchModeStreams:
		return result.Kind == MediaStream
	case SearchModePlaylists:
		return result.Kind == MediaPlaylist
	case SearchModeArtists:
		return result.Kind == MediaArtist
	case SearchModeAlbums:
		return result.Kind == MediaAlbum
	default:
		return true
	}
}

func (q *queueScreen) expandSelectedCollection() (string, tea.Cmd) {
	index := q.browser.SelectedIndex()
	if index < 0 || index >= len(q.browserData) {
		return "Select a collection row to expand.", nil
	}
	row := q.browserData[index]
	if row.kind != queueRowSearchResult || !isCollectionResult(row.result) {
		return "Select an album or playlist result to expand it.", nil
	}
	if q.expandedCollectionIDs[row.result.ID] {
		delete(q.expandedCollectionIDs, row.result.ID)
		q.rebuildBrowser()
		return fmt.Sprintf("Collapsed %q.", row.result.Title), nil
	}
	if cached := q.expandedCollections[row.result.ID]; len(cached) > 0 {
		q.expandedCollectionIDs[row.result.ID] = true
		q.rebuildBrowser()
		return fmt.Sprintf("Expanded %q.", row.result.Title), nil
	}
	if q.services.Search == nil {
		return "Collection expansion is unavailable.", nil
	}

	q.cancelRunningExpand()
	ctx, cancel := context.WithCancel(context.Background())
	q.cancelExpand = cancel
	q.expandingCollectionID = row.result.ID
	q.rebuildBrowser()
	return fmt.Sprintf("Loading songs from %q.", row.result.Title), func() tea.Msg {
		defer cancel()
		results, err := q.services.Search.ExpandCollection(ctx, row.result)
		if err != nil && errors.Is(err, context.Canceled) {
			return nil
		}
		return queueExpandResultsMsg{resultID: row.result.ID, results: results, err: err}
	}
}

func isCollectionResult(result SearchResult) bool {
	return result.Kind == MediaAlbum || result.Kind == MediaPlaylist
}

func isSpecialYouTubeResult(result SearchResult) bool {
	return result.Kind == MediaArtist || result.Kind == MediaAlbum || result.Kind == MediaPlaylist
}

func (q *queueScreen) groupTitle(groupID string) string {
	for _, entry := range q.queueData {
		if entry.GroupID == groupID {
			return entry.GroupTitle
		}
	}
	return ""
}

func (q *queueScreen) resetSourceScopedState() {
	q.searchMode = q.defaultSearchModeForSource(q.activeSource())
	if q.activeSearchMode() != SearchModeSongs {
		q.artistFilter = SearchArtistFilter{}
	}
}
