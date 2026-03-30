package local

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/musicon/internal/audio"
	teaui "github.com/darkliquid/musicon/internal/ui"
	"github.com/darkliquid/musicon/pkg/coverart"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/wav"
)

func TestLibrarySearchFindsLocalAudioFiles(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "My Song.wav")
	writeSilentWAV(t, audioPath)

	library := NewLibrary(dir)
	results, err := library.Search(teaui.SearchRequest{
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

	library := NewLibrary(dir)
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

	library := NewLibrary(dir)
	library.RefreshInterval = 0

	results, err := library.Search(teaui.SearchRequest{
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

	results, err = library.Search(teaui.SearchRequest{
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
