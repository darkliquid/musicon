package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type appTestQueueService struct {
	entries []QueueEntry
}

func (s *appTestQueueService) Snapshot() []QueueEntry { return append([]QueueEntry(nil), s.entries...) }
func (s *appTestQueueService) Add(SearchResult) error { return nil }
func (s *appTestQueueService) Move(string, int) error { return nil }
func (s *appTestQueueService) Remove(string) error    { return nil }
func (s *appTestQueueService) RemoveGroup(string) error {
	return nil
}
func (s *appTestQueueService) Clear() error { return nil }

type appTestPlaybackService struct {
	snapshot PlaybackSnapshot
}

func (s *appTestPlaybackService) Snapshot() PlaybackSnapshot { return s.snapshot }
func (s *appTestPlaybackService) TogglePause() error         { return nil }
func (s *appTestPlaybackService) Previous() error            { return nil }
func (s *appTestPlaybackService) Next() error                { return nil }
func (s *appTestPlaybackService) SeekTo(time.Duration) error { return nil }
func (s *appTestPlaybackService) AdjustVolume(int) error     { return nil }
func (s *appTestPlaybackService) SetRepeat(bool) error       { return nil }
func (s *appTestPlaybackService) SetStream(bool) error       { return nil }

type appTestSessionStore struct {
	snapshots []SessionSnapshot
}

func (s *appTestSessionStore) Save(snapshot SessionSnapshot) error {
	s.snapshots = append(s.snapshots, snapshot)
	return nil
}

func TestRequestWindowSizeCmdReturnsWindowSizeRequest(t *testing.T) {
	msg := requestWindowSizeCmd()()
	if got := fmt.Sprintf("%T", msg); got != "tea.windowSizeMsg" {
		t.Fatalf("expected startup window-size request, got %s", got)
	}
}

func TestRootModelInitRequestsWindowSize(t *testing.T) {
	model := &rootModel{
		queue:    newQueueScreen(Services{}),
		playback: newPlaybackScreen(Services{}, AlbumArtOptions{}),
	}

	msg := model.Init()()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected batched startup commands, got %T", msg)
	}

	foundWindowSize := false
	for _, cmd := range batch {
		if cmd == nil {
			continue
		}
		if got := fmt.Sprintf("%T", cmd()); got == "tea.windowSizeMsg" {
			foundWindowSize = true
			break
		}
	}

	if !foundWindowSize {
		t.Fatal("expected Init to request terminal dimensions")
	}
}

func TestTerminalSizeFromEnv(t *testing.T) {
	t.Setenv("COLUMNS", "132")
	t.Setenv("LINES", "41")

	width, height, ok := terminalSizeFromEnv()
	if !ok {
		t.Fatal("expected environment terminal size to be detected")
	}
	if width != 132 || height != 41 {
		t.Fatalf("expected 132x41, got %dx%d", width, height)
	}
}

func TestTerminalSizeFromEnvRejectsInvalidValues(t *testing.T) {
	t.Setenv("COLUMNS", "-1")
	t.Setenv("LINES", "0")

	_, _, ok := terminalSizeFromEnv()
	if ok {
		t.Fatal("expected invalid environment dimensions to be rejected")
	}
}

func TestTerminalCellWidthRatioDefault(t *testing.T) {
	t.Setenv("MUSICON_CELL_WIDTH_RATIO", "")

	if got := terminalCellWidthRatio(); got <= 0 {
		t.Fatalf("expected positive default cell width ratio, got %v", got)
	}
}

func TestTerminalCellWidthRatioFromEnv(t *testing.T) {
	t.Setenv("MUSICON_CELL_WIDTH_RATIO", "0.6")

	if got := terminalCellWidthRatio(); got != 0.6 {
		t.Fatalf("expected env cell width ratio 0.6, got %v", got)
	}
}

func TestNormalizedOptionsUsesConfiguredStartModeAndCellRatio(t *testing.T) {
	options := normalizedOptions(Options{
		StartMode:      ModePlayback,
		CellWidthRatio: 0.75,
	})
	if options.StartMode != ModePlayback {
		t.Fatalf("expected playback start mode, got %v", options.StartMode)
	}
	if options.CellWidthRatio != 0.75 {
		t.Fatalf("expected configured cell width ratio, got %v", options.CellWidthRatio)
	}
}

func TestNormalizedOptionsUsesConfiguredKeybinds(t *testing.T) {
	options := normalizedOptions(Options{
		Keybinds: KeybindOptions{
			Global: GlobalKeybindOptions{
				ToggleMode: []string{"ctrl+o"},
			},
		},
	})

	if len(options.Keybinds.Global.ToggleMode) != 1 || options.Keybinds.Global.ToggleMode[0] != "ctrl+o" {
		t.Fatalf("expected configured toggle-mode keybind, got %#v", options.Keybinds.Global.ToggleMode)
	}
	if len(options.Keybinds.Global.Quit) != 1 || options.Keybinds.Global.Quit[0] != "ctrl+c" {
		t.Fatalf("expected default quit keybind, got %#v", options.Keybinds.Global.Quit)
	}
}

func TestRootModelUsesConfiguredModeToggleBinding(t *testing.T) {
	model := &rootModel{
		width:          80,
		height:         40,
		cellWidthRatio: 1,
		mode:           ModeQueue,
		keymap: normalizedKeyMap(KeybindOptions{
			Global: GlobalKeybindOptions{
				ToggleMode: []string{"ctrl+o"},
			},
		}),
		queue:    newQueueScreen(Services{}),
		playback: newPlaybackScreen(Services{}, AlbumArtOptions{}),
	}

	next, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
	updated := next.(*rootModel)
	if updated.mode != ModePlayback {
		t.Fatalf("expected custom toggle-mode binding to switch to playback, got %v", updated.mode)
	}
}

func TestLayoutCheckAccountsForNonSquareTerminalCells(t *testing.T) {
	model := &rootModel{width: 120, height: 40, cellWidthRatio: 0.5}

	check := model.layoutCheck()
	if !check.Fits() {
		t.Fatalf("expected 120x40 terminal to fit with 0.5 cell width ratio, got %#v", check)
	}
	if check.Viewport.Width != 80 || check.Viewport.Height != 40 {
		t.Fatalf("expected 80x40 viewport, got %#v", check.Viewport)
	}
}

func TestMakeViewSeparatesContentAndWindowTitle(t *testing.T) {
	model := &rootModel{}

	view := model.makeView("visible content", "My\x1b Title\n")
	if view.Content != "visible content" {
		t.Fatalf("expected visible content to stay unchanged, got %q", view.Content)
	}
	if strings.Contains(view.Content, "\x1b]") {
		t.Fatalf("expected content to be free of OSC control sequences, got %q", view.Content)
	}
	if view.WindowTitle != "My Title" {
		t.Fatalf("expected sanitized window title, got %q", view.WindowTitle)
	}
	if !view.AltScreen {
		t.Fatal("expected alt screen to remain enabled")
	}
}

func TestTerminalTitleIdle(t *testing.T) {
	model := &rootModel{
		mode:     ModeQueue,
		playback: newPlaybackScreen(Services{}, AlbumArtOptions{}),
	}

	got := model.terminalTitle()
	want := "Musicon - Queue - Idle"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestTerminalTitlePlaybackTrack(t *testing.T) {
	model := &rootModel{
		mode:     ModePlayback,
		playback: newPlaybackScreen(Services{}, AlbumArtOptions{}),
	}
	model.playback.snapshot = PlaybackSnapshot{
		Paused: false,
		Track: &TrackInfo{
			Title:  "Track",
			Artist: "Artist",
		},
	}

	got := model.terminalTitle()
	want := "Musicon - Playback - Artist - Track - Playing"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSanitizeTitleRemovesControlCharacters(t *testing.T) {
	got := sanitizeTitle("bad\x1btitle\nwith\tbells\x07")
	want := "badtitle with bells"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestRootViewOmitsOuterChrome(t *testing.T) {
	model := &rootModel{
		width:          80,
		height:         40,
		cellWidthRatio: 1,
		mode:           ModeQueue,
		queue:          newQueueScreen(Services{}),
		playback:       newPlaybackScreen(Services{}, AlbumArtOptions{}),
	}

	view := model.View().Content
	if strings.Contains(view, "tab Queue") || strings.Contains(view, "ctrl+c exits") {
		t.Fatalf("expected outer chrome to be removed, got %q", view)
	}
}

func TestRootViewOverlaysHelpInsideSquare(t *testing.T) {
	model := &rootModel{
		width:          80,
		height:         40,
		cellWidthRatio: 1,
		mode:           ModeQueue,
		showHelp:       true,
		queue:          newQueueScreen(Services{}),
		playback:       newPlaybackScreen(Services{}, AlbumArtOptions{}),
	}

	view := model.View().Content
	if !strings.Contains(view, "Queue help") {
		t.Fatalf("expected queue help overlay, got %q", view)
	}
}

func TestApplyRestoredSessionRestoresQueueAndPlaybackState(t *testing.T) {
	queue := &appTestQueueService{
		entries: []QueueEntry{{ID: "queued-1", Title: "Queued", Source: "local"}},
	}
	playback := &appTestPlaybackService{
		snapshot: PlaybackSnapshot{
			Track:       &TrackInfo{ID: "queued-1", Title: "Queued", Source: "local"},
			Paused:      true,
			Position:    42 * time.Second,
			Duration:    3 * time.Minute,
			QueueIndex:  0,
			QueueLength: 1,
		},
	}
	model := &rootModel{
		services: Services{Queue: queue, Playback: playback},
		mode:     ModeQueue,
		queue:    newQueueScreen(Services{Queue: queue, Playback: playback}),
		playback: newPlaybackScreen(Services{Playback: playback}, AlbumArtOptions{}),
	}

	model.applyRestoredSession(&SessionSnapshot{
		Mode:     ModePlayback,
		ShowHelp: true,
		Queue: QueueSessionState{
			SourceID:       "all",
			SearchMode:     SearchModeTracks,
			Query:          "boards of canada",
			Focus:          "browser",
			SelectedRowKey: "result:result-2",
			SearchResults: []SearchResult{
				{ID: "result-1", Title: "Dayvan Cowboy", Source: "local", Kind: MediaTrack},
				{ID: "result-2", Title: "Roygbiv", Source: "local", Kind: MediaTrack},
			},
		},
		Playback: PlaybackSessionState{
			Pane:         PaneLyrics,
			ShowInfo:     true,
			LyricsScroll: 3,
		},
		PlaybackSnapshot: playback.snapshot,
	})

	if model.mode != ModePlayback || !model.showHelp {
		t.Fatalf("expected playback mode with help restored, got mode=%v help=%t", model.mode, model.showHelp)
	}
	if got := model.queue.searchInput.Value(); got != "boards of canada" {
		t.Fatalf("expected restored query, got %q", got)
	}
	if model.queue.focus != focusBrowser {
		t.Fatalf("expected browser focus, got %v", model.queue.focus)
	}
	if got := model.queue.browser.SelectedIndex(); got != 2 {
		t.Fatalf("expected restored selection on second result, got %d", got)
	}
	if model.playback.pane != PaneLyrics || !model.playback.showInfo || model.playback.lyricsScroll != 3 {
		t.Fatalf("expected playback pane/info/scroll restored, got pane=%v info=%t scroll=%d", model.playback.pane, model.playback.showInfo, model.playback.lyricsScroll)
	}
	if model.status != "Restored previous session." {
		t.Fatalf("expected restored status, got %q", model.status)
	}
}

func TestPersistSessionSavesQueueAndPlaybackContext(t *testing.T) {
	queue := &appTestQueueService{
		entries: []QueueEntry{{ID: "queued-1", Title: "Queued", Source: "local"}},
	}
	playback := &appTestPlaybackService{
		snapshot: PlaybackSnapshot{
			Track:       &TrackInfo{ID: "queued-1", Title: "Queued", Source: "local"},
			Paused:      true,
			Position:    90 * time.Second,
			Duration:    3 * time.Minute,
			QueueIndex:  0,
			QueueLength: 1,
			Volume:      55,
		},
	}
	store := &appTestSessionStore{}
	model := &rootModel{
		services:     Services{Queue: queue, Playback: playback},
		sessionStore: store,
		mode:         ModePlayback,
		showHelp:     true,
		queue:        newQueueScreen(Services{Queue: queue, Playback: playback}),
		playback:     newPlaybackScreen(Services{Playback: playback}, AlbumArtOptions{}),
	}
	model.queue.searchInput.SetValue("aphex twin")
	model.queue.resultData = []SearchResult{{ID: "result-1", Title: "Xtal", Source: "local", Kind: MediaTrack}}
	model.queue.rebuildBrowser()
	model.playback.pane = PaneVisualizer
	model.playback.showInfo = true
	model.playback.snapshot = playback.snapshot

	if err := model.persistSession(true, false); err != nil {
		t.Fatalf("persist session failed: %v", err)
	}
	if len(store.snapshots) != 1 {
		t.Fatalf("expected one persisted snapshot, got %d", len(store.snapshots))
	}
	snapshot := store.snapshots[0]
	if snapshot.Mode != ModePlayback || !snapshot.ShowHelp {
		t.Fatalf("expected mode/help persisted, got %#v", snapshot)
	}
	if snapshot.Queue.Query != "aphex twin" || len(snapshot.Queue.SearchResults) != 1 {
		t.Fatalf("expected queue search persisted, got %#v", snapshot.Queue)
	}
	if len(snapshot.QueueEntries) != 1 || snapshot.QueueEntries[0].ID != "queued-1" {
		t.Fatalf("expected queue entries persisted, got %#v", snapshot.QueueEntries)
	}
	if snapshot.Playback.Pane != PaneVisualizer || !snapshot.Playback.ShowInfo {
		t.Fatalf("expected playback UI persisted, got %#v", snapshot.Playback)
	}
	if snapshot.PlaybackSnapshot.Track == nil || snapshot.PlaybackSnapshot.Track.ID != "queued-1" || snapshot.PlaybackSnapshot.Volume != 55 {
		t.Fatalf("expected playback snapshot persisted, got %#v", snapshot.PlaybackSnapshot)
	}
}
