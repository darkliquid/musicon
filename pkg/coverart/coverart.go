package coverart

import (
	"context"
	"errors"
	"strings"
)

// ErrNotFound reports that a provider could not locate cover art for the supplied metadata.
var ErrNotFound = errors.New("cover art not found")

// IDs groups external service identifiers that can improve cover-art lookup.
type IDs struct {
	MusicBrainzReleaseID      string
	MusicBrainzReleaseGroupID string
	MusicBrainzRecordingID    string
	SpotifyAlbumID            string
	SpotifyTrackID            string
	AppleMusicAlbumID         string
	AppleMusicSongID          string
}

// Image describes raw cover-art bytes plus light provenance metadata.
type Image struct {
	Data        []byte
	MIMEType    string
	Description string
}

// LocalMetadata describes optional local-file context for cover-art lookup.
type LocalMetadata struct {
	AudioPath     string
	CoverFilePath string
	Embedded      *Image
}

// Metadata describes the inputs a cover-art provider can use to locate art.
type Metadata struct {
	Title  string
	Album  string
	Artist string
	IDs    IDs
	Local  *LocalMetadata
}

// Merge returns metadata that prefers the receiver's populated fields and fills
// gaps from fallback.
func (m Metadata) Merge(fallback Metadata) Metadata {
	m = m.Normalize()
	fallback = fallback.Normalize()

	if m.Title == "" {
		m.Title = fallback.Title
	}
	if m.Album == "" {
		m.Album = fallback.Album
	}
	if m.Artist == "" {
		m.Artist = fallback.Artist
	}

	if m.IDs.MusicBrainzReleaseID == "" {
		m.IDs.MusicBrainzReleaseID = fallback.IDs.MusicBrainzReleaseID
	}
	if m.IDs.MusicBrainzReleaseGroupID == "" {
		m.IDs.MusicBrainzReleaseGroupID = fallback.IDs.MusicBrainzReleaseGroupID
	}
	if m.IDs.MusicBrainzRecordingID == "" {
		m.IDs.MusicBrainzRecordingID = fallback.IDs.MusicBrainzRecordingID
	}
	if m.IDs.SpotifyAlbumID == "" {
		m.IDs.SpotifyAlbumID = fallback.IDs.SpotifyAlbumID
	}
	if m.IDs.SpotifyTrackID == "" {
		m.IDs.SpotifyTrackID = fallback.IDs.SpotifyTrackID
	}
	if m.IDs.AppleMusicAlbumID == "" {
		m.IDs.AppleMusicAlbumID = fallback.IDs.AppleMusicAlbumID
	}
	if m.IDs.AppleMusicSongID == "" {
		m.IDs.AppleMusicSongID = fallback.IDs.AppleMusicSongID
	}

	switch {
	case m.Local == nil:
		m.Local = fallback.Local
	case fallback.Local != nil:
		if m.Local.AudioPath == "" {
			m.Local.AudioPath = fallback.Local.AudioPath
		}
		if m.Local.CoverFilePath == "" {
			m.Local.CoverFilePath = fallback.Local.CoverFilePath
		}
		if m.Local.Embedded == nil {
			m.Local.Embedded = fallback.Local.Embedded
		}
	}

	return m.Normalize()
}

// Normalize trims metadata and fills zero nested structs with nil.
func (m Metadata) Normalize() Metadata {
	m.Title = strings.TrimSpace(m.Title)
	m.Album = strings.TrimSpace(m.Album)
	m.Artist = strings.TrimSpace(m.Artist)
	m.IDs.MusicBrainzReleaseID = strings.TrimSpace(m.IDs.MusicBrainzReleaseID)
	m.IDs.MusicBrainzReleaseGroupID = strings.TrimSpace(m.IDs.MusicBrainzReleaseGroupID)
	m.IDs.MusicBrainzRecordingID = strings.TrimSpace(m.IDs.MusicBrainzRecordingID)
	m.IDs.SpotifyAlbumID = strings.TrimSpace(m.IDs.SpotifyAlbumID)
	m.IDs.SpotifyTrackID = strings.TrimSpace(m.IDs.SpotifyTrackID)
	m.IDs.AppleMusicAlbumID = strings.TrimSpace(m.IDs.AppleMusicAlbumID)
	m.IDs.AppleMusicSongID = strings.TrimSpace(m.IDs.AppleMusicSongID)
	if m.Local != nil {
		m.Local.AudioPath = strings.TrimSpace(m.Local.AudioPath)
		m.Local.CoverFilePath = strings.TrimSpace(m.Local.CoverFilePath)
		if m.Local.AudioPath == "" && m.Local.CoverFilePath == "" && m.Local.Embedded == nil {
			m.Local = nil
		}
	}
	return m
}

// Empty reports whether the metadata contains no useful lookup inputs.
func (m Metadata) Empty() bool {
	m = m.Normalize()
	return m.Title == "" &&
		m.Album == "" &&
		m.Artist == "" &&
		m.IDs == (IDs{}) &&
		m.Local == nil
}

// Result is a successful provider lookup.
type Result struct {
	Image    Image
	Provider string
}

// Provider looks up cover art from one source.
type Provider interface {
	Name() string
	Lookup(ctx context.Context, metadata Metadata) (Result, error)
}

// Chain resolves cover art through providers in priority order.
type Chain struct {
	providers []Provider
}

// NewChain creates a new priority-ordered provider chain.
func NewChain(providers ...Provider) *Chain {
	filtered := make([]Provider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			filtered = append(filtered, provider)
		}
	}
	return &Chain{providers: filtered}
}

// Providers returns a shallow copy of the configured provider list.
func (c *Chain) Providers() []Provider {
	if c == nil {
		return nil
	}
	return append([]Provider(nil), c.providers...)
}

// Resolve runs providers in order until one returns a usable image.
func (c *Chain) Resolve(ctx context.Context, metadata Metadata) (Result, error) {
	metadata = metadata.Normalize()
	if metadata.Empty() {
		return Result{}, ErrNotFound
	}
	if c == nil || len(c.providers) == 0 {
		return Result{}, ErrNotFound
	}

	for _, provider := range c.providers {
		result, err := provider.Lookup(ctx, metadata)
		switch {
		case err == nil:
			result.Provider = strings.TrimSpace(result.Provider)
			if result.Provider == "" {
				result.Provider = provider.Name()
			}
			return result, nil
		case errors.Is(err, ErrNotFound):
			continue
		default:
			return Result{}, err
		}
	}

	return Result{}, ErrNotFound
}

// IsNotFound reports whether the error is a provider miss rather than a hard failure.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
