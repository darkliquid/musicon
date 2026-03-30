package mpris

import (
	"testing"
	"time"

	"github.com/darkliquid/musicon/internal/ui"
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

func TestBridgeSeekUsesRelativeTargetAgainstSnapshotPosition(t *testing.T) {
	playback := &stubPlayback{
		snapshot: ui.PlaybackSnapshot{
			Position: 30 * time.Second,
			Track:    &ui.TrackInfo{ID: "abc", Title: "Song"},
		},
	}
	bridge := NewBridge(playback)

	if err := bridge.Seek(int64((5 * time.Second) / time.Microsecond)); err != nil {
		t.Fatalf("seek failed: %v", err)
	}
	if playback.seekCalls != 1 {
		t.Fatalf("expected one seek call, got %d", playback.seekCalls)
	}
	if got := playback.seekTargets[0]; got != 35*time.Second {
		t.Fatalf("expected relative seek target 35s, got %s", got)
	}
}

func TestBridgeSetPositionUsesAbsoluteTargetForCurrentTrack(t *testing.T) {
	playback := &stubPlayback{
		snapshot: ui.PlaybackSnapshot{
			Position: 12 * time.Second,
			Track:    &ui.TrackInfo{ID: "abc", Title: "Song"},
		},
	}
	bridge := NewBridge(playback)

	if err := bridge.SetPosition(trackObjectPath("abc"), int64((42*time.Second)/time.Microsecond)); err != nil {
		t.Fatalf("set position failed: %v", err)
	}
	if playback.seekCalls != 1 {
		t.Fatalf("expected one seek call, got %d", playback.seekCalls)
	}
	if got := playback.seekTargets[0]; got != 42*time.Second {
		t.Fatalf("expected absolute seek target 42s, got %s", got)
	}
}
