package ui

import "testing"

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
