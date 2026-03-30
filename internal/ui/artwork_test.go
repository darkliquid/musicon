package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/darkliquid/musicon/pkg/coverart"
)

type stubCoverArtResolver struct {
	result coverart.Result
	err    error
}

func (s stubCoverArtResolver) Resolve(ctx context.Context, metadata coverart.Metadata) (coverart.Result, error) {
	_ = ctx
	_ = metadata
	return s.result, s.err
}

func TestCoverArtProviderMapsResult(t *testing.T) {
	provider := NewCoverArtProvider(stubCoverArtResolver{
		result: coverart.Result{
			Image: coverart.Image{
				Data:        []byte("img"),
				Description: "desc",
			},
		},
	})
	source, err := provider.Artwork(coverart.Metadata{Title: "Song"})
	if err != nil {
		t.Fatalf("artwork failed: %v", err)
	}
	if source == nil || string(source.Data) != "img" || source.Description != "desc" {
		t.Fatalf("unexpected source %#v", source)
	}
}

func TestCoverArtProviderTreatsNotFoundAsNil(t *testing.T) {
	provider := NewCoverArtProvider(stubCoverArtResolver{err: coverart.ErrNotFound})
	source, err := provider.Artwork(coverart.Metadata{Title: "Song"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if source != nil {
		t.Fatalf("expected nil source on not found, got %#v", source)
	}
}

func TestCoverArtProviderSurfacesHardError(t *testing.T) {
	want := errors.New("boom")
	provider := NewCoverArtProvider(stubCoverArtResolver{err: want})
	_, err := provider.Artwork(coverart.Metadata{Title: "Song"})
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}
