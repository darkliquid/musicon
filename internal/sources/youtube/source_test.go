package youtube

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	teaui "github.com/darkliquid/musicon/internal/ui"
	"github.com/darkliquid/musicon/pkg/coverart"
	"github.com/gopxl/beep"
	"github.com/lrstanley/go-ytdlp"
)

type stubStream struct {
	length   int
	position int
}

func (s *stubStream) Stream(samples [][2]float64) (int, bool) { return 0, false }
func (s *stubStream) Err() error                              { return nil }
func (s *stubStream) Len() int                                { return s.length }
func (s *stubStream) Position() int                           { return s.position }
func (s *stubStream) Seek(p int) error {
	s.position = p
	return nil
}
func (s *stubStream) Close() error { return nil }

func TestSearchQueryMapsJSONResults(t *testing.T) {
	source := NewSource(Options{Enabled: true, MaxResults: 5})
	source.ensureYTDLP = func(context.Context) error { return nil }
	source.run = func(context.Context, *ytdlp.Command, ...string) (commandOutput, error) {
		return commandOutput{
			Stdout: strings.Join([]string{
				`{"id":"video-1","title":"Song One","artist":"Artist One","album":"Album One","webpage_url":"https://music.youtube.com/watch?v=video-1","duration":123}`,
				`{"id":"live-2","title":"Live Two","uploader":"Channel Two","webpage_url":"https://www.youtube.com/watch?v=live-2","duration":321,"is_live":true}`,
			}, "\n"),
		}, nil
	}

	results, err := source.Search(teaui.SearchRequest{
		SourceID: sourceID,
		Query:    "song",
		Filters:  teaui.DefaultSearchFilters(),
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected two results, got %#v", results)
	}
	if results[0].ID != entryIDPrefix+"https://music.youtube.com/watch?v=video-1" {
		t.Fatalf("unexpected first result id: %#v", results[0])
	}
	if results[0].Artwork.Artist != "Artist One" || results[0].Artwork.Album != "Album One" {
		t.Fatalf("expected metadata carried forward, got %#v", results[0].Artwork)
	}
	if results[1].Kind != teaui.MediaStream {
		t.Fatalf("expected live entry to map to stream, got %#v", results[1])
	}
}

func TestInspectURLFlattensPlaylistEntries(t *testing.T) {
	source := NewSource(Options{Enabled: true, MaxResults: 10})
	source.ensureYTDLP = func(context.Context) error { return nil }
	source.run = func(context.Context, *ytdlp.Command, ...string) (commandOutput, error) {
		return commandOutput{
			Stdout: `{
				"_type":"playlist",
				"title":"Private Mix",
				"entries":[
					{"id":"track-a","title":"Track A","artist":"Artist A","url":"https://music.youtube.com/watch?v=track-a","duration":111},
					{"id":"track-b","title":"Track B","uploader":"Uploader B","duration":222}
				]
			}`,
		}, nil
	}

	results, err := source.Search(teaui.SearchRequest{
		SourceID: sourceID,
		Query:    "https://music.youtube.com/playlist?list=abc",
		Filters:  teaui.DefaultSearchFilters(),
	})
	if err != nil {
		t.Fatalf("playlist inspect failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected flattened playlist entries, got %#v", results)
	}
	if results[1].ID != entryIDPrefix+"https://music.youtube.com/watch?v=track-b" {
		t.Fatalf("expected fallback ID URL generation, got %#v", results[1])
	}
}

func TestResolveUsesCachedFileAndPreservesMetadata(t *testing.T) {
	source := NewSource(Options{Enabled: true, CacheDir: t.TempDir()})
	source.ensureYTDLP = func(context.Context) error { return nil }
	source.ensurePostProcess = func(context.Context) error { return nil }
	source.run = func(context.Context, *ytdlp.Command, ...string) (commandOutput, error) {
		t.Fatal("resolve should use cached file without downloading")
		return commandOutput{}, nil
	}
	source.decode = func(path string) (beep.StreamSeekCloser, beep.Format, error) {
		return &stubStream{length: 48_000}, beep.Format{SampleRate: 48_000, NumChannels: 2, Precision: 2}, nil
	}

	entryID := entryIDPrefix + "https://music.youtube.com/watch?v=cached-track"
	cacheFile, err := source.cacheFilePath(entryURLFromID(entryID))
	if err != nil {
		t.Fatalf("cache path failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(cacheFile, []byte("cached"), 0o644); err != nil {
		t.Fatalf("write cache file failed: %v", err)
	}

	resolved, err := source.Resolve(teaui.QueueEntry{
		ID:       entryID,
		Title:    "Queue Title",
		Subtitle: "Queue Artist",
		Source:   sourceName,
		Duration: 3 * time.Minute,
		Artwork: coverart.Metadata{
			Title:  "Artwork Title",
			Artist: "Artwork Artist",
			Album:  "Artwork Album",
		},
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.Info.Title != "Artwork Title" || resolved.Info.Artist != "Artwork Artist" || resolved.Info.Album != "Artwork Album" {
		t.Fatalf("unexpected resolved info: %#v", resolved.Info)
	}
	if resolved.Format.SampleRate != 48_000 {
		t.Fatalf("unexpected format: %#v", resolved.Format)
	}
}
