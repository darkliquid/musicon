package mpris

import (
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/musicon/internal/ui"
	"github.com/godbus/dbus/v5"
)

type stubPlayback struct {
	snapshot    ui.PlaybackSnapshot
	seekCalls   int
	seekTargets []time.Duration
}

func (s stubPlayback) Snapshot() ui.PlaybackSnapshot { return s.snapshot }
func (s stubPlayback) TogglePause() error            { return nil }
func (s stubPlayback) Previous() error               { return nil }
func (s stubPlayback) Next() error                   { return nil }
func (s *stubPlayback) SeekTo(target time.Duration) error {
	s.seekCalls++
	s.seekTargets = append(s.seekTargets, target)
	s.snapshot.Position = target
	return nil
}
func (s stubPlayback) AdjustVolume(delta int) error { _ = delta; return nil }
func (s stubPlayback) SetRepeat(repeat bool) error  { _ = repeat; return nil }
func (s stubPlayback) SetStream(stream bool) error  { _ = stream; return nil }

func TestPlaybackStatus(t *testing.T) {
	if got := playbackStatus(ui.PlaybackSnapshot{}); got != "Stopped" {
		t.Fatalf("expected Stopped, got %q", got)
	}
	if got := playbackStatus(ui.PlaybackSnapshot{Track: &ui.TrackInfo{Title: "x"}, Paused: true}); got != "Paused" {
		t.Fatalf("expected Paused, got %q", got)
	}
	if got := playbackStatus(ui.PlaybackSnapshot{Track: &ui.TrackInfo{Title: "x"}}); got != "Playing" {
		t.Fatalf("expected Playing, got %q", got)
	}
}

func TestTrackObjectPathSanitizes(t *testing.T) {
	got := string(trackObjectPath("bad/id with spaces"))
	want := "/org/mpris/MediaPlayer2/track/bad_id_with_spaces"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestMetadataIncludesTrackFields(t *testing.T) {
	snapshot := ui.PlaybackSnapshot{
		Duration: 3 * time.Minute,
		Track:    &ui.TrackInfo{ID: "abc", Title: "Song", Artist: "Artist", Album: "Album", Source: "Demo"},
	}

	data := metadata(snapshot)
	if got := data["xesam:title"].Value().(string); got != "Song" {
		t.Fatalf("expected title Song, got %q", got)
	}
	artists := data["xesam:artist"].Value().([]string)
	if len(artists) != 1 || artists[0] != "Artist" {
		t.Fatalf("unexpected artists %#v", artists)
	}
}

func TestBridgeSeekFailsExplicitly(t *testing.T) {
	playback := &stubPlayback{
		snapshot: ui.PlaybackSnapshot{
			Position: 30 * time.Second,
			Track:    &ui.TrackInfo{ID: "abc", Title: "Song"},
		},
	}
	bridge := NewBridge(playback)

	seek, ok := bridge.playerMethods()["Seek"].(func(int64) *dbus.Error)
	if !ok {
		t.Fatal("expected Seek method in player method table")
	}

	err := seek(int64((5 * time.Second) / time.Microsecond))
	if err == nil || !strings.Contains(err.Error(), "seek is not supported") {
		t.Fatalf("expected explicit seek unsupported error, got %v", err)
	}
	if playback.seekCalls != 0 {
		t.Fatalf("expected no seek calls, got %d", playback.seekCalls)
	}
}

func TestBridgeSetPositionFailsExplicitly(t *testing.T) {
	playback := &stubPlayback{
		snapshot: ui.PlaybackSnapshot{
			Position: 12 * time.Second,
			Track:    &ui.TrackInfo{ID: "abc", Title: "Song"},
		},
	}
	bridge := NewBridge(playback)

	setPosition, ok := bridge.playerMethods()["SetPosition"].(func(dbus.ObjectPath, int64) *dbus.Error)
	if !ok {
		t.Fatal("expected SetPosition method in player method table")
	}

	err := setPosition(trackObjectPath("abc"), int64((42*time.Second)/time.Microsecond))
	if err == nil || !strings.Contains(err.Error(), "seek is not supported") {
		t.Fatalf("expected explicit set position unsupported error, got %v", err)
	}
	if playback.seekCalls != 0 {
		t.Fatalf("expected no seek calls, got %d", playback.seekCalls)
	}
}
