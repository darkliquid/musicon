package ui

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/darkliquid/musicon/pkg/components"
	"github.com/darkliquid/musicon/pkg/coverart"
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
}

func (p *artworkTestProvider) Artwork(_ coverart.Metadata) (*components.ImageSource, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.source, nil
}

type lyricsTestProvider struct {
	mu    sync.Mutex
	calls int
	lines []string
}

func (p *lyricsTestProvider) Lyrics(string) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return append([]string(nil), p.lines...), nil
}

type visualizationTestProvider struct {
	mu      sync.Mutex
	calls   int
	content string
}

func (p *visualizationTestProvider) Placeholder(PlaybackPane, int, int) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
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

func TestPlaybackLyricsLookupIsCachedAcrossViews(t *testing.T) {
	provider := &lyricsTestProvider{lines: []string{"line one", "line two"}}
	screen := newPlaybackScreen(Services{Lyrics: provider}, AlbumArtOptions{})
	screen.pane = PaneLyrics
	screen.SetSize(40, 20)
	screen.snapshot = PlaybackSnapshot{
		Track: &TrackInfo{ID: "track-1"},
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

func TestPlaybackVisualizationLookupIsCachedAcrossViews(t *testing.T) {
	provider := &visualizationTestProvider{content: "bars"}
	screen := newPlaybackScreen(Services{Visualization: provider}, AlbumArtOptions{})
	screen.pane = PaneVisualizer
	screen.SetSize(40, 20)

	_ = screen.View()
	_ = screen.View()

	provider.mu.Lock()
	calls := provider.calls
	provider.mu.Unlock()
	if calls != 1 {
		t.Fatalf("expected visualization provider to be called once, got %d", calls)
	}
}
