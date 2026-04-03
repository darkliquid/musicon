package ui

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// SessionStore persists restorable app session snapshots outside the UI process.
type SessionStore interface {
	Save(SessionSnapshot) error
}

// SessionSnapshot captures the restorable UI and playback context for one Musicon session.
type SessionSnapshot struct {
	Mode             Mode                 `json:"mode"`
	CompactMode      bool                 `json:"compact_mode"`
	ShowHelp         bool                 `json:"show_help"`
	Queue            QueueSessionState    `json:"queue"`
	Playback         PlaybackSessionState `json:"playback"`
	QueueEntries     []QueueEntry         `json:"queue_entries,omitempty"`
	PlaybackSnapshot PlaybackSnapshot     `json:"playback_snapshot"`
}

// QueueSessionState captures the queue-screen UI state that should survive restarts.
type QueueSessionState struct {
	SourceID              string                    `json:"source_id,omitempty"`
	SearchMode            SearchMode                `json:"search_mode,omitempty"`
	ArtistFilter          SearchArtistFilter        `json:"artist_filter"`
	Query                 string                    `json:"query,omitempty"`
	Focus                 string                    `json:"focus,omitempty"`
	SelectedRowKey        string                    `json:"selected_row_key,omitempty"`
	SearchResults         []SearchResult            `json:"search_results,omitempty"`
	ExpandedCollections   map[string][]SearchResult `json:"expanded_collections,omitempty"`
	ExpandedCollectionIDs []string                  `json:"expanded_collection_ids,omitempty"`
}

// PlaybackSessionState captures the playback-screen UI state that should survive restarts.
type PlaybackSessionState struct {
	Pane         PlaybackPane `json:"pane"`
	ShowInfo     bool         `json:"show_info"`
	LyricsScroll int          `json:"lyrics_scroll"`
}

func (m *rootModel) applyRestoredSession(snapshot *SessionSnapshot) {
	if snapshot == nil {
		return
	}

	switch snapshot.Mode {
	case ModePlayback:
		m.mode = ModePlayback
	default:
		m.mode = ModeQueue
	}
	m.showHelp = snapshot.ShowHelp
	m.compactMode = snapshot.CompactMode
	if m.queue != nil {
		m.queue.applySessionState(snapshot.Queue)
	}
	if m.playback != nil {
		m.playback.applySessionState(snapshot.Playback, snapshot.PlaybackSnapshot)
	}
	m.syncCompactMode()
	m.status = "Restored previous session."
}

func (m *rootModel) persistSession(force, allowThrottle bool) error {
	if m.sessionStore == nil || m.queue == nil || m.playback == nil {
		return nil
	}
	if allowThrottle && !force && !m.lastPersistAt.IsZero() && time.Since(m.lastPersistAt) < time.Second {
		return nil
	}

	snapshot := m.sessionSnapshot()
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if !force && string(data) == m.lastPersistedJSON {
		return nil
	}
	if err := m.sessionStore.Save(snapshot); err != nil {
		return err
	}
	m.lastPersistedJSON = string(data)
	m.lastPersistAt = time.Now()
	return nil
}

func (m *rootModel) rememberRestoredSession() {
	snapshot := m.sessionSnapshot()
	data, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	m.lastPersistedJSON = string(data)
	m.lastPersistAt = time.Now()
}

func (m *rootModel) sessionSnapshot() SessionSnapshot {
	snapshot := SessionSnapshot{
		Mode:             m.mode,
		CompactMode:      m.compactMode,
		ShowHelp:         m.showHelp,
		Queue:            m.queue.sessionState(),
		Playback:         m.playback.sessionState(),
		QueueEntries:     cloneQueueEntries(m.queue.queueSnapshot()),
		PlaybackSnapshot: m.playback.snapshotForSession(),
	}
	snapshot.PlaybackSnapshot.Position = snapshot.PlaybackSnapshot.Position.Round(0)
	snapshot.PlaybackSnapshot.Duration = snapshot.PlaybackSnapshot.Duration.Round(0)
	return snapshot
}

func (q *queueScreen) sessionState() QueueSessionState {
	state := QueueSessionState{
		SourceID:            q.activeSource().ID,
		SearchMode:          q.activeSearchMode(),
		ArtistFilter:        q.artistFilter,
		Query:               q.searchInput.Value(),
		Focus:               q.focus.sessionName(),
		SelectedRowKey:      q.selectedRowKey(),
		SearchResults:       cloneSearchResults(q.resultData),
		ExpandedCollections: cloneExpandedCollections(q.expandedCollections),
	}
	for id, expanded := range q.expandedCollectionIDs {
		if !expanded {
			continue
		}
		state.ExpandedCollectionIDs = append(state.ExpandedCollectionIDs, id)
	}
	sort.Strings(state.ExpandedCollectionIDs)
	return state
}

func (q *queueScreen) applySessionState(state QueueSessionState) {
	if index := q.sourceIndexByID(state.SourceID); index >= 0 {
		q.sourceIndex = index
	}
	q.searchMode = q.defaultSearchModeForSource(q.activeSource())
	if state.SearchMode != "" && q.sourceSupportsMode(state.SearchMode) {
		q.searchMode = state.SearchMode
	}
	if q.activeSearchMode() == SearchModeSongs {
		q.artistFilter = state.ArtistFilter
	} else {
		q.artistFilter = SearchArtistFilter{}
	}
	q.searchInput.SetValue(state.Query)
	q.resultData = cloneSearchResults(state.SearchResults)
	q.expandedCollections = cloneExpandedCollections(state.ExpandedCollections)
	q.expandedCollectionIDs = make(map[string]bool, len(state.ExpandedCollectionIDs))
	for _, id := range state.ExpandedCollectionIDs {
		if _, ok := q.expandedCollections[id]; ok {
			q.expandedCollectionIDs[id] = true
		}
	}
	q.searching = false
	q.cancelSearch = nil
	q.cancelExpand = nil
	q.expandingCollectionID = ""
	q.lastQuery = state.Query
	q.lastSourceID = q.activeSource().ID
	q.setFocus(queueFocusFromSession(state.Focus))
	q.rebuildBrowser()
	if state.SelectedRowKey != "" {
		for index, row := range q.browserData {
			if row.key() == state.SelectedRowKey {
				q.browser.SetSelectedIndex(index)
				break
			}
		}
	}
}

func (q *queueScreen) queueSnapshot() []QueueEntry {
	if q.services.Queue != nil {
		return q.services.Queue.Snapshot()
	}
	return append([]QueueEntry(nil), q.queueData...)
}

func (q *queueScreen) sourceIndexByID(id string) int {
	for index, source := range q.sources {
		if source.ID == id {
			return index
		}
	}
	return -1
}

func (p *playbackScreen) sessionState() PlaybackSessionState {
	return PlaybackSessionState{
		Pane:         p.pane,
		ShowInfo:     p.showInfo,
		LyricsScroll: p.lyricsScroll,
	}
}

func (p *playbackScreen) applySessionState(state PlaybackSessionState, snapshot PlaybackSnapshot) {
	switch state.Pane {
	case PaneLyrics, PaneEQ, PaneVisualizer:
		p.pane = state.Pane
	default:
		p.pane = PaneArtwork
	}
	p.showInfo = state.ShowInfo
	p.lyricsScroll = max(0, state.LyricsScroll)
	if snapshot.Volume < 0 && snapshot.QueueIndex == 0 && snapshot.QueueLength == 0 && snapshot.Track == nil {
		return
	}
	p.snapshot = clonePlaybackSnapshot(snapshot)
}

func (p *playbackScreen) snapshotForSession() PlaybackSnapshot {
	p.refreshSnapshot()
	return clonePlaybackSnapshot(p.snapshot)
}

func (f queueFocus) sessionName() string {
	switch f {
	case focusSources:
		return "sources"
	case focusModes:
		return "modes"
	case focusBrowser:
		return "browser"
	default:
		return "search"
	}
}

func queueFocusFromSession(raw string) queueFocus {
	switch raw {
	case "sources":
		return focusSources
	case "modes":
		return focusModes
	case "browser":
		return focusBrowser
	default:
		return focusSearch
	}
}

func cloneQueueEntries(entries []QueueEntry) []QueueEntry {
	if len(entries) == 0 {
		return nil
	}
	cloned := make([]QueueEntry, len(entries))
	copy(cloned, entries)
	return cloned
}

func cloneSearchResults(results []SearchResult) []SearchResult {
	if len(results) == 0 {
		return nil
	}
	cloned := make([]SearchResult, len(results))
	for index, result := range results {
		cloned[index] = result
		cloned[index].CollectionItems = cloneQueueEntries(result.CollectionItems)
	}
	return cloned
}

func cloneExpandedCollections(collections map[string][]SearchResult) map[string][]SearchResult {
	if len(collections) == 0 {
		return nil
	}
	cloned := make(map[string][]SearchResult, len(collections))
	for key, results := range collections {
		cloned[key] = cloneSearchResults(results)
	}
	return cloned
}

func cloneTrack(track *TrackInfo) *TrackInfo {
	if track == nil {
		return nil
	}
	cloned := *track
	return &cloned
}

func clonePlaybackSnapshot(snapshot PlaybackSnapshot) PlaybackSnapshot {
	cloned := snapshot
	cloned.Track = cloneTrack(snapshot.Track)
	return cloned
}

func (s SessionSnapshot) String() string {
	return fmt.Sprintf("%s:%t:%t:%s:%s", s.Mode.String(), s.CompactMode, s.ShowHelp, s.Queue.SourceID, s.Playback.Pane.String())
}
