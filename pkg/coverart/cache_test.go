package coverart

import (
	"context"
	"errors"
	"testing"
)

func TestCachedProviderUsesCacheAfterFirstLookup(t *testing.T) {
	cache := NewDiskCache(t.TempDir())
	provider := &stubProvider{
		name:   "provider",
		result: Result{Image: Image{Data: []byte("img"), MIMEType: "image/png"}},
	}

	wrapped := NewCachedProvider(provider, cache)
	if _, err := wrapped.Lookup(context.Background(), Metadata{Title: "Song"}); err != nil {
		t.Fatalf("first lookup failed: %v", err)
	}
	if _, err := wrapped.Lookup(context.Background(), Metadata{Title: "Song"}); err != nil {
		t.Fatalf("second lookup failed: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected underlying provider called once, got %d", provider.calls)
	}
}

func TestCachedProviderDoesNotCacheNotFound(t *testing.T) {
	cache := NewDiskCache(t.TempDir())
	provider := &stubProvider{name: "provider", err: ErrNotFound}

	wrapped := NewCachedProvider(provider, cache)
	_, err := wrapped.Lookup(context.Background(), Metadata{Title: "Song"})
	if !IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
	_, err = wrapped.Lookup(context.Background(), Metadata{Title: "Song"})
	if !IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
	if provider.calls != 2 {
		t.Fatalf("expected provider called twice for not-found misses, got %d", provider.calls)
	}
}

func TestCachedProviderSurfacesCacheWriteFailures(t *testing.T) {
	provider := &stubProvider{
		name:   "provider",
		result: Result{Image: Image{Data: []byte("img")}},
	}
	wrapped := NewCachedProvider(provider, failingCache{})
	_, err := wrapped.Lookup(context.Background(), Metadata{Title: "Song"})
	if !errors.Is(err, errCacheBoom) {
		t.Fatalf("expected cache failure, got %v", err)
	}
}

func TestCachedProviderReusesRemoteCacheAcrossLocalMetadataChanges(t *testing.T) {
	cache := NewDiskCache(t.TempDir())
	provider := &stubProvider{
		name:   "provider",
		result: Result{Image: Image{Data: []byte("img"), MIMEType: "image/png"}},
	}

	wrapped := NewCachedProvider(provider, cache)
	first := Metadata{
		Title:  "Song",
		Album:  "Album",
		Artist: "Artist",
		Local: &LocalMetadata{
			AudioPath: "/music/song.mp3",
			Embedded:  &Image{Data: []byte("embedded"), Description: "embedded"},
		},
	}
	second := Metadata{
		Title:  "Song",
		Album:  "Album",
		Artist: "Artist",
	}
	if _, err := wrapped.Lookup(context.Background(), first); err != nil {
		t.Fatalf("first lookup failed: %v", err)
	}
	if _, err := wrapped.Lookup(context.Background(), second); err != nil {
		t.Fatalf("second lookup failed: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected cache reuse across metadata shape changes, got %d provider calls", provider.calls)
	}
}

var errCacheBoom = errors.New("cache boom")

type failingCache struct{}

func (failingCache) Get(string) (Image, error) { return Image{}, ErrNotFound }
func (failingCache) Put(string, Image) error   { return errCacheBoom }
