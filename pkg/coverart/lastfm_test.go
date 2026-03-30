package coverart

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLastFMProviderUsesMusicBrainzReleaseID(t *testing.T) {
	var apiURL string
	var requestedMBID string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			requestedMBID = r.URL.Query().Get("mbid")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"album":{"image":[{"#text":"` + apiURL + `/cover","size":"mega"}]}}`))
		case "/cover":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg"))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer apiServer.Close()
	apiURL = apiServer.URL

	provider := &LastFMProvider{
		Client:  apiServer.Client(),
		APIKey:  "key",
		BaseURL: apiServer.URL,
	}

	result, err := provider.Lookup(context.Background(), Metadata{
		IDs: IDs{MusicBrainzReleaseID: "release-id"},
	})
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if requestedMBID != "release-id" {
		t.Fatalf("expected release mbid, got %q", requestedMBID)
	}
	if got := string(result.Image.Data); got != "jpeg" {
		t.Fatalf("unexpected image data %q", got)
	}
}

func TestLastFMProviderFallsBackToArtistAndAlbum(t *testing.T) {
	var apiURL string
	var requestedArtist string
	var requestedAlbum string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			requestedArtist = r.URL.Query().Get("artist")
			requestedAlbum = r.URL.Query().Get("album")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"album":{"image":[{"#text":"` + apiURL + `/cover","size":"large"}]}}`))
		case "/cover":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png"))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer apiServer.Close()
	apiURL = apiServer.URL

	provider := &LastFMProvider{
		Client:  apiServer.Client(),
		APIKey:  "key",
		BaseURL: apiServer.URL,
	}

	result, err := provider.Lookup(context.Background(), Metadata{
		Artist: "Artist",
		Album:  "Album",
	})
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if requestedArtist != "Artist" || requestedAlbum != "Album" {
		t.Fatalf("expected artist/album lookup, got artist=%q album=%q", requestedArtist, requestedAlbum)
	}
	if got := string(result.Image.Data); got != "png" {
		t.Fatalf("unexpected image data %q", got)
	}
}

func TestLastFMProviderWithoutAPIKeyFallsThrough(t *testing.T) {
	provider := &LastFMProvider{}
	_, err := provider.Lookup(context.Background(), Metadata{
		Artist: "Artist",
		Album:  "Album",
	})
	if !IsNotFound(err) {
		t.Fatalf("expected not found without api key, got %v", err)
	}
}
