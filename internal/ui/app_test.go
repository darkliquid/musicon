package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

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
