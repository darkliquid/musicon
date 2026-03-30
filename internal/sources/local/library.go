package local

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/musicon/internal/audio"
	teaui "github.com/darkliquid/musicon/internal/ui"
	"github.com/darkliquid/musicon/pkg/coverart"
	"github.com/dhowden/tag"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/flac"
	"github.com/gopxl/beep/mp3"
	"github.com/gopxl/beep/vorbis"
	"github.com/gopxl/beep/wav"
)

const sourceID = "local-files"
const defaultRefreshInterval = 2 * time.Second

var supportedExtensions = map[string]struct{}{
	".mp3":  {},
	".wav":  {},
	".flac": {},
	".ogg":  {},
	".oga":  {},
}

// Library provides a concrete local-file source and resolver.
type Library struct {
	root string

	RefreshInterval time.Duration

	mu       sync.RWMutex
	scanned  bool
	scanErr  error
	lastScan time.Time
	index    []indexedTrack
	byID     map[string]indexedTrack
}

type indexedTrack struct {
	path     string
	result   teaui.SearchResult
	track    teaui.TrackInfo
	haystack string
}

// NewLibrary constructs a local-file source rooted at root.
func NewLibrary(root string) *Library {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	return &Library{
		root:            root,
		RefreshInterval: defaultRefreshInterval,
	}
}

func (l *Library) Sources() []teaui.SourceDescriptor {
	return []teaui.SourceDescriptor{{
		ID:          sourceID,
		Name:        "Local files",
		Description: "Search and play local audio files from " + l.root,
	}}
}

func (l *Library) Search(request teaui.SearchRequest) ([]teaui.SearchResult, error) {
	if request.SourceID != "" && request.SourceID != "all" && request.SourceID != sourceID {
		return nil, nil
	}
	if !request.Filters.Matches(teaui.MediaTrack) {
		return nil, nil
	}
	tracks, err := l.scan()
	if err != nil {
		return nil, err
	}
	query := normalizeQuery(request.Query)
	if query.raw == "" {
		return nil, nil
	}

	results := make([]teaui.SearchResult, 0, min(len(tracks), 200))
	for _, track := range tracks {
		if !matchesQuery(track.haystack, query) {
			continue
		}
		results = append(results, track.result)
		if len(results) >= 200 {
			break
		}
	}
	return results, nil
}

func (l *Library) Resolve(entry teaui.QueueEntry) (audio.ResolvedTrack, error) {
	tracks, err := l.scan()
	if err != nil {
		return audio.ResolvedTrack{}, err
	}
	_ = tracks

	l.mu.RLock()
	indexed, ok := l.byID[entry.ID]
	l.mu.RUnlock()
	if !ok {
		return audio.ResolvedTrack{}, fmt.Errorf("local file %q not found in source library", entry.ID)
	}

	file, err := os.Open(indexed.path)
	if err != nil {
		return audio.ResolvedTrack{}, err
	}

	stream, format, err := decodeFile(indexed.path, file)
	if err != nil {
		_ = file.Close()
		return audio.ResolvedTrack{}, err
	}

	info := indexed.track
	info.Artwork = info.Artwork.Merge(entry.Artwork)
	return audio.ResolvedTrack{
		Info:   info,
		Format: format,
		Stream: stream,
	}, nil
}

func (l *Library) scan() ([]indexedTrack, error) {
	l.mu.RLock()
	if !l.needsRefreshLocked() {
		defer l.mu.RUnlock()
		return append([]indexedTrack(nil), l.index...), l.scanErr
	}
	l.mu.RUnlock()

	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.needsRefreshLocked() {
		return append([]indexedTrack(nil), l.index...), l.scanErr
	}

	root, err := filepath.Abs(l.root)
	if err != nil {
		l.scanned = true
		l.scanErr = err
		return nil, err
	}

	index := make([]indexedTrack, 0, 128)
	byID := make(map[string]indexedTrack)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if _, ok := supportedExtensions[strings.ToLower(filepath.Ext(path))]; !ok {
			return nil
		}
		track, err := indexTrack(root, path)
		if err != nil {
			return nil
		}
		index = append(index, track)
		byID[track.result.ID] = track
		return nil
	})

	l.scanned = true
	l.scanErr = err
	l.lastScan = time.Now()
	l.index = index
	l.byID = byID
	return append([]indexedTrack(nil), l.index...), err
}

func (l *Library) needsRefreshLocked() bool {
	if !l.scanned {
		return true
	}
	interval := l.RefreshInterval
	if interval <= 0 {
		return true
	}
	return time.Since(l.lastScan) >= interval
}

func indexTrack(root, path string) (indexedTrack, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return indexedTrack{}, err
	}
	relPath, err := filepath.Rel(root, absPath)
	if err != nil {
		relPath = filepath.Base(absPath)
	}
	baseTitle := strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))
	result := teaui.SearchResult{
		ID:     absPath,
		Title:  baseTitle,
		Source: "Local files",
		Kind:   teaui.MediaTrack,
		Artwork: coverart.Metadata{
			Local: &coverart.LocalMetadata{
				AudioPath: absPath,
			},
		},
	}
	track := teaui.TrackInfo{
		ID:      absPath,
		Title:   baseTitle,
		Source:  "Local files",
		Artwork: result.Artwork,
	}

	if metadata, err := readTagMetadata(absPath); err == nil {
		if metadata.Title != "" {
			result.Title = metadata.Title
			track.Title = metadata.Title
		}
		if metadata.Artist != "" {
			result.Subtitle = metadata.Artist
			track.Artist = metadata.Artist
		}
		if metadata.Album != "" && result.Subtitle == "" {
			result.Subtitle = metadata.Album
		}
		track.Album = metadata.Album
		track.Artwork = metadata
		result.Artwork = metadata
	}

	haystack := searchableText(
		result.Title,
		track.Artist,
		track.Album,
		filepath.Base(absPath),
		absPath,
		relPath,
		filepath.ToSlash(absPath),
		filepath.ToSlash(relPath),
	)
	return indexedTrack{
		path:     absPath,
		result:   result,
		track:    track,
		haystack: haystack,
	}, nil
}

func readTagMetadata(path string) (coverart.Metadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return coverart.Metadata{}, err
	}
	defer file.Close()

	parsed, err := tag.ReadFrom(file)
	if err != nil {
		return coverart.Metadata{}, err
	}
	metadata := coverart.Metadata{
		Title:  strings.TrimSpace(parsed.Title()),
		Album:  strings.TrimSpace(parsed.Album()),
		Artist: firstNonEmpty(parsed.Artist(), parsed.AlbumArtist(), parsed.Composer()),
		Local: &coverart.LocalMetadata{
			AudioPath: path,
		},
	}
	if picture := parsed.Picture(); picture != nil && len(picture.Data) > 0 {
		metadata.Local.Embedded = &coverart.Image{
			Data:        picture.Data,
			MIMEType:    picture.MIMEType,
			Description: picture.Description,
		}
	}
	metadata.IDs = idsFromRawTags(parsed.Raw())
	return metadata.Normalize(), nil
}

func idsFromRawTags(raw map[string]interface{}) coverart.IDs {
	var ids coverart.IDs
	for key, value := range raw {
		normalized := normalizeTagKey(key)
		text := strings.TrimSpace(stringifyTagValue(value))
		if text == "" {
			continue
		}
		switch normalized {
		case "musicbrainzalbumid", "musicbrainzreleaseid":
			if ids.MusicBrainzReleaseID == "" {
				ids.MusicBrainzReleaseID = text
			}
		case "musicbrainzreleasegroupid":
			if ids.MusicBrainzReleaseGroupID == "" {
				ids.MusicBrainzReleaseGroupID = text
			}
		case "musicbrainztrackid", "musicbrainzrecordingid":
			if ids.MusicBrainzRecordingID == "" {
				ids.MusicBrainzRecordingID = text
			}
		case "spotifyalbumid":
			if ids.SpotifyAlbumID == "" {
				ids.SpotifyAlbumID = text
			}
		case "spotifytrackid":
			if ids.SpotifyTrackID == "" {
				ids.SpotifyTrackID = text
			}
		case "applemusicalbumid", "itunesalbumid":
			if ids.AppleMusicAlbumID == "" {
				ids.AppleMusicAlbumID = text
			}
		case "applemusicsongid", "itunestrackid", "applemusictrackid":
			if ids.AppleMusicSongID == "" {
				ids.AppleMusicSongID = text
			}
		}
	}
	return ids
}

func normalizeTagKey(key string) string {
	var builder strings.Builder
	builder.Grow(len(key))
	for _, r := range strings.ToLower(key) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func stringifyTagValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case []string:
		return strings.Join(v, " ")
	case []byte:
		return string(v)
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func searchableText(values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		for _, variant := range normalizedSearchVariants(value) {
			if variant != "" {
				parts = append(parts, variant)
			}
		}
	}
	return strings.Join(parts, " ")
}

type normalizedQuery struct {
	raw    string
	tokens []string
}

func normalizeQuery(query string) normalizedQuery {
	raw := normalizeSearchText(query)
	if raw == "" {
		return normalizedQuery{}
	}
	return normalizedQuery{
		raw:    raw,
		tokens: strings.Fields(raw),
	}
}

func matchesQuery(haystack string, query normalizedQuery) bool {
	if query.raw == "" {
		return false
	}

	if strings.Contains(haystack, query.raw) {
		return true
	}

	for _, token := range query.tokens {
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}

func normalizedSearchVariants(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	variants := []string{
		normalizeSearchText(value),
		normalizeSearchText(filepath.ToSlash(value)),
		normalizeSearchText(strings.ReplaceAll(filepath.ToSlash(value), "/", " ")),
	}

	seen := make(map[string]struct{}, len(variants))
	unique := make([]string, 0, len(variants))
	for _, variant := range variants {
		if variant == "" {
			continue
		}
		if _, ok := seen[variant]; ok {
			continue
		}
		seen[variant] = struct{}{}
		unique = append(unique, variant)
	}
	return unique
}

func normalizeSearchText(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	return strings.Join(strings.Fields(value), " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func decodeFile(path string, file *os.File) (beep.StreamSeekCloser, beep.Format, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3":
		return mp3.Decode(file)
	case ".wav":
		return wav.Decode(file)
	case ".flac":
		return flac.Decode(file)
	case ".ogg", ".oga":
		return vorbis.Decode(file)
	default:
		return nil, beep.Format{}, fmt.Errorf("unsupported local audio format %q", ext)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ teaui.SearchService = (*Library)(nil)
var _ audio.Resolver = (*Library)(nil)
