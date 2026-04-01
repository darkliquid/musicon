package audio

import (
	"errors"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"

	teaui "github.com/darkliquid/musicon/internal/ui"
	"github.com/darkliquid/musicon/pkg/coverart"
	"github.com/gopxl/beep"
)

type stubStream struct {
	length   int
	position int
	seekErr  error
	closed   bool
}

func (s *stubStream) Stream(samples [][2]float64) (int, bool) { return 0, false }
func (s *stubStream) Err() error                              { return nil }
func (s *stubStream) Len() int                                { return s.length }
func (s *stubStream) Position() int                           { return s.position }
func (s *stubStream) Seek(p int) error {
	if s.seekErr != nil {
		return s.seekErr
	}
	s.position = p
	return nil
}
func (s *stubStream) Close() error { s.closed = true; return nil }

type replacementStubStream struct {
	stubStream
	replacement     *stubStream
	prepareTarget   int
	prepareCalls    int
	prepareErr      error
	replacementMade bool
}

func (s *replacementStubStream) PrepareReplacement(target int) (beep.StreamSeekCloser, error) {
	s.prepareCalls++
	s.prepareTarget = target
	if s.prepareErr != nil {
		return nil, s.prepareErr
	}
	s.replacementMade = true
	return s.replacement, nil
}

type restoreTestResolver struct {
	stream *stubStream
}

func (r restoreTestResolver) Resolve(entry teaui.QueueEntry) (ResolvedTrack, error) {
	stream := r.stream
	if stream == nil {
		stream = &stubStream{length: defaultSampleRate.N(3 * time.Minute)}
	}
	return ResolvedTrack{
		Info: teaui.TrackInfo{
			ID:       entry.ID,
			Title:    entry.Title,
			Artist:   "Artist",
			Album:    "Album",
			Source:   entry.Source,
			Duration: 3 * time.Minute,
			Artwork:  entry.Artwork,
		},
		Format: beep.Format{SampleRate: defaultSampleRate, NumChannels: 2, Precision: 2},
		Stream: stream,
	}, nil
}

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

func TestEngineMoveQueueEntryReordersSnapshot(t *testing.T) {
	engine := NewEngine(Options{})
	defer engine.Close()

	queue := engine.QueueService()
	_ = queue.Add(teaui.SearchResult{ID: "one", Title: "First"})
	_ = queue.Add(teaui.SearchResult{ID: "two", Title: "Second"})
	_ = queue.Add(teaui.SearchResult{ID: "three", Title: "Third"})

	if err := queue.Move("one", 1); err != nil {
		t.Fatalf("move failed: %v", err)
	}

	snapshot := queue.Snapshot()
	if got, want := []string{snapshot[0].ID, snapshot[1].ID, snapshot[2].ID}, []string{"two", "one", "three"}; got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("unexpected queue order: %#v", got)
	}
}

func TestSelectSpeakerBackends(t *testing.T) {
	backends, err := selectSpeakerBackends("alsa")
	if err != nil {
		t.Fatalf("select backends failed: %v", err)
	}
	if len(backends) != 1 {
		t.Fatalf("expected single backend, got %#v", backends)
	}

	backends, err = selectSpeakerBackends("auto")
	if err != nil {
		t.Fatalf("select auto backends failed: %v", err)
	}
	if backends != nil {
		t.Fatalf("expected nil backends for auto, got %#v", backends)
	}

	if _, err := selectSpeakerBackends("mystery"); err == nil {
		t.Fatal("expected unsupported backend error")
	}
}

func TestBackendCandidatesMatchPlatform(t *testing.T) {
	candidates := backendCandidates()
	if len(candidates) == 0 {
		t.Fatal("expected backend candidates")
	}

	got := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		got = append(got, candidate.name)
	}

	switch runtime.GOOS {
	case "windows":
		if got[0] != "wasapi" {
			t.Fatalf("expected windows backends, got %#v", got)
		}
	case "darwin":
		if len(got) != 1 || got[0] != "coreaudio" {
			t.Fatalf("expected darwin backend, got %#v", got)
		}
	case "freebsd":
		if got[0] != "oss" {
			t.Fatalf("expected freebsd backends, got %#v", got)
		}
	case "netbsd":
		if len(got) != 1 || got[0] != "audio4" {
			t.Fatalf("expected netbsd backend, got %#v", got)
		}
	default:
		if got[0] != "pulse" || got[1] != "alsa" || got[2] != "jack" {
			t.Fatalf("expected unix backends, got %#v", got)
		}
	}
}

func TestCanonicalBackendNameNormalizesAliases(t *testing.T) {
	cases := map[string]string{
		"":             "auto",
		"auto":         "auto",
		"pulseaudio":   "pulse",
		"pulse":        "pulse",
		"directsound":  "dsound",
		"dsound":       "dsound",
		" CoreAudio  ": "coreaudio",
		"mystery":      "mystery",
	}

	for raw, want := range cases {
		if got := CanonicalBackendName(raw); got != want {
			t.Fatalf("canonical backend for %q: got %q want %q", raw, got, want)
		}
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

func TestEngineRestoreStateSeedsQueueAndPlaybackSnapshot(t *testing.T) {
	engine := NewEngine(Options{})
	defer engine.Close()

	restore := RestoreState{
		Queue: []teaui.QueueEntry{
			{ID: "one", Title: "First", Source: "local", Duration: 3 * time.Minute},
			{ID: "two", Title: "Second", Source: "local", Duration: 2 * time.Minute},
		},
		Playback: teaui.PlaybackSnapshot{
			Track: &teaui.TrackInfo{
				ID:       "two",
				Title:    "Second",
				Artist:   "Artist",
				Album:    "Album",
				Source:   "local",
				Duration: 2 * time.Minute,
			},
			Position:   95 * time.Second,
			Duration:   2 * time.Minute,
			QueueIndex: 1,
			Repeat:     true,
			Stream:     true,
			Volume:     33,
		},
	}
	if err := engine.RestoreState(restore); err != nil {
		t.Fatalf("restore state failed: %v", err)
	}

	snapshot := engine.PlaybackSnapshot()
	if snapshot.Track == nil || snapshot.Track.ID != "two" {
		t.Fatalf("expected restored track, got %#v", snapshot.Track)
	}
	if !snapshot.Paused {
		t.Fatal("expected restored snapshot to stay paused until resumed")
	}
	if snapshot.Position != 95*time.Second || snapshot.Duration != 2*time.Minute {
		t.Fatalf("expected restored timing, got position=%s duration=%s", snapshot.Position, snapshot.Duration)
	}
	if !snapshot.Repeat || !snapshot.Stream || snapshot.Volume != 33 || snapshot.QueueIndex != 1 || snapshot.QueueLength != 2 {
		t.Fatalf("expected restored flags and queue index, got %#v", snapshot)
	}
}

func TestEngineTogglePauseResumesRestoredTrackFromSavedPosition(t *testing.T) {
	stream := &stubStream{length: defaultSampleRate.N(3 * time.Minute)}
	engine := NewEngine(Options{Resolver: restoreTestResolver{stream: stream}})
	defer engine.Close()

	if err := engine.RestoreState(RestoreState{
		Queue: []teaui.QueueEntry{
			{ID: "one", Title: "First", Source: "local", Duration: 3 * time.Minute},
		},
		Playback: teaui.PlaybackSnapshot{
			Track:      &teaui.TrackInfo{ID: "one", Title: "First", Source: "local", Duration: 3 * time.Minute},
			Position:   70 * time.Second,
			Duration:   3 * time.Minute,
			QueueIndex: 0,
		},
	}); err != nil {
		t.Fatalf("restore state failed: %v", err)
	}

	if err := engine.TogglePause(); err != nil {
		t.Fatalf("toggle pause failed: %v", err)
	}

	snapshot := engine.PlaybackSnapshot()
	if snapshot.Track == nil || snapshot.Track.ID != "one" {
		t.Fatalf("expected active restored track after resume, got %#v", snapshot.Track)
	}
	if snapshot.Paused {
		t.Fatal("expected restored track to resume playing after toggle")
	}
	if got := stream.position; got != defaultSampleRate.N(70*time.Second) {
		t.Fatalf("expected restored stream to seek to saved position, got %d", got)
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

func TestEngineSeekToSeeksActiveStreamInPlace(t *testing.T) {
	engine := NewEngine(Options{})
	defer engine.Close()

	stream := &stubStream{length: 48_000 * 60}
	entry := teaui.QueueEntry{ID: "one", Title: "First", Source: "youtube"}
	info := teaui.TrackInfo{ID: "one", Title: "First", Source: "youtube"}

	engine.mu.Lock()
	engine.currentIndex = 0
	if err := engine.activateTrackLocked(entry, info, beep.Format{SampleRate: defaultSampleRate, NumChannels: 2, Precision: 2}, stream, false); err != nil {
		engine.mu.Unlock()
		t.Fatalf("activate track failed: %v", err)
	}
	engine.mu.Unlock()

	if err := engine.SeekTo(10 * time.Second); err != nil {
		t.Fatalf("seek failed: %v", err)
	}
	if got := stream.position; got != defaultSampleRate.N(10*time.Second) {
		t.Fatalf("expected in-place seek to sample %d, got %d", defaultSampleRate.N(10*time.Second), got)
	}
}

func TestEngineSeekToSwapsPreparedReplacementStream(t *testing.T) {
	engine := NewEngine(Options{})
	defer engine.Close()

	replacement := &stubStream{length: 48_000 * 60, position: defaultSampleRate.N(25 * time.Second)}
	stream := &replacementStubStream{
		stubStream:  stubStream{length: 48_000 * 60, seekErr: errors.New("out of window")},
		replacement: replacement,
	}
	entry := teaui.QueueEntry{ID: "one", Title: "First", Source: "youtube"}
	info := teaui.TrackInfo{ID: "one", Title: "First", Source: "youtube"}

	engine.mu.Lock()
	engine.currentIndex = 0
	if err := engine.activateTrackLocked(entry, info, beep.Format{SampleRate: defaultSampleRate, NumChannels: 2, Precision: 2}, stream, true); err != nil {
		engine.mu.Unlock()
		t.Fatalf("activate track failed: %v", err)
	}
	engine.mu.Unlock()

	if err := engine.SeekTo(25 * time.Second); err != nil {
		t.Fatalf("seek failed: %v", err)
	}

	if stream.prepareCalls != 1 {
		t.Fatalf("expected one replacement preparation, got %d", stream.prepareCalls)
	}
	if got := stream.prepareTarget; got != defaultSampleRate.N(25*time.Second) {
		t.Fatalf("expected replacement target %d, got %d", defaultSampleRate.N(25*time.Second), got)
	}
	if !stream.closed {
		t.Fatal("expected original stream to be closed after swap")
	}

	snapshot := engine.PlaybackSnapshot()
	if !snapshot.Paused {
		t.Fatal("expected pause state to survive replacement swap")
	}
	if got := snapshot.Position; got != 25*time.Second {
		t.Fatalf("expected replacement position 25s, got %s", got)
	}
}

func TestVisualizationServiceRendersEQFromCapturedSamples(t *testing.T) {
	engine := NewEngine(Options{})
	defer engine.Close()

	engine.visual.Reset(defaultSampleRate)
	samples := make([][2]float64, analysisFFTSize)
	for i := range samples {
		value := math.Sin((2 * math.Pi * 440 * float64(i)) / float64(defaultSampleRate))
		samples[i] = [2]float64{value, value}
	}
	engine.visual.Ingest(samples)

	content, err := engine.VisualizationService().Placeholder(teaui.PaneEQ, 32, 12)
	if err != nil {
		t.Fatalf("render eq failed: %v", err)
	}
	if !containsBraille(content) {
		t.Fatalf("expected eq output to contain braille cells, got %q", content)
	}
}

func TestVisualizationServiceReturnsEmptyWhenIdle(t *testing.T) {
	engine := NewEngine(Options{})
	defer engine.Close()

	content, err := engine.VisualizationService().Placeholder(teaui.PaneEQ, 32, 12)
	if err != nil {
		t.Fatalf("render eq failed: %v", err)
	}
	if content != "" {
		t.Fatalf("expected no visualization content while idle, got %q", content)
	}
}

func TestBrailleRuneMapsMaskIntoUnicodeBrailleBlock(t *testing.T) {
	if got := brailleRune(0x11); got != '⠑' {
		t.Fatalf("expected braille rune ⠑, got %q", got)
	}
}

func TestGradientColorAtInterpolatesStops(t *testing.T) {
	if got := gradientColorAt([]string{"#000000", "#ffffff"}, 0.5); got != "#808080" {
		t.Fatalf("expected midpoint gray, got %q", got)
	}
}

func TestRenderEQBarsUsesBrailleAndGradientColors(t *testing.T) {
	content := renderEQBars([]float64{0.5}, 1, 1)
	if !containsBraille(content) {
		t.Fatalf("expected braille glyph, got %q", content)
	}
	if low, high := gradientColorAt(eqGradientStops, 0), gradientColorAt(eqGradientStops, 1); low == high {
		t.Fatalf("expected gradient endpoints to differ, got low=%q high=%q", low, high)
	}
}

func TestRenderMirrorBarsUsesBrailleAndGradientColors(t *testing.T) {
	content := renderMirrorBars([]float64{0.5}, 1, 2)
	if !containsBraille(content) {
		t.Fatalf("expected mirrored braille glyph, got %q", content)
	}
	lines := strings.Split(content, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two mirrored rows, got %d in %q", len(lines), content)
	}
	if center, edge := gradientColorAt(eqGradientStops, 0), gradientColorAt(eqGradientStops, 1); center == edge {
		t.Fatalf("expected mirrored gradient endpoints to differ, got center=%q edge=%q", center, edge)
	}
}

func containsBraille(content string) bool {
	for _, r := range content {
		if r >= 0x2801 && r <= 0x28FF {
			return true
		}
	}
	return false
}
