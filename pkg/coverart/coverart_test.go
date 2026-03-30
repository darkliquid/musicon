package coverart

import (
	"context"
	"errors"
	"testing"
)

type stubProvider struct {
	name   string
	result Result
	err    error
	calls  int
}

func (s *stubProvider) Name() string { return s.name }

func (s *stubProvider) Lookup(ctx context.Context, metadata Metadata) (Result, error) {
	_ = ctx
	_ = metadata
	s.calls++
	return s.result, s.err
}

func TestMetadataNormalize(t *testing.T) {
	meta := Metadata{
		Title:  " Song ",
		Album:  " Album ",
		Artist: " Artist ",
		IDs: IDs{
			MusicBrainzReleaseID:      " mbid ",
			MusicBrainzReleaseGroupID: " rgid ",
			SpotifyAlbumID:            " spotify ",
			AppleMusicAlbumID:         " apple ",
		},
		Local: &LocalMetadata{
			AudioPath:     " /tmp/song.mp3 ",
			CoverFilePath: " cover.jpg ",
		},
	}

	got := meta.Normalize()
	if got.Title != "Song" || got.Album != "Album" || got.Artist != "Artist" {
		t.Fatalf("unexpected normalized fields: %#v", got)
	}
	if got.IDs.MusicBrainzReleaseID != "mbid" || got.IDs.SpotifyAlbumID != "spotify" || got.IDs.AppleMusicAlbumID != "apple" {
		t.Fatalf("unexpected normalized ids: %#v", got.IDs)
	}
	if got.Local == nil || got.Local.AudioPath != "/tmp/song.mp3" || got.Local.CoverFilePath != "cover.jpg" {
		t.Fatalf("unexpected normalized local metadata: %#v", got.Local)
	}
}

func TestChainResolveFallsThroughNotFound(t *testing.T) {
	first := &stubProvider{name: "first", err: ErrNotFound}
	second := &stubProvider{name: "second", result: Result{Image: Image{Data: []byte("img")}}}
	chain := NewChain(first, second)

	result, err := chain.Resolve(context.Background(), Metadata{Title: "Song"})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if result.Provider != "second" {
		t.Fatalf("expected provider second, got %q", result.Provider)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected both providers called once, got %d and %d", first.calls, second.calls)
	}
}

func TestChainResolveStopsOnHardFailure(t *testing.T) {
	want := errors.New("boom")
	first := &stubProvider{name: "first", err: want}
	second := &stubProvider{name: "second", result: Result{Image: Image{Data: []byte("img")}}}
	chain := NewChain(first, second)

	_, err := chain.Resolve(context.Background(), Metadata{Title: "Song"})
	if !errors.Is(err, want) {
		t.Fatalf("expected hard failure %v, got %v", want, err)
	}
	if first.calls != 1 || second.calls != 0 {
		t.Fatalf("expected chain to stop after hard failure, got %d and %d", first.calls, second.calls)
	}
}

func TestChainResolveRequiresUsefulMetadata(t *testing.T) {
	chain := NewChain(&stubProvider{name: "first"})
	_, err := chain.Resolve(context.Background(), Metadata{})
	if !IsNotFound(err) {
		t.Fatalf("expected not found for empty metadata, got %v", err)
	}
}
