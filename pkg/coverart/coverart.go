package coverart

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// This file defines the reusable cover-art domain model. Higher-level providers,
// caches, and adapters all build on these types and contracts, so this is the
// place to look first when understanding how artwork flows through the app.

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
	Title     string
	Album     string
	Artist    string
	RemoteURL string
	IDs       IDs
	Local     *LocalMetadata
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
	if m.RemoteURL == "" {
		m.RemoteURL = fallback.RemoteURL
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
	m.RemoteURL = strings.TrimSpace(m.RemoteURL)
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
		m.RemoteURL == "" &&
		m.IDs == (IDs{}) &&
		m.Local == nil
}

// MetadataURLProvider downloads artwork directly from metadata-supplied remote URLs.
type MetadataURLProvider struct {
	Client *http.Client
}

// Name returns the provider's stable identifier.
func (p MetadataURLProvider) Name() string { return "metadata-url" }

// Lookup fetches artwork directly from Metadata.RemoteURL when present.
func (p MetadataURLProvider) Lookup(ctx context.Context, metadata Metadata) (Result, error) {
	metadata = metadata.Normalize()
	if metadata.RemoteURL == "" {
		return Result{}, ErrNotFound
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadata.RemoteURL, nil)
	if err != nil {
		return Result{}, err
	}

	resp, err := p.client().Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Result{}, ErrNotFound
	default:
		return Result{}, errors.New("metadata artwork fetch returned " + resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}
	if len(data) == 0 {
		return Result{}, ErrNotFound
	}

	return Result{
		Image: Image{
			Data:        data,
			MIMEType:    strings.TrimSpace(resp.Header.Get("Content-Type")),
			Description: "metadata artwork",
		},
		Provider: p.Name(),
	}, nil
}

// Result is a successful provider lookup.
type Result struct {
	Image    Image
	Provider string
}

// AttemptStatus classifies one step in the provider resolution flow.
type AttemptStatus string

// AttemptStatus values capture cache checks, provider execution, and outcomes.
const (
	AttemptCacheHit  AttemptStatus = "cache-hit"
	AttemptCacheMiss AttemptStatus = "cache-miss"
	AttemptTrying    AttemptStatus = "trying"
	AttemptSuccess   AttemptStatus = "success"
	AttemptNotFound  AttemptStatus = "not-found"
	AttemptError     AttemptStatus = "error"
)

// AttemptEvent reports one observable step while resolving artwork.
type AttemptEvent struct {
	Provider string
	Status   AttemptStatus
	Message  string
}

// Provider looks up cover art from one source.
type Provider interface {
	Name() string
	Lookup(ctx context.Context, metadata Metadata) (Result, error)
}

// AttemptReportingProvider can stream provider/cache progress while resolving.
type AttemptReportingProvider interface {
	Provider
	LookupObserved(ctx context.Context, metadata Metadata, report func(AttemptEvent)) (Result, error)
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
	return c.ResolveObserved(ctx, metadata, nil)
}

// ResolveObserved runs providers in order while optionally reporting progress.
func (c *Chain) ResolveObserved(ctx context.Context, metadata Metadata, report func(AttemptEvent)) (Result, error) {
	metadata = metadata.Normalize()
	if metadata.Empty() {
		return Result{}, ErrNotFound
	}
	if c == nil || len(c.providers) == 0 {
		return Result{}, ErrNotFound
	}

	for _, provider := range c.providers {
		result, err := lookupProviderObserved(ctx, provider, metadata, report)
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

func lookupProviderObserved(ctx context.Context, provider Provider, metadata Metadata, report func(AttemptEvent)) (Result, error) {
	if observed, ok := provider.(AttemptReportingProvider); ok {
		result, err := observed.LookupObserved(ctx, metadata, report)
		if err != nil {
			return Result{}, err
		}
		return normalizeResult(result)
	}
	reportAttempt(report, AttemptEvent{Provider: provider.Name(), Status: AttemptTrying, Message: "trying provider"})
	result, err := provider.Lookup(ctx, metadata)
	switch {
	case err == nil:
		result, err = normalizeResult(result)
		if err == nil {
			reportAttempt(report, AttemptEvent{Provider: provider.Name(), Status: AttemptSuccess, Message: "artwork found"})
		} else if errors.Is(err, ErrNotFound) {
			reportAttempt(report, AttemptEvent{Provider: provider.Name(), Status: AttemptNotFound, Message: "artwork format is unsupported"})
		} else {
			reportAttempt(report, AttemptEvent{Provider: provider.Name(), Status: AttemptError, Message: err.Error()})
		}
	case errors.Is(err, ErrNotFound):
		reportAttempt(report, AttemptEvent{Provider: provider.Name(), Status: AttemptNotFound, Message: "no artwork found"})
	default:
		reportAttempt(report, AttemptEvent{Provider: provider.Name(), Status: AttemptError, Message: err.Error()})
	}
	return result, err
}

func reportAttempt(report func(AttemptEvent), event AttemptEvent) {
	if report == nil {
		return
	}
	event.Provider = strings.TrimSpace(event.Provider)
	event.Message = strings.TrimSpace(event.Message)
	report(event)
}

func (p MetadataURLProvider) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return http.DefaultClient
}

func normalizeResult(result Result) (Result, error) {
	image, err := normalizeImage(result.Image)
	if err != nil {
		return Result{}, err
	}
	result.Image = image
	return result, nil
}

func normalizeImage(img Image) (Image, error) {
	img.MIMEType = normalizeImageMIMEType(img.MIMEType)
	img.Description = strings.TrimSpace(img.Description)
	if len(img.Data) == 0 {
		return Image{}, ErrNotFound
	}

	if looksLikeSVG(img.Data, img.MIMEType) {
		pngData, err := rasterizeSVG(img.Data)
		if err != nil {
			return Image{}, fmt.Errorf("%w: svg artwork is not rasterizable", ErrNotFound)
		}
		img.Data = pngData
		img.MIMEType = "image/png"
		return img, nil
	}

	if _, _, err := image.DecodeConfig(bytes.NewReader(img.Data)); err != nil {
		return Image{}, fmt.Errorf("%w: unsupported artwork format", ErrNotFound)
	}
	return img, nil
}

func normalizeImageMIMEType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		if index := strings.IndexByte(value, ';'); index >= 0 {
			value = value[:index]
		}
		return strings.ToLower(strings.TrimSpace(value))
	}
	return strings.ToLower(strings.TrimSpace(mediaType))
}

func looksLikeSVG(data []byte, mimeType string) bool {
	if normalizeImageMIMEType(mimeType) == "image/svg+xml" {
		return true
	}
	snippet := strings.ToLower(string(data))
	return strings.Contains(snippet, "<svg") && strings.Contains(snippet, "</svg>")
}

func rasterizeSVG(data []byte) ([]byte, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data), oksvg.WarnErrorMode)
	if err != nil {
		return nil, err
	}

	width, height, ok := rasterizeDimensions(icon.ViewBox.W, icon.ViewBox.H)
	if !ok {
		return nil, errors.New("svg has invalid dimensions")
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	scanner := rasterx.NewScannerGV(width, height, img, img.Bounds())
	raster := rasterx.NewDasher(width, height, scanner)
	icon.SetTarget(0, 0, float64(width), float64(height))
	icon.Draw(raster, 1.0)

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func rasterizeDimensions(viewWidth, viewHeight float64) (int, int, bool) {
	const maxDimension = 512
	if viewWidth <= 0 || viewHeight <= 0 {
		return 0, 0, false
	}

	width := int(viewWidth + 0.5)
	height := int(viewHeight + 0.5)
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}

	largest := max(height, width)
	if largest > maxDimension {
		width = (width * maxDimension) / largest
		height = (height * maxDimension) / largest
	}
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	return width, height, true
}

// IsNotFound reports whether the error is a provider miss rather than a hard failure.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
