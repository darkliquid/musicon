package ui

import (
	"time"

	"github.com/darkliquid/musicon/pkg/components"
	"github.com/darkliquid/musicon/pkg/coverart"
)

type Mode int

const (
	ModeQueue Mode = iota
	ModePlayback
)

func (m Mode) String() string {
	switch m {
	case ModePlayback:
		return "Playback"
	default:
		return "Queue"
	}
}

type PlaybackPane int

const (
	PaneArtwork PlaybackPane = iota
	PaneLyrics
	PaneEQ
	PaneVisualizer
)

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

type MediaKind string

const (
	MediaTrack    MediaKind = "track"
	MediaStream   MediaKind = "stream"
	MediaPlaylist MediaKind = "playlist"
)

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

type SearchFilters struct {
	Tracks    bool
	Streams   bool
	Playlists bool
}

func DefaultSearchFilters() SearchFilters {
	return SearchFilters{Tracks: true, Streams: true, Playlists: true}
}

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

type SourceDescriptor struct {
	ID          string
	Name        string
	Description string
}

type SearchRequest struct {
	SourceID string
	Query    string
	Filters  SearchFilters
}

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

type QueueEntry struct {
	ID       string
	Title    string
	Subtitle string
	Source   string
	Kind     MediaKind
	Duration time.Duration
	Artwork  coverart.Metadata
}

type TrackInfo struct {
	ID       string
	Title    string
	Artist   string
	Album    string
	Source   string
	Duration time.Duration
	Artwork  coverart.Metadata
}

func (t TrackInfo) CoverArtMetadata() coverart.Metadata {
	return t.Artwork.Merge(coverart.Metadata{
		Title:  t.Title,
		Album:  t.Album,
		Artist: t.Artist,
	})
}

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

type SearchService interface {
	Sources() []SourceDescriptor
	Search(SearchRequest) ([]SearchResult, error)
}

type QueueService interface {
	Snapshot() []QueueEntry
	Add(SearchResult) error
	Move(id string, delta int) error
	Remove(id string) error
	Clear() error
}

type PlaybackService interface {
	Snapshot() PlaybackSnapshot
	TogglePause() error
	Previous() error
	Next() error
	Seek(delta time.Duration) error
	AdjustVolume(delta int) error
	SetRepeat(repeat bool) error
	SetStream(stream bool) error
}

type LyricsProvider interface {
	Lyrics(trackID string) ([]string, error)
}

type ArtworkProvider interface {
	Artwork(metadata coverart.Metadata) (*components.ImageSource, error)
}

type VisualizationProvider interface {
	Placeholder(mode PlaybackPane, width, height int) (string, error)
}

type Services struct {
	Search        SearchService
	Queue         QueueService
	Playback      PlaybackService
	Lyrics        LyricsProvider
	Artwork       ArtworkProvider
	Visualization VisualizationProvider
}

type Options struct {
	StartMode      Mode
	Theme          string
	CellWidthRatio float64
	AlbumArt       AlbumArtOptions
}

type AlbumArtOptions struct {
	FillMode string
	Protocol string
}
