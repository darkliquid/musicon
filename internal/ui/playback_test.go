package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/darkliquid/musicon/pkg/components"
	"github.com/darkliquid/musicon/pkg/coverart"
)

type stubArtworkProvider struct {
	source *components.ImageSource
	err    error
}

func (s stubArtworkProvider) Artwork(metadata coverart.Metadata) (*components.ImageSource, error) {
	_ = metadata
	return s.source, s.err
}

func TestPlaybackScreenArtworkPaneUsesImageComponent(t *testing.T) {
	screen := newPlaybackScreen(Services{
		Artwork: stubArtworkProvider{
			source: &components.ImageSource{Data: []byte("abc"), Description: "cover"},
		},
	}, AlbumArtOptions{})
	screen.snapshot.Track = &TrackInfo{ID: "track-1", Title: "Song"}
	screen.artwork = components.NewTerminalImageWithRenderer(components.ImageRendererFunc(func(source components.ImageSource, width, height int) (string, error) {
		return "rendered artwork", nil
	}))

	got := screen.centerView(24, 12)
	if !strings.Contains(got, "rendered artwork") {
		t.Fatalf("expected rendered artwork in view, got %q", got)
	}
	if !strings.Contains(got, "·") {
		t.Fatalf("expected filler pattern around artwork, got %q", got)
	}
}

func TestPlaybackScreenArtworkPaneShowsRenderFailure(t *testing.T) {
	screen := newPlaybackScreen(Services{
		Artwork: stubArtworkProvider{
			source: &components.ImageSource{Data: []byte("abc"), Description: "cover"},
		},
	}, AlbumArtOptions{})
	screen.snapshot.Track = &TrackInfo{ID: "track-1", Title: "Song"}
	screen.artwork = components.NewTerminalImageWithRenderer(components.ImageRendererFunc(func(source components.ImageSource, width, height int) (string, error) {
		return "", errors.New("no protocol")
	}))

	got := screen.centerView(24, 12)
	if !strings.Contains(got, "Artwork render") || !strings.Contains(got, "no protocol") {
		t.Fatalf("expected render failure message, got %q", got)
	}
}

func TestPlaybackScreenViewUsesOverlaysInsteadOfStackedPanels(t *testing.T) {
	screen := newPlaybackScreen(Services{}, AlbumArtOptions{})
	screen.SetSize(48, 24)

	got := screen.View()
	if !strings.Contains(got, "Playback") || !strings.Contains(got, "state:") {
		t.Fatalf("expected playback controls overlay, got %q", got)
	}
	if strings.Contains(got, "Playback controls") {
		t.Fatalf("expected stacked controls panel title to be removed, got %q", got)
	}
}

func TestPlaybackScreenUsesConfiguredPauseBinding(t *testing.T) {
	screen := newPlaybackScreenWithKeyMap(Services{}, AlbumArtOptions{}, normalizedKeyMap(KeybindOptions{
		Playback: PlaybackKeybindOptions{
			TogglePause: []string{"p"},
		},
	}).Playback)

	if got := screen.Update(tea.KeyPressMsg(tea.Key{Text: "p"})); got != "Playback paused." {
		t.Fatalf("expected custom pause keybind to pause playback, got %q", got)
	}
	if got := screen.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace})); got != "" {
		t.Fatalf("expected default space binding to stop matching after override, got %q", got)
	}
}
