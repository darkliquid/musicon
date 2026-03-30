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
		playback: newPlaybackScreen(Services{}),
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

	if got := terminalCellWidthRatio(); got != 0.5 {
		t.Fatalf("expected default cell width ratio 0.5, got %v", got)
	}
}

func TestTerminalCellWidthRatioFromEnv(t *testing.T) {
	t.Setenv("MUSICON_CELL_WIDTH_RATIO", "0.6")

	if got := terminalCellWidthRatio(); got != 0.6 {
		t.Fatalf("expected env cell width ratio 0.6, got %v", got)
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
		playback: newPlaybackScreen(Services{}),
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
		playback: newPlaybackScreen(Services{}),
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
