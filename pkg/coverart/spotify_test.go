package coverart

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSpotifyProviderUsesAlbumIDAndDownloadsImage(t *testing.T) {
	var authHeader string
	var apiURL string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/albums/album-id":
			authHeader = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"Album","images":[{"url":"` + apiURL + `/image"}]}`))
		case "/image":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg"))
		default:
			t.Fatalf("unexpected api path %q", r.URL.Path)
		}
	}))
	defer apiServer.Close()
	apiURL = apiServer.URL

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			t.Fatalf("unexpected token path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Basic "+base64.StdEncoding.EncodeToString([]byte("id:secret")) {
			t.Fatalf("unexpected basic auth %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"token","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	provider := &SpotifyProvider{
		Client:       apiServer.Client(),
		ClientID:     "id",
		ClientSecret: "secret",
		BaseURL:      apiServer.URL,
		TokenURL:     tokenServer.URL,
	}

	result, err := provider.Lookup(context.Background(), Metadata{
		IDs: IDs{SpotifyAlbumID: "album-id"},
	})
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if authHeader != "Bearer token" {
		t.Fatalf("expected bearer token, got %q", authHeader)
	}
	if got := string(result.Image.Data); got != "jpeg" {
		t.Fatalf("unexpected image data %q", got)
	}
}

func TestSpotifyProviderSearchesWhenIDMissing(t *testing.T) {
	var apiURL string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"albums":{"items":[{"id":"album-id"}]}}`))
		case "/albums/album-id":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"Album","images":[{"url":"` + apiURL + `/image"}]}`))
		case "/image":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png"))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer apiServer.Close()
	apiURL = apiServer.URL

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"token","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	provider := &SpotifyProvider{
		Client:       apiServer.Client(),
		ClientID:     "id",
		ClientSecret: "secret",
		BaseURL:      apiServer.URL,
		TokenURL:     tokenServer.URL,
		Market:       "US",
	}

	result, err := provider.Lookup(context.Background(), Metadata{
		Album:  "Album",
		Artist: "Artist",
		Title:  "Song",
	})
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if got := string(result.Image.Data); got != "png" {
		t.Fatalf("unexpected image data %q", got)
	}
}

func TestSpotifyProviderLooksUpAlbumFromTrackID(t *testing.T) {
	var apiURL string
	var requestedTrackPath string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tracks/track-id":
			requestedTrackPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"album":{"id":"album-id"}}`))
		case "/albums/album-id":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"Album","images":[{"url":"` + apiURL + `/image"}]}`))
		case "/image":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png"))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer apiServer.Close()
	apiURL = apiServer.URL

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"token","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	provider := &SpotifyProvider{
		Client:       apiServer.Client(),
		ClientID:     "id",
		ClientSecret: "secret",
		BaseURL:      apiServer.URL,
		TokenURL:     tokenServer.URL,
		Market:       "US",
	}

	result, err := provider.Lookup(context.Background(), Metadata{
		IDs: IDs{SpotifyTrackID: "track-id"},
	})
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if requestedTrackPath != "/tracks/track-id" {
		t.Fatalf("expected track lookup, got %q", requestedTrackPath)
	}
	if got := string(result.Image.Data); got != "png" {
		t.Fatalf("unexpected image data %q", got)
	}
}

func TestSpotifyProviderWithoutCredentialsFallsThrough(t *testing.T) {
	provider := &SpotifyProvider{}
	_, err := provider.Lookup(context.Background(), Metadata{
		IDs: IDs{SpotifyAlbumID: "album-id"},
	})
	if !IsNotFound(err) {
		t.Fatalf("expected not found without credentials, got %v", err)
	}
}
