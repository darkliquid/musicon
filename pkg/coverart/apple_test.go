package coverart

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAppleMusicProviderUsesAlbumIDAndDownloadsImage(t *testing.T) {
	var authHeader string
	var apiURL string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog/us/albums/album-id":
			authHeader = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"attributes":{"artwork":{"url":"` + apiURL + `/image/{w}x{h}.jpg","width":1200,"height":1200}}}]}`))
		case "/image/1000x1000.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg"))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer apiServer.Close()
	apiURL = apiServer.URL

	provider := &AppleMusicProvider{
		Client:         apiServer.Client(),
		DeveloperToken: "token",
		Storefront:     "us",
		BaseURL:        apiServer.URL,
	}

	result, err := provider.Lookup(context.Background(), Metadata{
		IDs: IDs{AppleMusicAlbumID: "album-id"},
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

func TestAppleMusicProviderSearchesWhenIDMissing(t *testing.T) {
	var apiURL string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog/us/search":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":{"albums":{"data":[{"id":"album-id"}]}}}`))
		case "/catalog/us/albums/album-id":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"attributes":{"artwork":{"url":"` + apiURL + `/image/{w}x{h}.jpg","width":800,"height":800}}}]}`))
		case "/image/800x800.jpg":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png"))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer apiServer.Close()
	apiURL = apiServer.URL

	provider := &AppleMusicProvider{
		Client:         apiServer.Client(),
		DeveloperToken: "token",
		Storefront:     "us",
		BaseURL:        apiServer.URL,
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

func TestAppleMusicProviderLooksUpAlbumFromSongID(t *testing.T) {
	var apiURL string
	var requestedSongPath string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog/us/songs/song-id":
			requestedSongPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"relationships":{"albums":{"data":[{"id":"album-id"}]}}}]}`))
		case "/catalog/us/albums/album-id":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"attributes":{"artwork":{"url":"` + apiURL + `/image/{w}x{h}.jpg","width":600,"height":600}}}]}`))
		case "/image/600x600.jpg":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png"))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer apiServer.Close()
	apiURL = apiServer.URL

	provider := &AppleMusicProvider{
		Client:         apiServer.Client(),
		DeveloperToken: "token",
		Storefront:     "us",
		BaseURL:        apiServer.URL,
	}

	result, err := provider.Lookup(context.Background(), Metadata{
		IDs: IDs{AppleMusicSongID: "song-id"},
	})
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if requestedSongPath != "/catalog/us/songs/song-id" {
		t.Fatalf("expected song lookup, got %q", requestedSongPath)
	}
	if got := string(result.Image.Data); got != "png" {
		t.Fatalf("unexpected image data %q", got)
	}
}

func TestAppleMusicProviderWithoutTokenFallsThrough(t *testing.T) {
	provider := &AppleMusicProvider{Storefront: "us"}
	_, err := provider.Lookup(context.Background(), Metadata{
		IDs: IDs{AppleMusicAlbumID: "album-id"},
	})
	if !IsNotFound(err) {
		t.Fatalf("expected not found without token, got %v", err)
	}
}
