package ui

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/darkliquid/musicon/pkg/components"
	"github.com/darkliquid/musicon/pkg/coverart"
	"github.com/darkliquid/musicon/pkg/lyrics"
)

type playbackTestService struct {
	mu            sync.Mutex
	snapshot      PlaybackSnapshot
	togglePauseCh chan struct{}
	toggleErr     error
	toggleCalls   int
	seekErr       error
	seekCalls     int
	seekTargets   []time.Duration
}

func (s *playbackTestService) Snapshot() PlaybackSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot
}
func (s *playbackTestService) TogglePause() error {
	s.mu.Lock()
	s.toggleCalls++
	wait := s.togglePauseCh
	err := s.toggleErr
	snapshot := s.snapshot
	s.mu.Unlock()
	if wait != nil {
		<-wait
	}
	if err != nil {
		return err
	}
	snapshot.Paused = !snapshot.Paused
	s.mu.Lock()
	s.snapshot = snapshot
	s.mu.Unlock()
	return nil
}
func (s *playbackTestService) Previous() error { return nil }
func (s *playbackTestService) Next() error     { return nil }
func (s *playbackTestService) SeekTo(target time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seekCalls++
	s.seekTargets = append(s.seekTargets, target)
	if s.seekErr != nil {
		return s.seekErr
	}
	s.snapshot.Position = target
	return nil
}
func (s *playbackTestService) AdjustVolume(delta int) error { return nil }
func (s *playbackTestService) SetRepeat(repeat bool) error  { return nil }
func (s *playbackTestService) SetStream(stream bool) error  { return nil }

type artworkTestProvider struct {
	mu     sync.Mutex
	calls  int
	source *components.ImageSource
	block  chan struct{}
}

func (p *artworkTestProvider) Artwork(metadata coverart.Metadata) (*components.ImageSource, error) {
	return p.ArtworkObserved(metadata, nil)
}

func (p *artworkTestProvider) ArtworkObserved(_ coverart.Metadata, report func(ArtworkAttempt)) (*components.ImageSource, error) {
	p.mu.Lock()
	p.calls++
	block := p.block
	source := p.source
	p.mu.Unlock()
	if report != nil {
		report(ArtworkAttempt{Provider: "stub", Status: "trying", Message: "trying provider"})
	}
	if block != nil {
		<-block
	}
	return source, nil
}

type lyricsTestProvider struct {
	mu    sync.Mutex
	calls int
	doc   *lyrics.Document
}

func (p *lyricsTestProvider) Lyrics(lyrics.Request) (*lyrics.Document, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.doc, nil
}

type visualizationTestProvider struct {
	mu      sync.Mutex
	calls   int
	content string
	steps   []string
}

func (p *visualizationTestProvider) Placeholder(PlaybackPane, int, int) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if len(p.steps) > 0 {
		content := p.steps[0]
		p.steps = p.steps[1:]
		return content, nil
	}
	return p.content, nil
}

func TestPlaybackControlsViewOmitsSeekStatus(t *testing.T) {
	screen := newPlaybackScreen(Services{}, AlbumArtOptions{})
	screen.snapshot = PlaybackSnapshot{Volume: 70}

	view := screen.controlsView(48)
	if strings.Contains(view, "seeking:") {
		t.Fatalf("did not expect seek status in controls view, got %q", view)
	}
}

func TestPlaybackUpdateDispatchesTogglePauseAsynchronously(t *testing.T) {
	service := &playbackTestService{
		snapshot:      PlaybackSnapshot{Volume: 70},
		togglePauseCh: make(chan struct{}),
	}
	screen := newPlaybackScreen(Services{Playback: service}, AlbumArtOptions{})

	status, cmd := screen.Update(tea.KeyPressMsg(tea.Key{Code: ' '}))
	if status != "" {
		t.Fatalf("expected no immediate status, got %q", status)
	}
	if cmd == nil {
		t.Fatal("expected async command for playback action")
	}
	if !screen.pending {
		t.Fatal("expected screen to mark action pending")
	}
	secondStatus, secondCmd := screen.Update(tea.KeyPressMsg(tea.Key{Code: ' '}))
	if secondStatus != "" || secondCmd != nil {
		t.Fatalf("expected repeated keypress to be ignored while pending, got status=%q cmd=%v", secondStatus, secondCmd)
	}
	close(service.togglePauseCh)
	msg := cmd()
	finalStatus, finalCmd := screen.Update(msg)
	if finalCmd != nil {
		t.Fatal("expected no follow-up command after action result")
	}
	if finalStatus != "Playback paused." {
		t.Fatalf("unexpected completion status: %q", finalStatus)
	}
	if screen.pending {
		t.Fatal("expected pending action to clear after result")
	}
	if !screen.snapshot.Paused {
		t.Fatal("expected snapshot to reflect toggled pause state")
	}
}

func TestPlaybackUpdateSurfacesAsyncPlaybackErrors(t *testing.T) {
	service := &playbackTestService{
		snapshot:  PlaybackSnapshot{Volume: 70},
		toggleErr: errors.New("toggle failed"),
	}
	screen := newPlaybackScreen(Services{Playback: service}, AlbumArtOptions{})

	_, cmd := screen.Update(tea.KeyPressMsg(tea.Key{Code: ' '}))
	if cmd == nil {
		t.Fatal("expected async command for playback action")
	}
	status, followUp := screen.Update(cmd())
	if followUp != nil {
		t.Fatal("expected no follow-up command after error")
	}
	if status != "toggle failed" {
		t.Fatalf("unexpected error status: %q", status)
	}
	if screen.pending {
		t.Fatal("expected pending action to clear after error")
	}
}

func TestPlaybackUpdateDebouncesSeekUntilTickDeadline(t *testing.T) {
	service := &playbackTestService{
		snapshot: PlaybackSnapshot{
			Volume:   70,
			Position: 30 * time.Second,
			Track:    &TrackInfo{ID: "track-1", Title: "Song"},
		},
	}
	screen := newPlaybackScreen(Services{Playback: service}, AlbumArtOptions{})
	screen.snapshot = service.snapshot

	if got := screen.accumulateSeek(playbackSeekStep); got != "Seek +5s queued." {
		t.Fatalf("unexpected first seek status: %q", got)
	}
	if got := screen.accumulateSeek(playbackSeekStep); got != "Seek +10s queued." {
		t.Fatalf("unexpected second seek status: %q", got)
	}
	if service.seekCalls != 0 {
		t.Fatalf("expected no immediate seek call, got %d", service.seekCalls)
	}

	status, cmd := screen.Update(tickMsg(time.Now()))
	if status != "" || cmd != nil {
		t.Fatalf("expected debounce to suppress early tick, got status=%q cmd=%v", status, cmd)
	}

	screen.seekDeadline = time.Now().Add(-time.Millisecond)
	status, cmd = screen.Update(tickMsg(time.Now()))
	if status != "" {
		t.Fatalf("expected no status when dispatching seek, got %q", status)
	}
	if cmd == nil {
		t.Fatal("expected debounced tick to dispatch seek command")
	}
	if !screen.pending {
		t.Fatal("expected screen to be pending while async seek runs")
	}
	if screen.seekAdjustment != 0 {
		t.Fatalf("expected queued seek to clear after dispatch, got %s", screen.seekAdjustment)
	}

	msg := cmd()
	status, followUp := screen.Update(msg)
	if status != "" || followUp != nil {
		t.Fatalf("unexpected seek completion result: status=%q cmd=%v", status, followUp)
	}
	if screen.pending {
		t.Fatal("expected pending flag cleared after seek completion")
	}
	if got := screen.snapshot.Position; got != 40*time.Second {
		t.Fatalf("expected settled playback position 40s, got %s", got)
	}
	if service.seekCalls != 1 {
		t.Fatalf("expected one seek call, got %d", service.seekCalls)
	}
	if got := service.seekTargets[0]; got != 40*time.Second {
		t.Fatalf("expected seek target 40s, got %s", got)
	}
}

func TestPlaybackArtworkLookupIsCachedAcrossViews(t *testing.T) {
	provider := &artworkTestProvider{
		source: &components.ImageSource{Description: "cached artwork"},
	}
	screen := newPlaybackScreen(Services{Artwork: provider}, AlbumArtOptions{})
	screen.SetSize(40, 20)
	screen.snapshot = PlaybackSnapshot{
		Track: &TrackInfo{
			ID:     "track-1",
			Title:  "Song",
			Artist: "Artist",
			Album:  "Album",
			Source: "youtube",
		},
	}

	_ = screen.View()
	_ = screen.View()

	provider.mu.Lock()
	calls := provider.calls
	provider.mu.Unlock()
	if calls != 1 {
		t.Fatalf("expected artwork provider to be called once, got %d", calls)
	}
}

func TestPlaybackArtworkLookupRestartsWhenArtworkMetadataChanges(t *testing.T) {
	provider := &artworkTestProvider{
		source: &components.ImageSource{Description: "cached artwork"},
	}
	screen := newPlaybackScreen(Services{Artwork: provider}, AlbumArtOptions{})
	screen.SetSize(40, 20)
	screen.snapshot = PlaybackSnapshot{
		Track: &TrackInfo{
			ID:     "track-1",
			Title:  "Song",
			Artist: "Artist",
			Album:  "Album",
			Source: "youtube",
		},
	}

	_ = screen.View()
	screen.snapshot.Track.Artwork = coverart.Metadata{
		IDs: coverart.IDs{SpotifyAlbumID: "spotify-album"},
	}
	_ = screen.View()

	provider.mu.Lock()
	calls := provider.calls
	provider.mu.Unlock()
	if calls != 2 {
		t.Fatalf("expected artwork provider to rerun after metadata changed, got %d calls", calls)
	}
}

func TestPlaybackArtworkOverlayShowsRecentAttemptsWhileLoading(t *testing.T) {
	block := make(chan struct{})
	provider := &artworkTestProvider{
		source: &components.ImageSource{Description: "art"},
		block:  block,
	}
	screen := newPlaybackScreen(Services{Artwork: provider}, AlbumArtOptions{})
	screen.SetSize(48, 24)
	screen.snapshot = PlaybackSnapshot{
		Track: &TrackInfo{
			ID:     "track-1",
			Title:  "Song",
			Artist: "Artist",
			Album:  "Album",
			Source: "youtube",
		},
	}

	_ = screen.View()
	deadline := time.Now().Add(250 * time.Millisecond)
	for {
		screen.consumeArtworkLookup()
		if len(screen.artworkAttempts) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expected artwork attempt to be recorded while loading")
		}
		time.Sleep(time.Millisecond)
	}

	view := screen.View()
	if !strings.Contains(view, "Artwork lookup") || !strings.Contains(view, "trying provider") {
		t.Fatalf("expected overlay log in artwork view, got %q", view)
	}
	close(block)
}

func TestPlaybackArtworkOverlayHidesAfterSuccessfulLoad(t *testing.T) {
	provider := &artworkTestProvider{
		source: &components.ImageSource{Description: "art"},
	}
	screen := newPlaybackScreen(Services{Artwork: provider}, AlbumArtOptions{})
	screen.SetSize(48, 24)
	screen.snapshot = PlaybackSnapshot{
		Track: &TrackInfo{
			ID:     "track-1",
			Title:  "Song",
			Artist: "Artist",
			Album:  "Album",
			Source: "youtube",
		},
	}

	_ = screen.View()
	deadline := time.Now().Add(250 * time.Millisecond)
	for {
		screen.consumeArtworkLookup()
		if !screen.artworkLoading {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expected artwork lookup to settle")
		}
		time.Sleep(time.Millisecond)
	}

	view := screen.View()
	if strings.Contains(view, "Artwork lookup") {
		t.Fatalf("expected artwork overlay to hide after successful load, got %q", view)
	}
}

func TestPlaybackLyricsLookupIsCachedAcrossViews(t *testing.T) {
	provider := &lyricsTestProvider{doc: &lyrics.Document{PlainLyrics: "line one\nline two"}}
	screen := newPlaybackScreen(Services{Lyrics: provider}, AlbumArtOptions{})
	screen.pane = PaneLyrics
	screen.SetSize(40, 20)
	screen.snapshot = PlaybackSnapshot{
		Track: &TrackInfo{ID: "track-1", Title: "Song", Artist: "Artist"},
	}

	_ = screen.View()
	_ = screen.View()

	provider.mu.Lock()
	calls := provider.calls
	provider.mu.Unlock()
	if calls != 1 {
		t.Fatalf("expected lyrics provider to be called once, got %d", calls)
	}
}

func TestPlaybackLyricsPaneScrollsWithinViewport(t *testing.T) {
	lines := make([]string, 18)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %02d", i+1)
	}
	screen := newPlaybackScreen(Services{}, AlbumArtOptions{})
	screen.pane = PaneLyrics
	screen.SetSize(40, 20)
	screen.lyricsDoc = &lyrics.Document{PlainLyrics: strings.Join(lines, "\n")}

	before := screen.lyricsViewportLines()
	if len(before) == 0 {
		t.Fatal("expected visible lyrics lines before scrolling")
	}
	if before[0] != "line 01" {
		t.Fatalf("expected first visible line to be line 01, got %q", before[0])
	}

	status, cmd := screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if cmd != nil {
		t.Fatalf("expected no async command when scrolling lyrics, got %v", cmd)
	}
	start, end := screen.lyricsWindow(len(lines))
	expectedStatus := fmt.Sprintf("Lyrics lines %d-%d of %d.", start+1, end, len(lines))
	if status != expectedStatus {
		t.Fatalf("unexpected scroll status: %q", status)
	}

	after := screen.lyricsViewportLines()
	if len(after) == 0 {
		t.Fatal("expected visible lyrics lines after scrolling")
	}
	if after[0] != "line 02" {
		t.Fatalf("expected first visible line to advance to line 02, got %q", after[0])
	}
}

func TestPlaybackLyricsScrollResetsForNewTrack(t *testing.T) {
	screen := newPlaybackScreen(Services{Lyrics: &lyricsTestProvider{}}, AlbumArtOptions{})
	screen.SetSize(40, 20)
	screen.lyricsTrackKey = "old-track"
	screen.lyricsScroll = 5
	screen.snapshot = PlaybackSnapshot{
		Track: &TrackInfo{ID: "track-1", Title: "Song", Artist: "Artist"},
	}

	screen.refreshLyrics(&TrackInfo{ID: "track-2", Title: "Other Song", Artist: "Artist"})

	if screen.lyricsScroll != 0 {
		t.Fatalf("expected lyrics scroll to reset for a new track, got %d", screen.lyricsScroll)
	}
}

func TestPlaybackVisualizationRefreshesAcrossViews(t *testing.T) {
	provider := &visualizationTestProvider{steps: []string{"bars 1", "bars 2"}}
	screen := newPlaybackScreen(Services{Visualization: provider}, AlbumArtOptions{})
	screen.pane = PaneVisualizer
	screen.SetSize(40, 20)

	_ = screen.View()
	first := screen.visualContent
	_ = screen.View()
	second := screen.visualContent

	provider.mu.Lock()
	calls := provider.calls
	provider.mu.Unlock()
	if calls != 2 {
		t.Fatalf("expected visualization provider to be called for each view, got %d", calls)
	}
	if first != "bars 1" {
		t.Fatalf("expected first visualization frame in first view, got %q", first)
	}
	if second != "bars 2" {
		t.Fatalf("expected updated visualization frame in second view, got %q", second)
	}
}

func TestPlaybackVisualizerWithoutProviderDoesNotShowPlaceholderText(t *testing.T) {
	screen := newPlaybackScreen(Services{}, AlbumArtOptions{})
	screen.pane = PaneVisualizer
	screen.SetSize(40, 20)

	view := screen.View()
	if strings.Contains(view, "placeholder") || strings.Contains(view, "Attach a visualization provider") {
		t.Fatalf("expected neutral visualizer background, got %q", view)
	}
}

func TestPlaybackArtworkWithoutProviderDoesNotShowPlaceholderText(t *testing.T) {
	screen := newPlaybackScreen(Services{}, AlbumArtOptions{})
	screen.pane = PaneArtwork
	screen.SetSize(40, 20)

	view := screen.View()
	if strings.Contains(view, "Artwork will appear here") {
		t.Fatalf("expected neutral artwork background, got %q", view)
	}
}
