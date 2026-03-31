package ui

import (
	"context"
	"time"

	"github.com/darkliquid/musicon/pkg/components"
	"github.com/darkliquid/musicon/pkg/coverart"
)

// Mode identifies the active top-level Musicon screen.
type Mode int

// Mode values enumerate Musicon's dedicated fullscreen screens.
const (
	ModeQueue Mode = iota
	ModePlayback
)

// String returns a user-facing label for the mode.
func (m Mode) String() string {
	switch m {
	case ModePlayback:
		return "Playback"
	default:
		return "Queue"
	}
}

// PlaybackPane identifies the center-pane content shown in playback mode.
type PlaybackPane int

// PlaybackPane values enumerate the playback-screen center panes.
const (
	PaneArtwork PlaybackPane = iota
	PaneLyrics
	PaneEQ
	PaneVisualizer
)

// String returns a user-facing label for the playback pane.
func (p PlaybackPane) String() string {
	switch p {
	case PaneLyrics:
		return "Lyrics"
	case PaneEQ:
		return "EQ"
	case PaneVisualizer:
		return "Visualizer"
	default:
		return "Artwork"
	}
}

// MediaKind classifies searchable items exposed by a source.
type MediaKind string

// MediaKind values classify queueable results.
const (
	MediaTrack    MediaKind = "track"
	MediaStream   MediaKind = "stream"
	MediaPlaylist MediaKind = "playlist"
)

// String returns a user-facing label for the media kind.
func (k MediaKind) String() string {
	switch k {
	case MediaStream:
		return "Stream"
	case MediaPlaylist:
		return "Playlist"
	default:
		return "Track"
	}
}

// SearchFilters declares which media kinds should be included in queue searches.
type SearchFilters struct {
	Tracks    bool
	Streams   bool
	Playlists bool
}

// DefaultSearchFilters enables every supported media kind.
func DefaultSearchFilters() SearchFilters {
	return SearchFilters{Tracks: true, Streams: true, Playlists: true}
}

// Toggle flips one media-kind filter while keeping at least one filter enabled.
func (f *SearchFilters) Toggle(kind MediaKind) {
	switch kind {
	case MediaTrack:
		if f.Streams || f.Playlists || !f.Tracks {
			f.Tracks = !f.Tracks
		}
	case MediaStream:
		if f.Tracks || f.Playlists || !f.Streams {
			f.Streams = !f.Streams
		}
	case MediaPlaylist:
		if f.Tracks || f.Streams || !f.Playlists {
			f.Playlists = !f.Playlists
		}
	}
}

// Matches reports whether the supplied media kind is currently enabled.
func (f SearchFilters) Matches(kind MediaKind) bool {
	switch kind {
	case MediaStream:
		return f.Streams
	case MediaPlaylist:
		return f.Playlists
	default:
		return f.Tracks
	}
}

// SourceDescriptor describes one searchable source exposed to the UI.
type SourceDescriptor struct {
	ID          string
	Name        string
	Description string
}

// SearchRequest captures one source-scoped search query from the UI.
type SearchRequest struct {
	SourceID string
	Query    string
	Filters  SearchFilters
}

// SearchResult describes one queueable item returned by a source search.
type SearchResult struct {
	ID        string
	Title     string
	Subtitle  string
	Source    string
	Kind      MediaKind
	Duration  time.Duration
	QueueHint string
	Artwork   coverart.Metadata
}

// QueueEntry stores the metadata the playback queue needs for one item.
type QueueEntry struct {
	ID       string
	Title    string
	Subtitle string
	Source   string
	Kind     MediaKind
	Duration time.Duration
	Artwork  coverart.Metadata
}

// TrackInfo describes the currently resolved track for playback and artwork display.
type TrackInfo struct {
	ID       string
	Title    string
	Artist   string
	Album    string
	Source   string
	Duration time.Duration
	Artwork  coverart.Metadata
}

// CoverArtMetadata merges resolved track fields into normalized cover-art lookup metadata.
func (t TrackInfo) CoverArtMetadata() coverart.Metadata {
	return t.Artwork.Merge(coverart.Metadata{
		Title:  t.Title,
		Album:  t.Album,
		Artist: t.Artist,
	})
}

// PlaybackSnapshot reports the current queue and playback state visible to the UI.
type PlaybackSnapshot struct {
	Track       *TrackInfo
	Paused      bool
	Repeat      bool
	Stream      bool
	Volume      int
	Position    time.Duration
	Duration    time.Duration
	QueueIndex  int
	QueueLength int
}

// SearchService provides source discovery and queue search results to the UI.
type SearchService interface {
	Sources() []SourceDescriptor
	Search(context.Context, SearchRequest) ([]SearchResult, error)
}

// QueueService provides queue snapshots and queue mutations to the UI.
type QueueService interface {
	Snapshot() []QueueEntry
	Add(SearchResult) error
	Move(id string, delta int) error
	Remove(id string) error
	Clear() error
}

// PlaybackService provides transport controls and playback snapshots to the UI.
type PlaybackService interface {
	Snapshot() PlaybackSnapshot
	TogglePause() error
	Previous() error
	Next() error
	SeekTo(target time.Duration) error
	AdjustVolume(delta int) error
	SetRepeat(repeat bool) error
	SetStream(stream bool) error
}

// LyricsProvider supplies optional lyrics for the active track.
type LyricsProvider interface {
	Lyrics(trackID string) ([]string, error)
}

// ArtworkProvider supplies optional artwork for the active track.
type ArtworkProvider interface {
	Artwork(metadata coverart.Metadata) (*components.ImageSource, error)
}

// VisualizationProvider supplies optional visualization placeholder content.
type VisualizationProvider interface {
	Placeholder(mode PlaybackPane, width, height int) (string, error)
}

// Services groups the backend-facing dependencies injected into the UI.
type Services struct {
	Search        SearchService
	Queue         QueueService
	Playback      PlaybackService
	Lyrics        LyricsProvider
	Artwork       ArtworkProvider
	Visualization VisualizationProvider
}

// Options configures the root UI shell at startup.
type Options struct {
	StartMode      Mode
	Theme          string
	CellWidthRatio float64
	AlbumArt       AlbumArtOptions
	Keybinds       KeybindOptions
}

// AlbumArtOptions configures playback artwork rendering defaults.
type AlbumArtOptions struct {
	FillMode string
	Protocol string
}
