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
	MediaArtist   MediaKind = "artist"
	MediaAlbum    MediaKind = "album"
	MediaPlaylist MediaKind = "playlist"
)

// String returns a user-facing label for the media kind.
func (k MediaKind) String() string {
	switch k {
	case MediaStream:
		return "Stream"
	case MediaArtist:
		return "Artist"
	case MediaAlbum:
		return "Album"
	case MediaPlaylist:
		return "Playlist"
	default:
		return "Track"
	}
}

// SearchMode identifies a source-specific search focus such as songs or albums.
type SearchMode string

// SearchMode values enumerate the focused search modes supported by richer sources.
const (
	SearchModeDefault   SearchMode = ""
	SearchModeAll       SearchMode = "all"
	SearchModeTracks    SearchMode = "tracks"
	SearchModeStreams   SearchMode = "streams"
	SearchModeSongs     SearchMode = "songs"
	SearchModeArtists   SearchMode = "artists"
	SearchModeAlbums    SearchMode = "albums"
	SearchModePlaylists SearchMode = "playlists"
)

// String returns a user-facing label for the search mode.
func (m SearchMode) String() string {
	switch m {
	case SearchModeAll:
		return "All"
	case SearchModeTracks:
		return "Tracks"
	case SearchModeStreams:
		return "Streams"
	case SearchModeArtists:
		return "Artists"
	case SearchModeAlbums:
		return "Albums"
	case SearchModePlaylists:
		return "Playlists"
	default:
		return "Songs"
	}
}

// SearchModeDescriptor describes one source-specific search mode shown by the UI.
type SearchModeDescriptor struct {
	ID   SearchMode
	Name string
}

// SearchArtistFilter captures the active YouTube-artist narrowing chosen from artist-mode results.
type SearchArtistFilter struct {
	ID   string
	Name string
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
	SearchModes []SearchModeDescriptor
	DefaultMode SearchMode
}

// SearchRequest captures one source-scoped search query from the UI.
type SearchRequest struct {
	SourceID     string
	Query        string
	Filters      SearchFilters
	Mode         SearchMode
	ArtistFilter SearchArtistFilter
}

// SearchResult describes one queueable item returned by a source search.
type SearchResult struct {
	ID              string
	Title           string
	Subtitle        string
	Source          string
	Kind            MediaKind
	Duration        time.Duration
	QueueHint       string
	Artwork         coverart.Metadata
	BrowseID        string
	PlaylistID      string
	ArtistFilter    SearchArtistFilter
	CollectionCount int
	CollectionItems []QueueEntry
}

// QueueEntry stores the metadata the playback queue needs for one item.
type QueueEntry struct {
	ID         string
	Title      string
	Subtitle   string
	Source     string
	Kind       MediaKind
	Duration   time.Duration
	Artwork    coverart.Metadata
	GroupID    string
	GroupTitle string
	GroupKind  MediaKind
	GroupIndex int
	GroupSize  int
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

// ArtworkAttempt reports one observable step while artwork resolution runs.
type ArtworkAttempt struct {
	Provider string
	Status   string
	Message  string
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
	ExpandCollection(context.Context, SearchResult) ([]SearchResult, error)
}

// QueueService provides queue snapshots and queue mutations to the UI.
type QueueService interface {
	Snapshot() []QueueEntry
	Add(SearchResult) error
	Move(id string, delta int) error
	Remove(id string) error
	RemoveGroup(groupID string) error
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
	ArtworkObserved(metadata coverart.Metadata, report func(ArtworkAttempt)) (*components.ImageSource, error)
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
