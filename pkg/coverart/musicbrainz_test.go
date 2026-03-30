package coverart

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMusicBrainzProviderUsesReleaseIDFirst(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("img"))
	}))
	defer server.Close()

	provider := NewMusicBrainzProvider("musicon-test")
	provider.CoverArtBaseURL = server.URL

	result, err := provider.Lookup(context.Background(), Metadata{
		IDs: IDs{MusicBrainzReleaseID: "release-id"},
	})
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if requestedPath != "/release/release-id/front-500" {
		t.Fatalf("unexpected cover art path %q", requestedPath)
	}
	if got := string(result.Image.Data); got != "img" {
		t.Fatalf("unexpected image data %q", got)
	}
}

func TestMusicBrainzProviderSearchesThenFetchesCover(t *testing.T) {
	var sawUserAgent string
	mbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUserAgent = r.Header.Get("User-Agent")
		if r.URL.Path != "/release" {
			t.Fatalf("unexpected musicbrainz path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"releases":[{"id":"release-id","release-group":{"id":"group-id"}}]}`))
	}))
	defer mbServer.Close()

	coverServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/release/release-id/front-500" {
			t.Fatalf("unexpected cover art path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("cover"))
	}))
	defer coverServer.Close()

	provider := NewMusicBrainzProvider("musicon-test/1.0")
	provider.BaseURL = mbServer.URL
	provider.CoverArtBaseURL = coverServer.URL
	provider.RateLimit = 0

	result, err := provider.Lookup(context.Background(), Metadata{
		Album:  "Album",
		Artist: "Artist",
		Title:  "Song",
	})
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if !strings.Contains(sawUserAgent, "musicon-test/1.0") {
		t.Fatalf("expected user agent to be forwarded, got %q", sawUserAgent)
	}
	if got := string(result.Image.Data); got != "cover" {
		t.Fatalf("unexpected image data %q", got)
	}
}

func TestMusicBrainzProviderLooksUpRecordingWhenReleaseIDsMissing(t *testing.T) {
	var sawLookupPath string
	mbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/recording/recording-id":
			sawLookupPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"releases":[{"id":"release-id","release-group":{"id":"group-id"}}]}`))
		default:
			t.Fatalf("unexpected musicbrainz path %q", r.URL.Path)
		}
	}))
	defer mbServer.Close()

	coverServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/release/release-id/front-500" {
			t.Fatalf("unexpected cover art path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("cover"))
	}))
	defer coverServer.Close()

	provider := NewMusicBrainzProvider("musicon-test/1.0")
	provider.BaseURL = mbServer.URL
	provider.CoverArtBaseURL = coverServer.URL
	provider.RateLimit = 0

	result, err := provider.Lookup(context.Background(), Metadata{
		IDs: IDs{MusicBrainzRecordingID: "recording-id"},
	})
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if sawLookupPath != "/recording/recording-id" {
		t.Fatalf("expected recording lookup, got %q", sawLookupPath)
	}
	if got := string(result.Image.Data); got != "cover" {
		t.Fatalf("unexpected image data %q", got)
	}
}

func TestMusicBrainzProviderReturnsNotFoundForEmptySearch(t *testing.T) {
	mbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"releases":[]}`))
	}))
	defer mbServer.Close()

	provider := NewMusicBrainzProvider("musicon-test")
	provider.BaseURL = mbServer.URL
	provider.RateLimit = 0

	_, err := provider.Lookup(context.Background(), Metadata{
		Album:  "Album",
		Artist: "Artist",
	})
	if !IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}
