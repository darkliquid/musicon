package lyrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLRCLibProviderExactMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get":
			if got := r.URL.Query().Get("track_name"); got != "Song" {
				t.Fatalf("unexpected track query: %q", got)
			}
			_, _ = w.Write([]byte(`{"trackName":"Song","artistName":"Artist","albumName":"Album","duration":180,"plainLyrics":"plain","syncedLyrics":"[00:01.00]hello"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := &LRCLibProvider{Client: server.Client(), BaseURL: server.URL}
	document, err := provider.Lookup(context.Background(), Request{
		Title:    "Song",
		Artist:   "Artist",
		Album:    "Album",
		Duration: 180 * time.Second,
	})
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if document.Provider != "lrclib" || len(document.TimedLines) != 1 || document.TimedLines[0].Text != "hello" {
		t.Fatalf("unexpected lrclib document: %#v", document)
	}
}

func TestLRCLibProviderSearchFiltersWeakMatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			_, _ = w.Write([]byte(`[
				{"trackName":"Wrong Song","artistName":"Artist","albumName":"Album","duration":180,"plainLyrics":"bad"},
				{"trackName":"Song","artistName":"Artist","albumName":"Album","duration":181,"plainLyrics":"good"}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := &LRCLibProvider{Client: server.Client(), BaseURL: server.URL}
	document, err := provider.Lookup(context.Background(), Request{
		Title:    "Song",
		Artist:   "Artist",
		Duration: 180 * time.Second,
	})
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if strings.TrimSpace(document.PlainLyrics) != "good" {
		t.Fatalf("expected strong match result, got %#v", document)
	}
}
