package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/darkliquid/musicon/pkg/components"
)

type stubArtworkProvider struct {
	source *components.ImageSource
	err    error
}

func (s stubArtworkProvider) Artwork(trackID string) (*components.ImageSource, error) {
	_ = trackID
	return s.source, s.err
}

func TestPlaybackScreenArtworkPaneUsesImageComponent(t *testing.T) {
	screen := newPlaybackScreen(Services{
		Artwork: stubArtworkProvider{
			source: &components.ImageSource{Data: []byte("abc"), Description: "cover"},
		},
	})
	screen.snapshot.Track = &TrackInfo{ID: "track-1", Title: "Song"}
	screen.artwork = components.NewTerminalImageWithRenderer(components.ImageRendererFunc(func(source components.ImageSource, width, height int) (string, error) {
		return "rendered artwork", nil
	}))

	got := screen.centerView(24, 12)
	if !strings.Contains(got, "rendered artwork") {
		t.Fatalf("expected rendered artwork in view, got %q", got)
	}
}

func TestPlaybackScreenArtworkPaneShowsRenderFailure(t *testing.T) {
	screen := newPlaybackScreen(Services{
		Artwork: stubArtworkProvider{
			source: &components.ImageSource{Data: []byte("abc"), Description: "cover"},
		},
	})
	screen.snapshot.Track = &TrackInfo{ID: "track-1", Title: "Song"}
	screen.artwork = components.NewTerminalImageWithRenderer(components.ImageRendererFunc(func(source components.ImageSource, width, height int) (string, error) {
		return "", errors.New("no protocol")
	}))

	got := screen.centerView(24, 12)
	if !strings.Contains(got, "Artwork render") || !strings.Contains(got, "no protocol") {
		t.Fatalf("expected render failure message, got %q", got)
	}
}
