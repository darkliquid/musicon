package audio

import (
	"testing"
	"time"

	teaui "github.com/darkliquid/musicon/internal/ui"
	"github.com/darkliquid/musicon/pkg/coverart"
	"github.com/gopxl/beep"
)

type stubStream struct {
	length   int
	position int
}

func (s *stubStream) Stream(samples [][2]float64) (int, bool) { return 0, false }
func (s *stubStream) Err() error                              { return nil }
func (s *stubStream) Len() int                                { return s.length }
func (s *stubStream) Position() int                           { return s.position }
func (s *stubStream) Seek(p int) error {
	s.position = p
	return nil
}
func (s *stubStream) Close() error { return nil }

func TestEngineQueueSnapshot(t *testing.T) {
	engine := NewEngine(Options{})
	defer engine.Close()

	queue := engine.QueueService()
	if err := queue.Add(teaui.SearchResult{ID: "one", Title: "First", Duration: 3 * time.Second}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if err := queue.Add(teaui.SearchResult{ID: "two", Title: "Second", Duration: 4 * time.Second}); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	snapshot := queue.Snapshot()
	if got := len(snapshot); got != 2 {
		t.Fatalf("expected queue length 2, got %d", got)
	}
	if snapshot[0].Title != "First" || snapshot[1].Title != "Second" {
		t.Fatalf("unexpected queue snapshot: %#v", snapshot)
	}
}

func TestEngineToggleFlags(t *testing.T) {
	engine := NewEngine(Options{})
	defer engine.Close()

	playback := engine.PlaybackService()
	if err := playback.SetRepeat(true); err != nil {
		t.Fatalf("set repeat failed: %v", err)
	}
	if err := playback.SetStream(true); err != nil {
		t.Fatalf("set stream failed: %v", err)
	}
	if err := playback.AdjustVolume(-25); err != nil {
		t.Fatalf("adjust volume failed: %v", err)
	}

	snapshot := playback.Snapshot()
	if !snapshot.Repeat || !snapshot.Stream {
		t.Fatalf("expected repeat and stream enabled, got %#v", snapshot)
	}
	if snapshot.Volume != 45 {
		t.Fatalf("expected volume 45, got %d", snapshot.Volume)
	}
}

func TestEngineTogglePauseWithoutResolverFails(t *testing.T) {
	engine := NewEngine(Options{})
	defer engine.Close()

	queue := engine.QueueService()
	if err := queue.Add(teaui.SearchResult{ID: "one", Title: "First"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if err := engine.PlaybackService().TogglePause(); err == nil {
		t.Fatal("expected toggle pause to fail without resolver")
	}
}

func TestEngineQueueSnapshotPreservesArtworkMetadata(t *testing.T) {
	engine := NewEngine(Options{})
	defer engine.Close()

	embedded := &coverart.Image{Data: []byte("img"), MIMEType: "image/jpeg"}
	if err := engine.QueueService().Add(teaui.SearchResult{
		ID:    "one",
		Title: "First",
		Artwork: coverart.Metadata{
			IDs: coverart.IDs{
				SpotifyTrackID: "spotify-track",
			},
			Local: &coverart.LocalMetadata{
				AudioPath: "/music/first.mp3",
				Embedded:  embedded,
			},
		},
	}); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	snapshot := engine.QueueService().Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("expected one queue item, got %d", len(snapshot))
	}
	if snapshot[0].Artwork.IDs.SpotifyTrackID != "spotify-track" {
		t.Fatalf("expected artwork ids preserved, got %#v", snapshot[0].Artwork.IDs)
	}
	if snapshot[0].Artwork.Local == nil || snapshot[0].Artwork.Local.AudioPath != "/music/first.mp3" || snapshot[0].Artwork.Local.Embedded != embedded {
		t.Fatalf("expected local artwork metadata preserved, got %#v", snapshot[0].Artwork.Local)
	}
}

func TestPrepareTrackInfoMergesQueueArtworkMetadata(t *testing.T) {
	embedded := &coverart.Image{Data: []byte("img"), MIMEType: "image/jpeg"}
	entry := teaui.QueueEntry{
		ID:     "one",
		Title:  "Queued Song",
		Source: "local",
		Artwork: coverart.Metadata{
			IDs: coverart.IDs{
				SpotifyTrackID:       "spotify-track",
				MusicBrainzReleaseID: "mb-release",
			},
			Local: &coverart.LocalMetadata{
				AudioPath: "/music/song.mp3",
				Embedded:  embedded,
			},
		},
	}
	info := prepareTrackInfo(entry, ResolvedTrack{
		Info: teaui.TrackInfo{
			Title:  "Resolved Song",
			Artist: "Resolved Artist",
			Album:  "Resolved Album",
			Artwork: coverart.Metadata{
				IDs: coverart.IDs{
					SpotifyAlbumID: "spotify-album",
				},
			},
		},
		Format: beep.Format{SampleRate: 48_000, NumChannels: 2, Precision: 2},
		Stream: &stubStream{length: 48_000},
	})

	metadata := info.CoverArtMetadata()
	if metadata.IDs.SpotifyAlbumID != "spotify-album" || metadata.IDs.SpotifyTrackID != "spotify-track" || metadata.IDs.MusicBrainzReleaseID != "mb-release" {
		t.Fatalf("expected merged artwork ids, got %#v", metadata.IDs)
	}
	if metadata.Local == nil || metadata.Local.AudioPath != "/music/song.mp3" || metadata.Local.Embedded != embedded {
		t.Fatalf("expected merged local metadata, got %#v", metadata.Local)
	}
	if metadata.Title != "Resolved Song" || metadata.Artist != "Resolved Artist" || metadata.Album != "Resolved Album" {
		t.Fatalf("expected resolved track labels in cover-art metadata, got %#v", metadata)
	}
	if info.Duration != time.Second {
		t.Fatalf("expected derived duration from resolved stream, got %v", info.Duration)
	}
}
