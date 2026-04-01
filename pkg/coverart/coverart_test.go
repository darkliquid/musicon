package coverart

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
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
		Title:     " Song ",
		Album:     " Album ",
		Artist:    " Artist ",
		RemoteURL: " https://img.example.test/cover.jpg ",
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
	if got.Title != "Song" || got.Album != "Album" || got.Artist != "Artist" || got.RemoteURL != "https://img.example.test/cover.jpg" {
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
	second := &stubProvider{name: "second", result: Result{Image: Image{Data: mustPNG(t, 4, 4)}}}
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
	second := &stubProvider{name: "second", result: Result{Image: Image{Data: mustPNG(t, 4, 4)}}}
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

func TestChainResolveObservedReportsAttempts(t *testing.T) {
	first := &stubProvider{name: "first", err: ErrNotFound}
	second := &stubProvider{name: "second", result: Result{Image: Image{Data: mustPNG(t, 4, 4)}}}
	chain := NewChain(first, second)
	var events []AttemptEvent

	result, err := chain.ResolveObserved(context.Background(), Metadata{Title: "Song"}, func(event AttemptEvent) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatalf("resolve observed failed: %v", err)
	}
	if result.Provider != "second" {
		t.Fatalf("expected provider second, got %q", result.Provider)
	}
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %#v", events)
	}
	if events[0].Provider != "first" || events[0].Status != AttemptTrying {
		t.Fatalf("unexpected first event: %#v", events[0])
	}
	if events[1].Provider != "first" || events[1].Status != AttemptNotFound {
		t.Fatalf("unexpected second event: %#v", events[1])
	}
	if events[2].Provider != "second" || events[2].Status != AttemptTrying {
		t.Fatalf("unexpected third event: %#v", events[2])
	}
	if events[3].Provider != "second" || events[3].Status != AttemptSuccess {
		t.Fatalf("unexpected fourth event: %#v", events[3])
	}
}

func TestMetadataMergeFillsArtworkGaps(t *testing.T) {
	embedded := &Image{Data: mustPNG(t, 4, 4), MIMEType: "image/png"}
	base := Metadata{
		Artist:    "Artist",
		RemoteURL: "https://img.example.test/base.jpg",
		IDs: IDs{
			SpotifyTrackID: "track-id",
		},
		Local: &LocalMetadata{
			AudioPath: "/music/song.mp3",
		},
	}
	fallback := Metadata{
		Title:     "Song",
		Album:     "Album",
		RemoteURL: "https://img.example.test/fallback.jpg",
		IDs: IDs{
			MusicBrainzReleaseID: "mb-release",
			SpotifyAlbumID:       "album-id",
		},
		Local: &LocalMetadata{
			CoverFilePath: "/music/cover.jpg",
			Embedded:      embedded,
		},
	}

	got := base.Merge(fallback)
	if got.Title != "Song" || got.Album != "Album" || got.Artist != "Artist" || got.RemoteURL != "https://img.example.test/base.jpg" {
		t.Fatalf("unexpected merged labels: %#v", got)
	}
	if got.IDs.SpotifyTrackID != "track-id" || got.IDs.SpotifyAlbumID != "album-id" || got.IDs.MusicBrainzReleaseID != "mb-release" {
		t.Fatalf("unexpected merged ids: %#v", got.IDs)
	}
	if got.Local == nil || got.Local.AudioPath != "/music/song.mp3" || got.Local.CoverFilePath != "/music/cover.jpg" || got.Local.Embedded != embedded {
		t.Fatalf("unexpected merged local metadata: %#v", got.Local)
	}
}

func TestMetadataURLProviderFetchesRemoteArtwork(t *testing.T) {
	pngData := mustPNG(t, 6, 6)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cover.jpg" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData)
	}))
	defer server.Close()

	provider := MetadataURLProvider{Client: server.Client()}
	result, err := provider.Lookup(context.Background(), Metadata{RemoteURL: server.URL + "/cover.jpg"})
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if result.Provider != "metadata-url" || result.Image.MIMEType != "image/png" || !bytes.Equal(result.Image.Data, pngData) {
		t.Fatalf("unexpected metadata-url result: %#v", result)
	}
}

func TestChainResolveFallsThroughUnsupportedArtwork(t *testing.T) {
	first := &stubProvider{name: "first", result: Result{Image: Image{
		Data:     []byte("not-an-image"),
		MIMEType: "application/octet-stream",
	}}}
	second := &stubProvider{name: "second", result: Result{Image: Image{Data: mustPNG(t, 8, 8)}}}
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

func TestChainResolveRasterizesSVGArtwork(t *testing.T) {
	first := &stubProvider{name: "first", result: Result{Image: Image{
		Data:        []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 16"><rect width="32" height="16" fill="#ff0000"/></svg>`),
		MIMEType:    "image/svg+xml; charset=utf-8",
		Description: "svg art",
	}}}
	chain := NewChain(first)

	result, err := chain.Resolve(context.Background(), Metadata{Title: "Song"})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if result.Provider != "first" {
		t.Fatalf("expected provider first, got %q", result.Provider)
	}
	if result.Image.MIMEType != "image/png" {
		t.Fatalf("expected rasterized png mime type, got %q", result.Image.MIMEType)
	}
	decoded, _, err := image.Decode(bytes.NewReader(result.Image.Data))
	if err != nil {
		t.Fatalf("expected rasterized png to decode, got %v", err)
	}
	if decoded.Bounds().Dx() != 32 || decoded.Bounds().Dy() != 16 {
		t.Fatalf("unexpected rasterized bounds: %v", decoded.Bounds())
	}
}

func mustPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.NRGBA{R: 0x33, G: 0x66, B: 0x99, A: 0xff})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return out.Bytes()
}
