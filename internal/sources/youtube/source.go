// Package youtube implements Musicon's YouTube Music provider.
//
// The package intentionally separates "what the source does" from "how bytes are
// fetched and decoded":
//   - source.go keeps the top-level Source surface small
//   - search.go owns search and URL inspection
//   - media.go owns yt-dlp extraction and ranged HTTP reads
//   - stream.go owns WebM/Opus buffering and decode
//
// That split matters because the package has to satisfy two different app
// contracts at once: `ui.SearchService` for discovery and `audio.Resolver` for
// playable audio streams.
package youtube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/darkliquid/musicon/internal/audio"
	teaui "github.com/darkliquid/musicon/internal/ui"
	"github.com/gopxl/beep"
	youtubev2 "github.com/kkdai/youtube/v2"
)

const (
	sourceID              = "youtube-music"
	sourceName            = "YouTube Music"
	entryIDPrefix         = "youtube:"
	defaultMaxResults     = 20
	defaultSearchTimeout  = 20 * time.Second
	defaultResolveTimeout = 10 * time.Minute
	musicSearchEndpoint   = "https://music.youtube.com/youtubei/v1/search?prettyPrint=false"
	musicClientName       = "WEB_REMIX"
	musicClientVersion    = "1.20240417.01.01"
	opusFrameSize         = 5760
	initialBufferDuration = 250 * time.Millisecond
	initialPCMBufferBytes = 1 << 20
	mediaRequestBlockSize = 256 << 10
	farSeekDebounce       = 80 * time.Millisecond
	seekSettleDuration    = 120 * time.Millisecond
)

// Options configures the YouTube Music source.
type Options struct {
	Enabled            bool
	MaxResults         int
	CookiesFile        string
	CookiesFromBrowser string
	ExtraArgs          []string
	CacheDir           string
}

type youtubeClient interface {
	GetVideoContext(context.Context, string) (*youtubev2.Video, error)
	GetPlaylistContext(context.Context, string) (*youtubev2.Playlist, error)
}

// Source is the high-level provider object wired into the rest of the app.
//
// It deliberately stores a few function fields (`openMedia`, `streamDecode`,
// `openYTDLP`) so tests can replace the expensive external/media pieces without
// stubbing the entire provider.
//
// In other words, this type is where discovery and playback resolution meet,
// while the lower-level helpers in sibling files handle the transport details.
type Source struct {
	enabled        bool
	maxResults     int
	httpClient     *http.Client
	searchEndpoint string
	cookiesFile    string
	cookiesBrowser string
	extraArgs      []string
	cacheDir       string
	yt             youtubeClient
	openMedia      func(context.Context, string, time.Duration) (beep.StreamSeekCloser, beep.Format, error)
	streamDecode   func(context.Context, io.ReadCloser, func() error, int) (beep.StreamSeekCloser, beep.Format, error)
	openYTDLP      func(context.Context, string) (io.ReadCloser, func() error, error)
}

// NewSource constructs a provider that:
//   - searches YouTube Music through its HTTP endpoint
//   - inspects pasted URLs through youtube/v2
//   - resolves playback by asking yt-dlp for a final media URL
//   - decodes WebM/Opus in-process instead of shelling out to ffmpeg
func NewSource(options Options) *Source {
	httpClient := http.DefaultClient
	source := &Source{
		enabled:        options.Enabled,
		maxResults:     normalizeMaxResults(options.MaxResults),
		httpClient:     httpClient,
		searchEndpoint: musicSearchEndpoint,
		cookiesFile:    strings.TrimSpace(options.CookiesFile),
		cookiesBrowser: strings.TrimSpace(options.CookiesFromBrowser),
		extraArgs:      append([]string(nil), options.ExtraArgs...),
		cacheDir:       strings.TrimSpace(options.CacheDir),
		yt:             &youtubev2.Client{HTTPClient: httpClient},
		openMedia:      nil,
		streamDecode:   streamWebMOpus,
	}
	source.openYTDLP = source.openYTDLPStream
	source.openMedia = source.openYTDLPMedia
	return source
}

// Sources reports the YouTube Music descriptor exposed to the UI.
func (s *Source) Sources() []teaui.SourceDescriptor {
	if s == nil || !s.enabled {
		return nil
	}
	return []teaui.SourceDescriptor{{
		ID:          sourceID,
		Name:        sourceName,
		Description: "Search YouTube Music directly and play streams through yt-dlp plus pure-Go Opus decode.",
		SearchModes: []teaui.SearchModeDescriptor{
			{ID: teaui.SearchModeSongs, Name: teaui.SearchModeSongs.String()},
			{ID: teaui.SearchModeArtists, Name: teaui.SearchModeArtists.String()},
			{ID: teaui.SearchModeAlbums, Name: teaui.SearchModeAlbums.String()},
			{ID: teaui.SearchModePlaylists, Name: teaui.SearchModePlaylists.String()},
		},
		DefaultMode: teaui.SearchModeSongs,
	}}
}

// Search routes plain-text queries to the YouTube Music search endpoint and URL
// inputs to metadata inspection.
//
// This keeps queue UX predictable: users can paste a concrete URL for exact
// resolution, while ordinary text stays on the fast search path.
func (s *Source) Search(ctx context.Context, request teaui.SearchRequest) ([]teaui.SearchResult, error) {
	if s == nil || !s.enabled {
		return nil, nil
	}
	if request.SourceID != "" && request.SourceID != "all" && request.SourceID != sourceID {
		return nil, nil
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, defaultSearchTimeout)
	defer cancel()

	if looksLikeURL(query) {
		if !isYouTubeURL(query) {
			return nil, nil
		}
		return s.inspectURL(ctx, query, request)
	}
	return s.searchQuery(ctx, query, request)
}

// ExpandCollection resolves a searched album or playlist row into its child songs.
func (s *Source) ExpandCollection(ctx context.Context, result teaui.SearchResult) ([]teaui.SearchResult, error) {
	if s == nil || !s.enabled {
		return nil, nil
	}
	if result.Source != "" && !strings.EqualFold(result.Source, sourceName) && !strings.EqualFold(result.Source, "youtube") {
		return nil, nil
	}
	if result.Kind != teaui.MediaAlbum && result.Kind != teaui.MediaPlaylist {
		return nil, nil
	}
	return s.expandCollection(ctx, result)
}

// Resolve turns a queued YouTube entry into a playable track.
//
// Resolution intentionally combines two different data sources:
//   - youtube/v2 for richer metadata when available
//   - yt-dlp for the actual media URL + request headers needed for playback
//
// If metadata lookup fails but yt-dlp playback still succeeds, Resolve returns
// a playable track and falls back to queue-provided metadata.
func (s *Source) Resolve(entry teaui.QueueEntry) (audio.ResolvedTrack, error) {
	if s == nil || !s.enabled {
		return audio.ResolvedTrack{}, errors.New("youtube source is disabled")
	}
	if !OwnsEntryID(entry.ID) {
		return audio.ResolvedTrack{}, fmt.Errorf("youtube source cannot resolve %q", entry.ID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultResolveTimeout)
	defer cancel()

	duration := entry.Duration
	video, metadataErr := s.yt.GetVideoContext(ctx, entryURLFromID(entry.ID))
	duration = firstDuration(duration, videoDuration(video))
	if s.openMedia == nil {
		return audio.ResolvedTrack{}, errors.New("youtube source has no media opener")
	}
	decoded, formatInfo, err := s.openMedia(ctx, entryURLFromID(entry.ID), duration)
	if err != nil {
		if metadataErr != nil {
			return audio.ResolvedTrack{}, fmt.Errorf("load youtube video metadata: %v; yt-dlp playback failed: %w", metadataErr, err)
		}
		return audio.ResolvedTrack{}, fmt.Errorf("yt-dlp playback failed: %w", err)
	}

	info := teaui.TrackInfo{
		ID:       entry.ID,
		Title:    firstNonEmpty(entry.Artwork.Title, videoTitle(video), entry.Title),
		Artist:   firstNonEmpty(entry.Artwork.Artist, videoAuthor(video), entry.Subtitle),
		Album:    entry.Artwork.Album,
		Source:   firstNonEmpty(entry.Source, sourceName),
		Duration: firstDuration(entry.Duration, videoDuration(video)),
		Artwork:  entry.Artwork.Normalize(),
	}

	return audio.ResolvedTrack{Info: info, Format: formatInfo, Stream: decoded}, nil
}

var _ teaui.SearchService = (*Source)(nil)
var _ audio.Resolver = (*Source)(nil)
