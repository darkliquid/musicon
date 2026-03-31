package youtube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	teaui "github.com/darkliquid/musicon/internal/ui"
	"github.com/darkliquid/musicon/pkg/coverart"
	"github.com/gopxl/beep"
	youtubev2 "github.com/kkdai/youtube/v2"
)

type stubStream struct {
	length   int
	position int
}

func (s *stubStream) Stream(samples [][2]float64) (int, bool) { return 0, false }
func (s *stubStream) Err() error                              { return nil }
func (s *stubStream) Len() int                                { return s.length }
func (s *stubStream) Position() int                           { return s.position }
func (s *stubStream) Seek(p int) error                        { s.position = p; return nil }
func (s *stubStream) Close() error                            { return nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type stubYouTubeClient struct {
	getVideo    func(context.Context, string) (*youtubev2.Video, error)
	getPlaylist func(context.Context, string) (*youtubev2.Playlist, error)
}

func (s stubYouTubeClient) GetVideoContext(ctx context.Context, raw string) (*youtubev2.Video, error) {
	return s.getVideo(ctx, raw)
}
func (s stubYouTubeClient) GetPlaylistContext(ctx context.Context, raw string) (*youtubev2.Playlist, error) {
	return s.getPlaylist(ctx, raw)
}

func TestSearchQueryMapsJSONResults(t *testing.T) {
	source := NewSource(Options{Enabled: true, MaxResults: 5})
	source.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(body), `"query":"song"`) {
			t.Fatalf("expected query in request body, got %s", string(body))
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"contents":{"tabbedSearchResultsRenderer":{"tabs":[{"tabRenderer":{"title":"YT Music","content":{"sectionListRenderer":{"contents":[{"musicShelfRenderer":{"contents":[{"musicResponsiveListItemRenderer":{"flexColumns":[{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Album Only"}]}}},{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Album"},{"text":"2024"}]}}}]}} ,{"musicResponsiveListItemRenderer":{"playlistItemData":{"videoId":"video-1"},"flexColumns":[{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Song One"}]}}},{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Artist One","navigationEndpoint":{"browseEndpoint":{"browseId":"artist-1","browseEndpointContextSupportedConfigs":{"browseEndpointContextMusicConfig":{"pageType":"MUSIC_PAGE_TYPE_ARTIST"}}}}},{"text":"Album One","navigationEndpoint":{"browseEndpoint":{"browseId":"album-1","browseEndpointContextSupportedConfigs":{"browseEndpointContextMusicConfig":{"pageType":"MUSIC_PAGE_TYPE_ALBUM"}}}}},{"text":"2:03"}]}}}]}},{"musicResponsiveListItemRenderer":{"playlistItemData":{"videoId":"video-2"},"flexColumns":[{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Video Two"}]}}},{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Channel Two","navigationEndpoint":{"browseEndpoint":{"browseId":"channel-2","browseEndpointContextSupportedConfigs":{"browseEndpointContextMusicConfig":{"pageType":"MUSIC_PAGE_TYPE_USER_CHANNEL"}}}}},{"text":"5:21"}]}}}]}}]}}]}}}}]}}}`)), Header: make(http.Header)}, nil
	})}

	results, err := source.Search(context.Background(), teaui.SearchRequest{SourceID: sourceID, Query: "song", Filters: teaui.DefaultSearchFilters()})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 2 || results[0].Artwork.Artist != "Artist One" || results[1].Subtitle != "Channel Two" {
		t.Fatalf("unexpected search results: %#v", results)
	}
}

func TestSearchQuerySupportsFocusedArtistAndPlaylistModes(t *testing.T) {
	source := NewSource(Options{Enabled: true, MaxResults: 5})
	source.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		switch {
		case strings.Contains(string(body), `"query":"artist"`):
			if !strings.Contains(string(body), `"params":"EgWKAQIgAWoMEA4QChADEAQQCRAF"`) {
				t.Fatalf("expected artist search params in request body, got %s", string(body))
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{
				"contents": {
					"sectionListRenderer": {
						"contents": [
							{
								"musicShelfRenderer": {
									"contents": [
										{
											"musicResponsiveListItemRenderer": {
												"navigationEndpoint": {
													"browseEndpoint": {
														"browseId": "artist-1",
														"browseEndpointContextSupportedConfigs": {
															"browseEndpointContextMusicConfig": {
																"pageType": "MUSIC_PAGE_TYPE_ARTIST"
															}
														}
													}
												},
												"flexColumns": [
													{
														"musicResponsiveListItemFlexColumnRenderer": {
															"text": {
																"runs": [
																	{
																		"text": "Artist One",
																		"navigationEndpoint": {
																			"browseEndpoint": {
																				"browseId": "artist-1",
																				"browseEndpointContextSupportedConfigs": {
																					"browseEndpointContextMusicConfig": {
																						"pageType": "MUSIC_PAGE_TYPE_ARTIST"
																					}
																				}
																			}
																		}
																	}
																]
															}
														}
													}
												]
											}
										}
									]
								}
							}
						]
					}
				}
			}`)), Header: make(http.Header)}, nil
		case strings.Contains(string(body), `"query":"playlist"`):
			if !strings.Contains(string(body), `"params":"Eg-KAQwIABAAGAAgACgBMABqChAEEAMQCRAFEAo%3D"`) {
				t.Fatalf("expected playlist search params in request body, got %s", string(body))
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{
				"contents": {
					"sectionListRenderer": {
						"contents": [
							{
								"musicShelfRenderer": {
									"contents": [
										{
											"musicResponsiveListItemRenderer": {
												"navigationEndpoint": {
													"watchEndpoint": {
														"playlistId": "playlist-1"
													}
												},
												"flexColumns": [
													{
														"musicResponsiveListItemFlexColumnRenderer": {
															"text": {
																"runs": [
																	{
																		"text": "Playlist One"
																	}
																]
															}
														}
													},
													{
														"musicResponsiveListItemFlexColumnRenderer": {
															"text": {
																"runs": [
																	{
																		"text": "Curator"
																	}
																]
															}
														}
													}
												]
											}
										}
									]
								}
							}
						]
					}
				}
			}`)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected request body: %s", string(body))
			return nil, nil
		}
	})}

	artists, err := source.Search(context.Background(), teaui.SearchRequest{SourceID: sourceID, Query: "artist", Mode: teaui.SearchModeArtists, Filters: teaui.DefaultSearchFilters()})
	if err != nil {
		t.Fatalf("artist search failed: %v", err)
	}
	if len(artists) != 1 || artists[0].Kind != teaui.MediaArtist || artists[0].ArtistFilter.Name != "Artist One" {
		t.Fatalf("unexpected artist results: %#v", artists)
	}

	playlists, err := source.Search(context.Background(), teaui.SearchRequest{SourceID: sourceID, Query: "playlist", Mode: teaui.SearchModePlaylists, Filters: teaui.DefaultSearchFilters()})
	if err != nil {
		t.Fatalf("playlist search failed: %v", err)
	}
	if len(playlists) != 1 || playlists[0].Kind != teaui.MediaPlaylist || playlists[0].PlaylistID != "playlist-1" {
		t.Fatalf("unexpected playlist results: %#v", playlists)
	}
}

func TestInspectURLReturnsPlaylistCollectionRow(t *testing.T) {
	source := NewSource(Options{Enabled: true, MaxResults: 10})
	source.yt = stubYouTubeClient{
		getPlaylist: func(context.Context, string) (*youtubev2.Playlist, error) {
			return &youtubev2.Playlist{Title: "Private Mix", Videos: []*youtubev2.PlaylistEntry{{ID: "track-a", Title: "Track A", Author: "Artist A", Duration: 111 * time.Second}, {ID: "track-b", Title: "Track B", Author: "Artist B", Duration: 222 * time.Second}}}, nil
		},
		getVideo: func(context.Context, string) (*youtubev2.Video, error) {
			t.Fatal("unexpected GetVideoContext call")
			return nil, nil
		},
	}

	results, err := source.Search(context.Background(), teaui.SearchRequest{SourceID: sourceID, Query: "https://music.youtube.com/playlist?list=abc", Filters: teaui.DefaultSearchFilters()})
	if err != nil {
		t.Fatalf("playlist inspect failed: %v", err)
	}
	if len(results) != 1 || results[0].Kind != teaui.MediaPlaylist || results[0].CollectionCount != 2 || results[0].PlaylistID != "abc" {
		t.Fatalf("unexpected playlist results: %#v", results)
	}
}

func TestExpandCollectionUsesPlaylistLookup(t *testing.T) {
	source := NewSource(Options{Enabled: true, MaxResults: 10})
	source.yt = stubYouTubeClient{
		getPlaylist: func(context.Context, string) (*youtubev2.Playlist, error) {
			return &youtubev2.Playlist{
				Title: "Private Mix",
				Videos: []*youtubev2.PlaylistEntry{
					{ID: "track-a", Title: "Track A", Author: "Artist A", Duration: 111 * time.Second},
					{ID: "track-b", Title: "Track B", Author: "Artist B", Duration: 222 * time.Second},
				},
			}, nil
		},
		getVideo: func(context.Context, string) (*youtubev2.Video, error) {
			t.Fatal("unexpected GetVideoContext call")
			return nil, nil
		},
	}

	results, err := source.ExpandCollection(context.Background(), teaui.SearchResult{
		ID:         entryIDPrefix + "playlist:abc",
		Title:      "Private Mix",
		Source:     sourceName,
		Kind:       teaui.MediaPlaylist,
		PlaylistID: "abc",
	})
	if err != nil {
		t.Fatalf("expand collection failed: %v", err)
	}
	if len(results) != 2 || results[1].ID != entryIDPrefix+"https://music.youtube.com/watch?v=track-b" {
		t.Fatalf("unexpected expanded playlist results: %#v", results)
	}
}

func TestResolveUsesYTDLPDecodedStream(t *testing.T) {
	source := NewSource(Options{Enabled: true})
	source.yt = stubYouTubeClient{
		getVideo: func(context.Context, string) (*youtubev2.Video, error) {
			return &youtubev2.Video{ID: "fresh-track", Title: "Fresh Title", Author: "Fresh Artist", Duration: 200 * time.Second}, nil
		},
		getPlaylist: func(context.Context, string) (*youtubev2.Playlist, error) {
			t.Fatal("unexpected GetPlaylistContext call")
			return nil, nil
		},
	}
	opened := false
	source.openMedia = func(_ context.Context, rawURL string, duration time.Duration) (beep.StreamSeekCloser, beep.Format, error) {
		opened = true
		if rawURL != "https://music.youtube.com/watch?v=fresh-track" {
			t.Fatalf("unexpected media url: %q", rawURL)
		}
		if duration != 200*time.Second {
			t.Fatalf("unexpected duration: %v", duration)
		}
		return &stubStream{length: 96_000}, beep.Format{SampleRate: 48_000, NumChannels: 2, Precision: 2}, nil
	}

	resolved, err := source.Resolve(teaui.QueueEntry{ID: entryIDPrefix + "https://music.youtube.com/watch?v=fresh-track", Title: "Fresh Title", Subtitle: "Fresh Artist", Source: sourceName, Artwork: coverart.Metadata{Title: "Artwork Title", Artist: "Artwork Artist", Album: "Artwork Album"}})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if !opened || resolved.Info.Title != "Artwork Title" || resolved.Info.Artist != "Artwork Artist" {
		t.Fatalf("unexpected resolved track: %#v", resolved)
	}
}

func TestResolveUsesYTDLPEvenWhenMetadataFails(t *testing.T) {
	source := NewSource(Options{Enabled: true})
	source.yt = stubYouTubeClient{
		getVideo: func(context.Context, string) (*youtubev2.Video, error) {
			return nil, fmt.Errorf("metadata unavailable")
		},
		getPlaylist: func(context.Context, string) (*youtubev2.Playlist, error) {
			t.Fatal("unexpected GetPlaylistContext call")
			return nil, nil
		},
	}
	opened := false
	source.openMedia = func(_ context.Context, rawURL string, duration time.Duration) (beep.StreamSeekCloser, beep.Format, error) {
		opened = true
		if rawURL != "https://music.youtube.com/watch?v=fresh-track" {
			t.Fatalf("unexpected fallback url: %q", rawURL)
		}
		if duration != 0 {
			t.Fatalf("expected zero duration when metadata is unavailable, got %v", duration)
		}
		return &stubStream{length: 48_000}, beep.Format{SampleRate: 48_000, NumChannels: 2, Precision: 2}, nil
	}

	resolved, err := source.Resolve(teaui.QueueEntry{ID: entryIDPrefix + "https://music.youtube.com/watch?v=fresh-track", Title: "Fresh Title", Subtitle: "Fresh Artist", Source: sourceName})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if !opened || resolved.Stream.Len() != 48_000 || resolved.Info.Title != "Fresh Title" || resolved.Info.Artist != "Fresh Artist" {
		t.Fatalf("expected yt-dlp fallback to resolve track, got %#v", resolved)
	}
}

func TestYTDLPArgsIncludeConfiguredOptions(t *testing.T) {
	source := NewSource(Options{
		Enabled:            true,
		CookiesFile:        "/tmp/cookies.txt",
		CookiesFromBrowser: "firefox",
		ExtraArgs:          []string{"--extractor-args", "youtube:player_client=web"},
		CacheDir:           "/tmp/yt-cache",
	})

	args := source.ytDLPArgs("https://music.youtube.com/watch?v=abc")
	expected := []string{
		"--quiet",
		"--no-warnings",
		"--no-progress",
		"--no-playlist",
		"-f", "ba[ext=webm]/ba",
		"-o", "-",
		"--cookies", "/tmp/cookies.txt",
		"--cookies-from-browser", "firefox",
		"--cache-dir", "/tmp/yt-cache",
		"--extractor-args", "youtube:player_client=web",
		"https://music.youtube.com/watch?v=abc",
	}
	if !slices.Equal(args, expected) {
		t.Fatalf("unexpected yt-dlp args: %#v", args)
	}
}

func TestResolveReportsFallbackFailure(t *testing.T) {
	source := NewSource(Options{Enabled: true})
	source.yt = stubYouTubeClient{
		getVideo: func(context.Context, string) (*youtubev2.Video, error) {
			return &youtubev2.Video{ID: "fresh-track"}, nil
		},
		getPlaylist: func(context.Context, string) (*youtubev2.Playlist, error) { return nil, nil },
	}
	source.openMedia = func(context.Context, string, time.Duration) (beep.StreamSeekCloser, beep.Format, error) {
		return nil, beep.Format{}, exec.ErrNotFound
	}

	_, err := source.Resolve(teaui.QueueEntry{ID: entryIDPrefix + "https://music.youtube.com/watch?v=fresh-track"})
	if err == nil || !strings.Contains(err.Error(), "yt-dlp playback failed") {
		t.Fatalf("expected yt-dlp playback error, got %v", err)
	}
}

func TestParseMatroskaBlockWithoutLacing(t *testing.T) {
	block := append([]byte{0x81, 0x00, 0x00, 0x00}, []byte{0xAA, 0xBB, 0xCC}...)
	track, frames, err := parseMatroskaBlock(block)
	if err != nil {
		t.Fatalf("parseMatroskaBlock failed: %v", err)
	}
	if track != 1 || len(frames) != 1 || string(frames[0]) != string([]byte{0xAA, 0xBB, 0xCC}) {
		t.Fatalf("unexpected parsed block: track=%d frames=%v", track, frames)
	}
}

func TestParseOpusHead(t *testing.T) {
	head, err := parseOpusHead([]byte{'O', 'p', 'u', 's', 'H', 'e', 'a', 'd', 1, 2, 0x38, 0x01, 0x80, 0xBB, 0x00, 0x00, 0, 0, 0}, 2)
	if err != nil {
		t.Fatalf("parseOpusHead failed: %v", err)
	}
	if head.Channels != 2 || head.PreSkip != 312 {
		t.Fatalf("unexpected opus head: %#v", head)
	}
}

func TestInterleavedToStereo(t *testing.T) {
	mono := interleavedToStereo([]int16{1, 2, 3}, 1)
	if len(mono) != 6 || mono[0] != 1 || mono[1] != 1 || mono[4] != 3 || mono[5] != 3 {
		t.Fatalf("unexpected mono expansion: %#v", mono)
	}
}

func TestRangeReadSeekerReadsViaHTTPRanges(t *testing.T) {
	payload := []byte("abcdefghijklmnopqrstuvwxyz")
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.Header.Get("Range"))
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var start, end int
		if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end); err != nil {
			t.Fatalf("parse range: %v", err)
		}
		if end >= len(payload) {
			end = len(payload) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[start : end+1])
	}))
	defer server.Close()

	reader, err := newRangeReadSeeker(context.Background(), server.Client(), server.URL, make(http.Header), 5)
	if err != nil {
		t.Fatalf("new range read seeker failed: %v", err)
	}
	defer reader.Close()

	buf := make([]byte, 7)
	n, err := io.ReadFull(reader, buf)
	if err != nil {
		t.Fatalf("first range read failed: %v", err)
	}
	if got := string(buf[:n]); got != "abcdefg" {
		t.Fatalf("unexpected first chunk: %q", got)
	}
	if _, err := reader.Seek(10, io.SeekStart); err != nil {
		t.Fatalf("seek failed: %v", err)
	}
	buf = make([]byte, 4)
	n, err = io.ReadFull(reader, buf)
	if err != nil {
		t.Fatalf("second range read failed: %v", err)
	}
	if got := string(buf[:n]); got != "klmn" {
		t.Fatalf("unexpected second chunk: %q", got)
	}
	if len(requested) < 2 || requested[0] != "bytes=0-4" || requested[1] != "bytes=5-9" {
		t.Fatalf("unexpected range sequence: %#v", requested)
	}
}

func TestRangeReadSeekerReusesDiskCachedBlocksOnReseek(t *testing.T) {
	payload := []byte("abcdefghijklmnopqrstuvwxyz")
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.Header.Get("Range"))
		rangeHeader := r.Header.Get("Range")
		var start, end int
		if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end); err != nil {
			t.Fatalf("parse range: %v", err)
		}
		if end >= len(payload) {
			end = len(payload) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[start : end+1])
	}))
	defer server.Close()

	reader, err := newRangeReadSeeker(context.Background(), server.Client(), server.URL, make(http.Header), 5)
	if err != nil {
		t.Fatalf("new range read seeker failed: %v", err)
	}
	cacheDir := reader.cache.dir()

	buf := make([]byte, 12)
	if _, err := io.ReadFull(reader, buf); err != nil {
		t.Fatalf("populate range cache failed: %v", err)
	}
	if _, err := reader.Seek(2, io.SeekStart); err != nil {
		t.Fatalf("seek back into cached block failed: %v", err)
	}
	buf = make([]byte, 3)
	if _, err := io.ReadFull(reader, buf); err != nil {
		t.Fatalf("read from cached block failed: %v", err)
	}
	if got := string(buf); got != "cde" {
		t.Fatalf("unexpected cached read: %q", got)
	}
	if len(requested) != 3 || requested[0] != "bytes=0-4" || requested[1] != "bytes=5-9" || requested[2] != "bytes=10-14" {
		t.Fatalf("unexpected range sequence: %#v", requested)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Fatalf("expected cache dir to be removed on close, stat err=%v", err)
	}
}

func TestCueSeekableOpusStreamAppendFramesZeroFillsGapAfterSeek(t *testing.T) {
	stream := &cueSeekableOpusStream{
		buffer:       make([]int16, 16*2),
		windowFrames: 16,
		windowStart:  100,
		windowEnd:    100,
		pos:          100,
	}
	stream.cond = sync.NewCond(&stream.mu)
	for i := range stream.buffer {
		stream.buffer[i] = 99
	}

	stream.appendFrames(102, []int16{1, 1, 2, 2})

	for _, sample := range []int{100, 101} {
		idx := (sample % stream.windowFrames) * 2
		if stream.buffer[idx] != 0 || stream.buffer[idx+1] != 0 {
			t.Fatalf("expected zero-filled gap at sample %d, got %d/%d", sample, stream.buffer[idx], stream.buffer[idx+1])
		}
	}
}

func TestPreparedMediaStreamKeepsReplacementSeekCapabilityAcrossSwaps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-0/1")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{'x'})
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	reader, err := newRangeReadSeeker(ctx, server.Client(), server.URL, make(http.Header), 1)
	if err != nil {
		t.Fatalf("new range read seeker failed: %v", err)
	}

	factory := func(_ context.Context, _ io.ReadSeeker, _ time.Duration, startSample int) (beep.StreamSeekCloser, beep.Format, error) {
		return &stubStream{length: 48_000 * 60, position: startSample}, beep.Format{SampleRate: 48_000, NumChannels: 2, Precision: 2}, nil
	}

	stream, _, err := openPreparedMediaStream(ctx, cancel, reader, time.Minute, 0, factory)
	if err != nil {
		t.Fatalf("open prepared media stream failed: %v", err)
	}
	defer stream.Close()

	preparer, ok := stream.(interface {
		PrepareReplacement(target int) (beep.StreamSeekCloser, error)
	})
	if !ok {
		t.Fatal("expected prepared stream to support replacement seeking")
	}

	firstReplacement, err := preparer.PrepareReplacement(10)
	if err != nil {
		t.Fatalf("first replacement failed: %v", err)
	}
	defer firstReplacement.Close()
	if got := firstReplacement.Position(); got != 10 {
		t.Fatalf("expected first replacement position 10, got %d", got)
	}

	secondPreparer, ok := firstReplacement.(interface {
		PrepareReplacement(target int) (beep.StreamSeekCloser, error)
	})
	if !ok {
		t.Fatal("expected first replacement to preserve replacement capability")
	}

	secondReplacement, err := secondPreparer.PrepareReplacement(20)
	if err != nil {
		t.Fatalf("second replacement failed: %v", err)
	}
	defer secondReplacement.Close()
	if got := secondReplacement.Position(); got != 20 {
		t.Fatalf("expected second replacement position 20, got %d", got)
	}
}
