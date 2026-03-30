package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/musicon/internal/audio"
	teaui "github.com/darkliquid/musicon/internal/ui"
	"github.com/darkliquid/musicon/pkg/coverart"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/wav"
)

var localTinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
	0x00, 0x03, 0x01, 0x01, 0x00, 0xc9, 0xfe, 0x92,
	0xef, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
	0x44, 0xae, 0x42, 0x60, 0x82,
}

func TestLibrarySearchFindsLocalAudioFiles(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "My Song.wav")
	writeSilentWAV(t, audioPath)

	library := NewLibrary(Options{Roots: []string{dir}})
	results, err := library.Search(context.Background(), teaui.SearchRequest{
		SourceID: sourceID,
		Query:    "song",
		Filters:  teaui.DefaultSearchFilters(),
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].ID != audioPath || results[0].Title != "My Song" {
		t.Fatalf("unexpected result: %#v", results[0])
	}
	if results[0].Artwork.Local == nil || results[0].Artwork.Local.AudioPath != audioPath {
		t.Fatalf("expected local audio path metadata, got %#v", results[0].Artwork.Local)
	}
}

func TestLibraryResolveDecodesWAVFiles(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "Track.wav")
	writeSilentWAV(t, audioPath)

	library := NewLibrary(Options{Roots: []string{dir}})
	resolved, err := library.Resolve(teaui.QueueEntry{
		ID:     audioPath,
		Title:  "Track",
		Source: "Local files",
		Artwork: coverart.Metadata{
			Local: &coverart.LocalMetadata{AudioPath: audioPath},
		},
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	defer resolved.Stream.Close()

	if resolved.Info.ID != audioPath || resolved.Info.Title != "Track" {
		t.Fatalf("unexpected resolved track info: %#v", resolved.Info)
	}
	if resolved.Info.Artwork.Local == nil || resolved.Info.Artwork.Local.AudioPath != audioPath {
		t.Fatalf("expected local artwork metadata preserved, got %#v", resolved.Info.Artwork.Local)
	}
	if resolved.Format.SampleRate <= 0 {
		t.Fatalf("expected valid format, got %#v", resolved.Format)
	}
}

func TestLibraryRefreshesSearchResultsWhenFilesChange(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "First.wav")
	writeSilentWAV(t, firstPath)

	library := NewLibrary(Options{Roots: []string{dir}})
	library.RefreshInterval = 0

	results, err := library.Search(context.Background(), teaui.SearchRequest{
		SourceID: sourceID,
		Query:    "first",
		Filters:  teaui.DefaultSearchFilters(),
	})
	if err != nil {
		t.Fatalf("initial search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected initial result, got %d", len(results))
	}

	secondPath := filepath.Join(dir, "Second.wav")
	writeSilentWAV(t, secondPath)

	results, err = library.Search(context.Background(), teaui.SearchRequest{
		SourceID: sourceID,
		Query:    "second",
		Filters:  teaui.DefaultSearchFilters(),
	})
	if err != nil {
		t.Fatalf("refreshed search failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != secondPath {
		t.Fatalf("expected refreshed result for second file, got %#v", results)
	}
}

func TestLibrarySearchFindsNestedFilesByPathFragment(t *testing.T) {
	dir := t.TempDir()
	nestedDir := filepath.Join(dir, "Artist", "Album")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	audioPath := filepath.Join(nestedDir, "Track.wav")
	writeSilentWAV(t, audioPath)

	library := NewLibrary(Options{Roots: []string{dir}})
	results, err := library.Search(context.Background(), teaui.SearchRequest{
		SourceID: sourceID,
		Query:    "artist/album/track.wav",
		Filters:  teaui.DefaultSearchFilters(),
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != audioPath {
		t.Fatalf("expected nested path result, got %#v", results)
	}
}

func TestLibrarySearchArtworkMetadataResolvesSiblingCover(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "Track.wav")
	coverPath := filepath.Join(dir, "Cover.JPG")
	writeSilentWAV(t, audioPath)
	if err := os.WriteFile(coverPath, localTinyPNG, 0o644); err != nil {
		t.Fatalf("write cover failed: %v", err)
	}

	library := NewLibrary(Options{Roots: []string{dir}})
	results, err := library.Search(context.Background(), teaui.SearchRequest{
		SourceID: sourceID,
		Query:    "track",
		Filters:  teaui.DefaultSearchFilters(),
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}

	provider := coverart.NewLocalFilesProvider()
	image, err := provider.Lookup(context.Background(), results[0].Artwork)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if image.Image.Description != "Cover.JPG" {
		t.Fatalf("expected sibling cover to resolve from search metadata, got %#v", image.Image)
	}
}

func TestLibrarySearchSpansMultipleConfiguredRoots(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	audioPath := filepath.Join(second, "Elsewhere.wav")
	writeSilentWAV(t, audioPath)

	library := NewLibrary(Options{Roots: []string{first, second}})
	results, err := library.Search(context.Background(), teaui.SearchRequest{
		SourceID: sourceID,
		Query:    "elsewhere",
		Filters:  teaui.DefaultSearchFilters(),
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != audioPath {
		t.Fatalf("expected multi-root match, got %#v", results)
	}
}

func TestIDsFromRawTagsExtractsKnownExternalIDs(t *testing.T) {
	ids := idsFromRawTags(map[string]interface{}{
		"MUSICBRAINZ_ALBUMID":          "mb-release",
		"MusicBrainz Release Group Id": "mb-group",
		"musicbrainz_trackid":          "mb-recording",
		"spotify_track_id":             "spotify-track",
		"spotify album id":             "spotify-album",
		"itunesalbumid":                "apple-album",
		"itunestrackid":                "apple-song",
	})
	if ids.MusicBrainzReleaseID != "mb-release" || ids.MusicBrainzReleaseGroupID != "mb-group" || ids.MusicBrainzRecordingID != "mb-recording" {
		t.Fatalf("unexpected musicbrainz ids: %#v", ids)
	}
	if ids.SpotifyAlbumID != "spotify-album" || ids.SpotifyTrackID != "spotify-track" {
		t.Fatalf("unexpected spotify ids: %#v", ids)
	}
	if ids.AppleMusicAlbumID != "apple-album" || ids.AppleMusicSongID != "apple-song" {
		t.Fatalf("unexpected apple ids: %#v", ids)
	}
}

func writeSilentWAV(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create wav failed: %v", err)
	}
	defer file.Close()

	format := beep.Format{SampleRate: 48_000, NumChannels: 2, Precision: 2}
	streamer := beep.Take(format.SampleRate.N(100000000), beep.Silence(-1))
	if err := wav.Encode(file, streamer, format); err != nil {
		t.Fatalf("encode wav failed: %v", err)
	}
}

var _ audio.Resolver = (*Library)(nil)
