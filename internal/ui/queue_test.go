package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type stubQueueService struct {
	entries []QueueEntry
}

func (s *stubQueueService) Snapshot() []QueueEntry { return append([]QueueEntry(nil), s.entries...) }
func (s *stubQueueService) Add(result SearchResult) error {
	if isCollectionResult(result) && len(result.CollectionItems) > 0 {
		for index, entry := range result.CollectionItems {
			entry.GroupID = result.ID
			entry.GroupTitle = result.Title
			entry.GroupKind = result.Kind
			entry.GroupIndex = index
			entry.GroupSize = len(result.CollectionItems)
			s.entries = append(s.entries, entry)
		}
		return nil
	}
	s.entries = append(s.entries, QueueEntry{ID: result.ID, Title: result.Title, Subtitle: result.Subtitle, Source: result.Source, Kind: result.Kind, Duration: result.Duration, Artwork: result.Artwork})
	return nil
}
func (s *stubQueueService) Move(id string, delta int) error {
	index := -1
	for i, entry := range s.entries {
		if entry.ID == id {
			index = i
			break
		}
	}
	if index == -1 {
		return nil
	}
	target := index + delta
	if target < 0 {
		target = 0
	}
	if target >= len(s.entries) {
		target = len(s.entries) - 1
	}
	if target == index {
		return nil
	}
	entry := s.entries[index]
	s.entries = append(s.entries[:index], s.entries[index+1:]...)
	head := append([]QueueEntry(nil), s.entries[:target]...)
	head = append(head, entry)
	s.entries = append(head, s.entries[target:]...)
	return nil
}
func (s *stubQueueService) Remove(id string) error           { return nil }
func (s *stubQueueService) RemoveGroup(groupID string) error { return nil }
func (s *stubQueueService) Clear() error {
	s.entries = nil
	return nil
}

type stubPlaybackService struct {
	snapshot         PlaybackSnapshot
	togglePauseCalls int
	togglePauseErr   error
}

func (s *stubPlaybackService) Snapshot() PlaybackSnapshot        { return s.snapshot }
func (s *stubPlaybackService) TogglePause() error                { s.togglePauseCalls++; return s.togglePauseErr }
func (s *stubPlaybackService) Previous() error                   { return nil }
func (s *stubPlaybackService) Next() error                       { return nil }
func (s *stubPlaybackService) SeekTo(target time.Duration) error { _ = target; return nil }
func (s *stubPlaybackService) AdjustVolume(delta int) error      { return nil }
func (s *stubPlaybackService) SetRepeat(repeat bool) error       { return nil }
func (s *stubPlaybackService) SetStream(stream bool) error       { return nil }

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

func TestQueueBrowserPrefixesQueuedRowTitlesWithNormalizedSource(t *testing.T) {
	screen := newQueueScreen(Services{})
	screen.queueData = []QueueEntry{{ID: "queued-1", Title: "Queued track", Source: "youtube music"}}
	screen.SetSize(60, 8)
	screen.rebuildBrowser()

	view := screen.browser.View()
	if !strings.Contains(view, "youtube: Queued track") {
		t.Fatalf("expected normalized source prefix in queued row, got %q", view)
	}
}

func TestQueueBrowserPrefixesSearchResultTitlesWithNormalizedSource(t *testing.T) {
	screen := newQueueScreen(Services{})
	screen.resultData = []SearchResult{{ID: "result-1", Title: "Search result", Source: "Local files", Kind: MediaTrack}}
	screen.SetSize(60, 8)
	screen.rebuildBrowser()

	view := screen.browser.View()
	if !strings.Contains(view, "local: Search result") {
		t.Fatalf("expected normalized source prefix in search result row, got %q", view)
	}
}

func TestQueueBrowserAddsSearchResultToQueue(t *testing.T) {
	screen := newQueueScreen(Services{})
	screen.resultData = []SearchResult{{ID: "result-1", Title: "Search result", Source: "Local files", Kind: MediaTrack}}
	screen.rebuildBrowser()

	got, _ := screen.activateSelectedRow()
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

	got, _ := screen.activateSelectedRow()
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

	got, cmd := screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if got != "Search cleared." {
		t.Fatalf("expected search cleared status, got %q", got)
	}
	if cmd != nil {
		t.Fatalf("expected no search command for cleared query, got %v", cmd)
	}
	if screen.searchInput.Value() != "" {
		t.Fatalf("expected cleared search input, got %q", screen.searchInput.Value())
	}
	if len(screen.resultData) != 0 {
		t.Fatalf("expected cleared results, got %#v", screen.resultData)
	}
}

func TestQueueArrowKeysMoveFocusIntoBrowserBeforeBrowsing(t *testing.T) {
	screen := newQueueScreen(Services{})
	screen.resultData = []SearchResult{
		{ID: "result-1", Title: "First", Source: "Local files", Kind: MediaTrack},
		{ID: "result-2", Title: "Second", Source: "Local files", Kind: MediaTrack},
	}
	screen.rebuildBrowser()

	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if screen.focus != focusBrowser {
		t.Fatalf("expected down from search to focus browser, got %v", screen.focus)
	}
	if screen.browser.SelectedIndex() != 0 {
		t.Fatalf("expected first down to keep current browser row selected, got %d", screen.browser.SelectedIndex())
	}

	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	got := screen.browser.SelectedIndex()
	if got != 1 {
		t.Fatalf("expected second down to move browser selection, got %d", got)
	}
	if !screen.browserData[got].resultMatchesID("result-2") {
		t.Fatalf("expected second result selected, got %#v", screen.browserData[got])
	}
	if screen.searchInput.Value() != "" {
		t.Fatalf("expected search input value unchanged, got %q", screen.searchInput.Value())
	}
}

func TestQueueBrowserEnterTogglesSearchResultQueueMembership(t *testing.T) {
	screen := newQueueScreen(Services{})
	screen.resultData = []SearchResult{{ID: "result-1", Title: "Search result", Source: "Local files", Kind: MediaTrack}}
	screen.rebuildBrowser()

	got, _ := screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !strings.Contains(got, `Added "Search result"`) {
		t.Fatalf("expected add status, got %q", got)
	}
	if len(screen.queueData) != 1 {
		t.Fatalf("expected queued item after first enter, got %#v", screen.queueData)
	}

	got, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !strings.Contains(got, `Removed "Search result"`) {
		t.Fatalf("expected remove status, got %q", got)
	}
	if len(screen.queueData) != 0 {
		t.Fatalf("expected queue emptied after second enter, got %#v", screen.queueData)
	}
}

func TestQueueBrowserAutoStartsFirstQueuedStream(t *testing.T) {
	queue := &stubQueueService{}
	playback := &stubPlaybackService{}
	screen := newQueueScreen(Services{Queue: queue, Playback: playback})
	screen.resultData = []SearchResult{{ID: "radio:station-1:mp3:direct", Title: "Jazz FM", Source: "Radio", Kind: MediaStream}}
	screen.rebuildBrowser()

	got, cmd := screen.activateSelectedRow()
	if !strings.Contains(got, `Starting playback.`) {
		t.Fatalf("expected start-playback status, got %q", got)
	}
	if cmd == nil {
		t.Fatal("expected async playback command for first queued stream")
	}
	if playback.togglePauseCalls != 0 {
		t.Fatalf("expected playback not started until command runs, got %d calls", playback.togglePauseCalls)
	}

	msg := cmd()
	got, _ = screen.Update(msg)
	if !strings.Contains(got, `started playback`) {
		t.Fatalf("expected playback-started status, got %q", got)
	}
	if playback.togglePauseCalls != 1 {
		t.Fatalf("expected one toggle-pause call, got %d", playback.togglePauseCalls)
	}
}

func TestQueueBrowserDoesNotAutoStartAdditionalQueuedStream(t *testing.T) {
	queue := &stubQueueService{entries: []QueueEntry{{ID: "existing", Title: "Existing", Source: "Local files", Kind: MediaTrack}}}
	playback := &stubPlaybackService{}
	screen := newQueueScreen(Services{Queue: queue, Playback: playback})
	screen.resultData = []SearchResult{{ID: "radio:station-1:mp3:direct", Title: "Jazz FM", Source: "Radio", Kind: MediaStream}}
	screen.syncQueue()
	screen.rebuildBrowser()

	got, cmd := screen.activateSelectedRow()
	if strings.Contains(got, `Starting playback.`) {
		t.Fatalf("did not expect auto-start status, got %q", got)
	}
	if cmd != nil {
		t.Fatalf("expected no playback command for non-empty queue, got %v", cmd)
	}
	if playback.togglePauseCalls != 0 {
		t.Fatalf("expected no toggle-pause call, got %d", playback.togglePauseCalls)
	}
}

func TestQueueBrowserMoveSelectedQueuedItem(t *testing.T) {
	queue := &stubQueueService{entries: []QueueEntry{
		{ID: "one", Title: "First"},
		{ID: "two", Title: "Second"},
	}}
	screen := newQueueScreen(Services{Queue: queue})
	screen.rebuildBrowser()

	got := screen.moveSelectedQueueEntry(1)
	if !strings.Contains(got, `Moved "First" to queue position 2.`) {
		t.Fatalf("expected move status, got %q", got)
	}
	if len(screen.queueData) != 2 || screen.queueData[0].ID != "two" || screen.queueData[1].ID != "one" {
		t.Fatalf("expected reordered queue, got %#v", screen.queueData)
	}
	if screen.browser.SelectedIndex() != 1 {
		t.Fatalf("expected moved row to remain selected, got %d", screen.browser.SelectedIndex())
	}
}

func TestQueueBrowserMarksNowPlayingQueuedItem(t *testing.T) {
	screen := newQueueScreen(Services{
		Playback: &stubPlaybackService{
			snapshot: PlaybackSnapshot{
				Track: &TrackInfo{ID: "queued-1", Title: "Queued track"},
			},
		},
	})
	screen.queueData = []QueueEntry{{ID: "queued-1", Title: "Queued track", Source: "Queue"}}
	screen.SetSize(40, 10)

	view := screen.View()
	if !strings.Contains(view, "▶ Queued track") {
		t.Fatalf("expected now-playing marker in queue view, got %q", view)
	}
	if !strings.Contains(view, "playing") {
		t.Fatalf("expected now-playing meta in queue view, got %q", view)
	}
}

func (r queueBrowserRow) resultMatchesID(id string) bool {
	return r.kind == queueRowSearchResult && r.result.ID == id
}

type blockingSearchService struct {
	calls   int
	block   chan struct{}
	started chan struct{}
}

func (s *blockingSearchService) Sources() []SourceDescriptor {
	return []SourceDescriptor{{ID: "youtube-music", Name: "YouTube Music"}}
}

func (s *blockingSearchService) Search(ctx context.Context, request SearchRequest) ([]SearchResult, error) {
	s.calls++
	if s.started != nil {
		s.started <- struct{}{}
	}
	select {
	case <-s.block:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return []SearchResult{{ID: "result-1", Title: "Async result", Source: "YouTube Music", Kind: MediaTrack}}, nil
}
func (s *blockingSearchService) ExpandCollection(context.Context, SearchResult) ([]SearchResult, error) {
	return nil, nil
}

type multiSourceSearchService struct {
	sources []SourceDescriptor
}

func (s multiSourceSearchService) Sources() []SourceDescriptor {
	return append([]SourceDescriptor(nil), s.sources...)
}
func (s multiSourceSearchService) Search(context.Context, SearchRequest) ([]SearchResult, error) {
	return nil, nil
}
func (s multiSourceSearchService) ExpandCollection(context.Context, SearchResult) ([]SearchResult, error) {
	return nil, nil
}

type expandingSearchService struct {
	sources   []SourceDescriptor
	expanded  []SearchResult
	expandFor string
}

func (s expandingSearchService) Sources() []SourceDescriptor {
	return append([]SourceDescriptor(nil), s.sources...)
}

func (s expandingSearchService) Search(context.Context, SearchRequest) ([]SearchResult, error) {
	return nil, nil
}

func (s expandingSearchService) ExpandCollection(_ context.Context, result SearchResult) ([]SearchResult, error) {
	if result.ID != s.expandFor {
		return nil, nil
	}
	return append([]SearchResult(nil), s.expanded...), nil
}

func TestQueueBrowserTypingStartsAsyncSearch(t *testing.T) {
	search := &blockingSearchService{block: make(chan struct{}), started: make(chan struct{}, 1)}
	screen := newQueueScreen(Services{Search: search})
	screen.searchDelay = 0

	status, cmd := screen.Update(tea.KeyPressMsg(tea.Key{Text: "y"}))
	if !strings.Contains(status, `Searching YouTube Music for "y".`) {
		t.Fatalf("expected search status, got %q", status)
	}
	if cmd == nil {
		t.Fatal("expected async search cmd")
	}
	if search.calls != 0 {
		t.Fatalf("expected search not to run inline, got %d calls", search.calls)
	}
	if !screen.searching {
		t.Fatal("expected screen to remain in searching state until async results return")
	}

	startMsg := cmd()
	status, cmd = screen.Update(startMsg)
	if status != "" {
		t.Fatalf("expected debounce message to keep status unchanged, got %q", status)
	}
	if cmd == nil {
		t.Fatal("expected search command after debounce message")
	}

	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	<-search.started
	if search.calls != 1 {
		t.Fatalf("expected search to run when cmd executes, got %d calls", search.calls)
	}

	close(search.block)
	msg := <-done
	status, _ = screen.Update(msg)
	if status != "" {
		t.Fatalf("expected result application to keep status unchanged, got %q", status)
	}
	if len(screen.resultData) != 1 || screen.resultData[0].Title != "Async result" {
		t.Fatalf("expected async results applied, got %#v", screen.resultData)
	}
}

func TestQueueBrowserTypingShowsSearchingStateImmediately(t *testing.T) {
	search := &blockingSearchService{block: make(chan struct{}), started: make(chan struct{}, 1)}
	screen := newQueueScreen(Services{Search: search})
	screen.SetSize(50, 12)

	_, cmd := screen.Update(tea.KeyPressMsg(tea.Key{Text: "y"}))
	if cmd == nil {
		t.Fatal("expected async search cmd")
	}

	view := screen.View()
	if !strings.Contains(view, "Searching") {
		t.Fatalf("expected searching state in queue view, got %q", view)
	}
	close(search.block)
}

func TestQueueBrowserDebounceDropsSupersededSearchStart(t *testing.T) {
	search := &blockingSearchService{block: make(chan struct{}), started: make(chan struct{}, 1)}
	screen := newQueueScreen(Services{Search: search})
	screen.searchDelay = 0

	_, cmd1 := screen.Update(tea.KeyPressMsg(tea.Key{Text: "y"}))
	_, cmd2 := screen.Update(tea.KeyPressMsg(tea.Key{Text: "o"}))
	if cmd1 == nil || cmd2 == nil {
		t.Fatal("expected debounce commands for both keypresses")
	}

	status, next := screen.Update(cmd1())
	if status != "" || next != nil {
		t.Fatalf("expected stale debounce message to be ignored, got status=%q cmd=%v", status, next)
	}
	if search.calls != 0 {
		t.Fatalf("expected stale debounce not to start a search, got %d calls", search.calls)
	}
}

func TestQueueBrowserNewSearchCancelsRunningSearch(t *testing.T) {
	search := &blockingSearchService{block: make(chan struct{}), started: make(chan struct{}, 2)}
	screen := newQueueScreen(Services{Search: search})
	screen.searchDelay = 0

	_, cmd := screen.Update(tea.KeyPressMsg(tea.Key{Text: "y"}))
	if cmd == nil {
		t.Fatal("expected first debounce command")
	}
	_, searchCmd := screen.Update(cmd())
	if searchCmd == nil {
		t.Fatal("expected first search command")
	}

	firstDone := make(chan tea.Msg, 1)
	go func() { firstDone <- searchCmd() }()
	<-search.started

	_, cmd = screen.Update(tea.KeyPressMsg(tea.Key{Text: "o"}))
	if cmd == nil {
		t.Fatal("expected second debounce command")
	}
	select {
	case msg := <-firstDone:
		if msg != nil {
			t.Fatalf("expected canceled first search to return no message, got %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("expected first search to be canceled promptly")
	}

	_, searchCmd = screen.Update(cmd())
	if searchCmd == nil {
		t.Fatal("expected second search command")
	}
	go func() { _ = searchCmd() }()
	<-search.started
	if search.calls != 2 {
		t.Fatalf("expected second search to start after cancellation, got %d calls", search.calls)
	}
	close(search.block)
}

func TestQueueBrowserSourceHotkeysDoNotEditSearchInput(t *testing.T) {
	screen := newQueueScreen(Services{
		Search: multiSourceSearchService{
			sources: []SourceDescriptor{
				{ID: "local", Name: "Local files"},
				{ID: "youtube-music", Name: "YouTube Music"},
			},
		},
	})
	screen.searchInput.SetValue("mix")
	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: 'f', Mod: tea.ModCtrl}))

	status, _ := screen.Update(tea.KeyPressMsg(tea.Key{Text: "]"}))
	if status != "Active source: YouTube Music" {
		t.Fatalf("expected source switch status, got %q", status)
	}
	if screen.searchInput.Value() != "mix" {
		t.Fatalf("expected source hotkey to leave query unchanged, got %q", screen.searchInput.Value())
	}
	if screen.activeSource().ID != "youtube-music" {
		t.Fatalf("expected active source to switch, got %#v", screen.activeSource())
	}
}

func TestQueueBrowserSearchKindHotkeysDoNotEditSearchInput(t *testing.T) {
	screen := newQueueScreen(Services{
		Search: multiSourceSearchService{
			sources: []SourceDescriptor{{
				ID:   "all",
				Name: "All sources",
				SearchModes: []SearchModeDescriptor{
					{ID: SearchModeAll, Name: "All"},
					{ID: SearchModeTracks, Name: "Tracks"},
					{ID: SearchModeStreams, Name: "Streams"},
					{ID: SearchModePlaylists, Name: "Playlists"},
				},
				DefaultMode: SearchModeAll,
			}},
		},
	})
	screen.searchInput.SetValue("mix")
	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: 'f', Mod: tea.ModCtrl}))

	status, _ := screen.Update(tea.KeyPressMsg(tea.Key{Code: '1', Text: "1"}))
	if status != "Search mode: All" {
		t.Fatalf("expected search mode status, got %q", status)
	}
	if screen.searchInput.Value() != "mix" {
		t.Fatalf("expected search kind hotkey to leave query unchanged, got %q", screen.searchInput.Value())
	}
	if screen.activeSearchMode() != SearchModeAll {
		t.Fatalf("expected first visible search kind to be selected, got %q", screen.activeSearchMode())
	}
}

func TestQueueSearchFocusToggleAllowsLiteralReservedCharacters(t *testing.T) {
	screen := newQueueScreen(Services{})
	screen.searchInput.SetValue("mix")

	status, _ := screen.Update(tea.KeyPressMsg(tea.Key{Code: 'f', Mod: tea.ModCtrl}))
	if !strings.Contains(status, "Browser focused") {
		t.Fatalf("expected browser focused status, got %q", status)
	}
	if screen.focus != focusBrowser {
		t.Fatalf("expected browser focus, got %v", screen.focus)
	}

	status, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: 'f', Mod: tea.ModCtrl}))
	if !strings.Contains(status, "Search focused") {
		t.Fatalf("expected search focused status, got %q", status)
	}
	if screen.focus != focusSearch {
		t.Fatalf("expected search focus, got %v", screen.focus)
	}

	status, _ = screen.Update(tea.KeyPressMsg(tea.Key{Text: "]"}))
	if !strings.Contains(status, `Searching`) {
		t.Fatalf("expected literal keypress to update search while focused, got %q", status)
	}
	if screen.searchInput.Value() != "mix]" {
		t.Fatalf("expected reserved character to be inserted into search, got %q", screen.searchInput.Value())
	}
}

func TestQueueSourceHotkeysRequireUnfocusedSearch(t *testing.T) {
	screen := newQueueScreen(Services{
		Search: multiSourceSearchService{
			sources: []SourceDescriptor{
				{ID: "local", Name: "Local files"},
				{ID: "youtube-music", Name: "YouTube Music"},
			},
		},
	})

	status, _ := screen.Update(tea.KeyPressMsg(tea.Key{Text: "]"}))
	if !strings.Contains(status, `Searching`) {
		t.Fatalf("expected focused search to treat ] as text, got %q", status)
	}
	if screen.activeSource().ID != "local" {
		t.Fatalf("expected source to stay unchanged while search is focused, got %#v", screen.activeSource())
	}

	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: 'f', Mod: tea.ModCtrl}))
	status, _ = screen.Update(tea.KeyPressMsg(tea.Key{Text: "]"}))
	if status != "Active source: YouTube Music" {
		t.Fatalf("expected source switch once search is unfocused, got %q", status)
	}
}

func TestQueueArrowKeysMoveFocusAcrossZones(t *testing.T) {
	screen := newQueueScreen(Services{
		Search: multiSourceSearchService{
			sources: []SourceDescriptor{
				{
					ID:   "local",
					Name: "Local files",
					SearchModes: []SearchModeDescriptor{
						{ID: SearchModeSongs, Name: "Songs"},
						{ID: SearchModeAlbums, Name: "Albums"},
					},
					DefaultMode: SearchModeSongs,
				},
				{
					ID:   "youtube-music",
					Name: "YouTube Music",
					SearchModes: []SearchModeDescriptor{
						{ID: SearchModeSongs, Name: "Songs"},
						{ID: SearchModeAlbums, Name: "Albums"},
					},
					DefaultMode: SearchModeSongs,
				},
			},
		},
	})
	screen.resultData = []SearchResult{
		{ID: "result-1", Title: "First", Source: "Local files", Kind: MediaTrack},
		{ID: "result-2", Title: "Second", Source: "Local files", Kind: MediaTrack},
	}
	screen.rebuildBrowser()

	if screen.focus != focusSearch {
		t.Fatalf("expected initial search focus, got %v", screen.focus)
	}

	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if screen.focus != focusModes {
		t.Fatalf("expected up from search to focus modes, got %v", screen.focus)
	}

	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if screen.focus != focusSources {
		t.Fatalf("expected up from modes to focus sources, got %v", screen.focus)
	}

	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if screen.focus != focusModes {
		t.Fatalf("expected down from sources to focus modes, got %v", screen.focus)
	}

	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if screen.focus != focusSearch {
		t.Fatalf("expected down from modes to focus search, got %v", screen.focus)
	}

	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if screen.focus != focusBrowser {
		t.Fatalf("expected down from search to focus browser, got %v", screen.focus)
	}

	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if screen.focus != focusSearch {
		t.Fatalf("expected up at browser top to focus search, got %v", screen.focus)
	}
}

func TestQueueFocusedChipsUseLeftRightNavigation(t *testing.T) {
	screen := newQueueScreen(Services{
		Search: multiSourceSearchService{
			sources: []SourceDescriptor{
				{
					ID:   "local",
					Name: "Local files",
					SearchModes: []SearchModeDescriptor{
						{ID: SearchModeSongs, Name: "Songs"},
						{ID: SearchModeAlbums, Name: "Albums"},
					},
					DefaultMode: SearchModeSongs,
				},
				{
					ID:   "youtube-music",
					Name: "YouTube Music",
					SearchModes: []SearchModeDescriptor{
						{ID: SearchModeSongs, Name: "Songs"},
						{ID: SearchModeAlbums, Name: "Albums"},
					},
					DefaultMode: SearchModeSongs,
				},
			},
		},
	})

	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if screen.focus != focusModes {
		t.Fatalf("expected modes focus, got %v", screen.focus)
	}

	status, _ := screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	if status != "Search mode: Albums" {
		t.Fatalf("expected right to advance search mode, got %q", status)
	}
	if screen.activeSearchMode() != SearchModeAlbums {
		t.Fatalf("expected albums mode after right key, got %q", screen.activeSearchMode())
	}

	status, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	if status != "Search mode: Songs" {
		t.Fatalf("expected left to move search mode backward, got %q", status)
	}
	if screen.activeSearchMode() != SearchModeSongs {
		t.Fatalf("expected songs mode after left key, got %q", screen.activeSearchMode())
	}

	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if screen.focus != focusSources {
		t.Fatalf("expected sources focus, got %v", screen.focus)
	}

	status, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	if status != "Active source: YouTube Music" {
		t.Fatalf("expected right to advance source, got %q", status)
	}
	if screen.activeSource().ID != "youtube-music" {
		t.Fatalf("expected source to change on right key, got %#v", screen.activeSource())
	}

	status, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	if status != "Active source: Local files" {
		t.Fatalf("expected left to move source backward, got %q", status)
	}
	if screen.activeSource().ID != "local" {
		t.Fatalf("expected source to change back on left key, got %#v", screen.activeSource())
	}
}

func TestQueueModeCycleSwitchesFocusedSearchModes(t *testing.T) {
	screen := newQueueScreen(Services{
		Search: multiSourceSearchService{
			sources: []SourceDescriptor{{
				ID:   "youtube-music",
				Name: "YouTube Music",
				SearchModes: []SearchModeDescriptor{
					{ID: SearchModeSongs, Name: "Songs"},
					{ID: SearchModeArtists, Name: "Artists"},
					{ID: SearchModeAlbums, Name: "Albums"},
				},
				DefaultMode: SearchModeSongs,
			}},
		},
	})
	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: 'f', Mod: tea.ModCtrl}))

	status, _ := screen.Update(tea.KeyPressMsg(tea.Key{Text: "m"}))
	if status != "Search mode: Artists" {
		t.Fatalf("expected mode cycle status, got %q", status)
	}
	if screen.activeSearchMode() != SearchModeArtists {
		t.Fatalf("expected artists mode, got %q", screen.activeSearchMode())
	}
}

func TestQueueModeHotkeysSelectSpecificMode(t *testing.T) {
	screen := newQueueScreen(Services{
		Search: multiSourceSearchService{
			sources: []SourceDescriptor{{
				ID:   "youtube-music",
				Name: "YouTube Music",
				SearchModes: []SearchModeDescriptor{
					{ID: SearchModeSongs, Name: "Songs"},
					{ID: SearchModeArtists, Name: "Artists"},
					{ID: SearchModeAlbums, Name: "Albums"},
					{ID: SearchModePlaylists, Name: "Playlists"},
				},
				DefaultMode: SearchModeSongs,
			}},
		},
	})
	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: 'f', Mod: tea.ModCtrl}))

	status, _ := screen.Update(tea.KeyPressMsg(tea.Key{Text: "4"}))
	if status != "Search mode: Playlists" {
		t.Fatalf("expected direct playlist mode status, got %q", status)
	}
	if screen.activeSearchMode() != SearchModePlaylists {
		t.Fatalf("expected playlists mode, got %q", screen.activeSearchMode())
	}
}

func TestQueueArtistSelectionAppliesSongsFilter(t *testing.T) {
	screen := newQueueScreen(Services{
		Search: multiSourceSearchService{
			sources: []SourceDescriptor{{
				ID:   "youtube-music",
				Name: "YouTube Music",
				SearchModes: []SearchModeDescriptor{
					{ID: SearchModeSongs, Name: "Songs"},
					{ID: SearchModeArtists, Name: "Artists"},
				},
				DefaultMode: SearchModeArtists,
			}},
		},
	})
	screen.searchMode = SearchModeArtists
	screen.searchInput.SetValue("beatles")
	screen.resultData = []SearchResult{{
		ID:           "youtube:artist:beatles",
		Title:        "The Beatles",
		Source:       "YouTube Music",
		Kind:         MediaArtist,
		ArtistFilter: SearchArtistFilter{ID: "beatles", Name: "The Beatles"},
	}}
	screen.rebuildBrowser()

	status, cmd := screen.activateSelectedRow()
	if status != "Artist filter: The Beatles" {
		t.Fatalf("expected artist filter status, got %q", status)
	}
	if cmd == nil {
		t.Fatal("expected follow-up search command after artist selection")
	}
	if screen.activeSearchMode() != SearchModeSongs {
		t.Fatalf("expected songs mode after artist selection, got %q", screen.activeSearchMode())
	}
	if screen.artistFilter.Name != "The Beatles" {
		t.Fatalf("expected artist filter to be stored, got %#v", screen.artistFilter)
	}
}

func TestQueueExpandCollectionShowsChildSongs(t *testing.T) {
	screen := newQueueScreen(Services{
		Search: expandingSearchService{
			sources: []SourceDescriptor{{
				ID:          "youtube-music",
				Name:        "YouTube Music",
				SearchModes: []SearchModeDescriptor{{ID: SearchModeAlbums, Name: "Albums"}},
				DefaultMode: SearchModeAlbums,
			}},
			expandFor: "youtube:album:album-1",
			expanded: []SearchResult{{
				ID:       "youtube:https://music.youtube.com/watch?v=track-1",
				Title:    "Track One",
				Subtitle: "Artist One",
				Source:   "YouTube Music",
				Kind:     MediaTrack,
			}},
		},
	})
	screen.resultData = []SearchResult{{
		ID:              "youtube:album:album-1",
		Title:           "Album One",
		Subtitle:        "Artist One",
		Source:          "YouTube Music",
		Kind:            MediaAlbum,
		BrowseID:        "album-1",
		CollectionCount: 1,
	}}
	screen.rebuildBrowser()
	_, _ = screen.Update(tea.KeyPressMsg(tea.Key{Code: 'f', Mod: tea.ModCtrl}))

	status, cmd := screen.Update(tea.KeyPressMsg(tea.Key{Text: "e"}))
	if status != `Loading songs from "Album One".` {
		t.Fatalf("expected expand status, got %q", status)
	}
	if cmd == nil {
		t.Fatal("expected async expand command")
	}

	msg := cmd()
	status, _ = screen.Update(msg)
	if status != "" {
		t.Fatalf("expected expand result application to leave status unchanged, got %q", status)
	}
	if !screen.expandedCollectionIDs["youtube:album:album-1"] {
		t.Fatal("expected collection to be marked expanded")
	}
	if len(screen.expandedCollections["youtube:album:album-1"]) != 1 {
		t.Fatalf("expected cached child rows, got %#v", screen.expandedCollections)
	}
}

func TestQueueCollectionAddCreatesRemovableGroupRow(t *testing.T) {
	queue := &stubQueueService{}
	screen := newQueueScreen(Services{Queue: queue})
	screen.resultData = []SearchResult{{
		ID:     "youtube:playlist:mix-1",
		Title:  "Mix One",
		Source: "YouTube Music",
		Kind:   MediaPlaylist,
		CollectionItems: []QueueEntry{
			{ID: "track-1", Title: "Track One", Source: "YouTube Music", Kind: MediaTrack},
			{ID: "track-2", Title: "Track Two", Source: "YouTube Music", Kind: MediaTrack},
		},
	}}
	screen.rebuildBrowser()

	status, _ := screen.activateSelectedRow()
	if status != `Added "Mix One" to the queue.` {
		t.Fatalf("expected add-all collection status, got %q", status)
	}
	if len(screen.queueData) != 2 {
		t.Fatalf("expected grouped child entries in queue, got %#v", screen.queueData)
	}
	if len(screen.browserData) == 0 || screen.browserData[0].kind != queueRowQueueGroup {
		t.Fatalf("expected synthetic group row at top of browser, got %#v", screen.browserData)
	}

	screen.browser.SetSelectedIndex(0)
	status, _ = screen.activateSelectedRow()
	if status != `Removed "Mix One" from the queue.` {
		t.Fatalf("expected group removal status, got %q", status)
	}
}
