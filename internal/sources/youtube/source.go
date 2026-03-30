package youtube

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/musicon/internal/audio"
	teaui "github.com/darkliquid/musicon/internal/ui"
	"github.com/darkliquid/musicon/pkg/coverart"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/mp3"
	"github.com/lrstanley/go-ytdlp"
)

const (
	sourceID              = "youtube-music"
	sourceName            = "YouTube Music"
	entryIDPrefix         = "youtube:"
	defaultMaxResults     = 20
	defaultSearchTimeout  = 90 * time.Second
	defaultResolveTimeout = 10 * time.Minute
)

type commandOutput struct {
	Stdout string
	Stderr string
}

// Options configures the yt-dlp-backed YouTube source.
type Options struct {
	Enabled            bool
	MaxResults         int
	CookiesFile        string
	CookiesFromBrowser string
	ExtraArgs          []string
	CacheDir           string
}

// Source provides YouTube-backed search and cached playback resolution.
type Source struct {
	enabled            bool
	maxResults         int
	cookiesFile        string
	cookiesFromBrowser string
	extraArgs          []string
	cacheDir           string

	installOnce sync.Once
	installErr  error

	postProcessOnce sync.Once
	postProcessErr  error

	run               func(context.Context, *ytdlp.Command, ...string) (commandOutput, error)
	ensureYTDLP       func(context.Context) error
	ensurePostProcess func(context.Context) error
	decode            func(string) (beep.StreamSeekCloser, beep.Format, error)
}

type ytEntry struct {
	Type        string    `json:"_type"`
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Artist      string    `json:"artist"`
	Album       string    `json:"album"`
	Uploader    string    `json:"uploader"`
	Channel     string    `json:"channel"`
	WebpageURL  string    `json:"webpage_url"`
	OriginalURL string    `json:"original_url"`
	URL         string    `json:"url"`
	Duration    float64   `json:"duration"`
	LiveStatus  string    `json:"live_status"`
	IsLive      bool      `json:"is_live"`
	Entries     []ytEntry `json:"entries"`
}

// NewSource constructs a YouTube-backed source using yt-dlp under the hood.
func NewSource(options Options) *Source {
	s := &Source{
		enabled:            options.Enabled,
		maxResults:         normalizeMaxResults(options.MaxResults),
		cookiesFile:        strings.TrimSpace(options.CookiesFile),
		cookiesFromBrowser: strings.TrimSpace(options.CookiesFromBrowser),
		extraArgs:          normalizeExtraArgs(options.ExtraArgs),
		cacheDir:           strings.TrimSpace(options.CacheDir),
	}
	s.run = s.runCommand
	s.ensureYTDLP = s.ensureYTDLPInstalled
	s.ensurePostProcess = s.ensurePostProcessorsInstalled
	s.decode = decodeMP3File
	return s
}

func (s *Source) Sources() []teaui.SourceDescriptor {
	if s == nil || !s.enabled {
		return nil
	}
	description := "Search and play YouTube Music via yt-dlp."
	if s.cookiesFile != "" || s.cookiesFromBrowser != "" {
		description += " Auth is configured for private playlists and uploads."
	}
	return []teaui.SourceDescriptor{{
		ID:          sourceID,
		Name:        sourceName,
		Description: description,
	}}
}

func (s *Source) Search(request teaui.SearchRequest) ([]teaui.SearchResult, error) {
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

	ctx, cancel := context.WithTimeout(context.Background(), defaultSearchTimeout)
	defer cancel()

	if err := s.ensureYTDLP(ctx); err != nil {
		return nil, err
	}

	if looksLikeURL(query) {
		if !isYouTubeURL(query) {
			return nil, nil
		}
		return s.inspectURL(ctx, query, request.Filters)
	}
	return s.searchQuery(ctx, query, request.Filters)
}

func (s *Source) Resolve(entry teaui.QueueEntry) (audio.ResolvedTrack, error) {
	if s == nil || !s.enabled {
		return audio.ResolvedTrack{}, errors.New("youtube source is disabled")
	}
	if !OwnsEntryID(entry.ID) {
		return audio.ResolvedTrack{}, fmt.Errorf("youtube source cannot resolve %q", entry.ID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultResolveTimeout)
	defer cancel()

	if err := s.ensureYTDLP(ctx); err != nil {
		return audio.ResolvedTrack{}, err
	}
	if err := s.ensurePostProcess(ctx); err != nil {
		return audio.ResolvedTrack{}, err
	}

	cacheFile, err := s.cacheFilePath(entryURLFromID(entry.ID))
	if err != nil {
		return audio.ResolvedTrack{}, err
	}
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		return audio.ResolvedTrack{}, fmt.Errorf("create youtube cache dir: %w", err)
	}
	if _, statErr := os.Stat(cacheFile); statErr != nil {
		if !errors.Is(statErr, os.ErrNotExist) {
			return audio.ResolvedTrack{}, fmt.Errorf("stat youtube cache: %w", statErr)
		}
		if err := s.downloadAudio(ctx, entryURLFromID(entry.ID), cacheFile); err != nil {
			return audio.ResolvedTrack{}, err
		}
	}

	stream, format, err := s.decode(cacheFile)
	if err != nil {
		return audio.ResolvedTrack{}, err
	}

	info := teaui.TrackInfo{
		ID:       entry.ID,
		Title:    firstNonEmpty(entry.Artwork.Title, entry.Title),
		Artist:   firstNonEmpty(entry.Artwork.Artist, entry.Subtitle),
		Album:    entry.Artwork.Album,
		Source:   firstNonEmpty(entry.Source, sourceName),
		Duration: entry.Duration,
		Artwork:  entry.Artwork.Normalize(),
	}

	return audio.ResolvedTrack{
		Info:   info,
		Format: format,
		Stream: stream,
	}, nil
}

// OwnsEntryID reports whether the queue ID belongs to the YouTube source.
func OwnsEntryID(id string) bool {
	return strings.HasPrefix(id, entryIDPrefix)
}

func (s *Source) searchQuery(ctx context.Context, query string, filters teaui.SearchFilters) ([]teaui.SearchResult, error) {
	cmd := ytdlp.New().
		DefaultSearch(fmt.Sprintf("ytsearch%d", s.maxResults)).
		DumpJSON().
		SkipDownload().
		SetSeparateProcessGroup(true)
	s.applyAuth(cmd)

	output, err := s.run(ctx, cmd, query)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(strings.NewReader(output.Stdout))
	const maxJSONLine = 10 * 1024 * 1024
	scanner.Buffer(make([]byte, 0, 64*1024), maxJSONLine)

	results := make([]teaui.SearchResult, 0, s.maxResults)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry ytEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("parse yt-dlp search output: %w", err)
		}
		results = append(results, s.entryResults(entry, filters)...)
		if len(results) >= s.maxResults {
			return results[:s.maxResults], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan yt-dlp search output: %w", err)
	}
	return results, nil
}

func (s *Source) inspectURL(ctx context.Context, rawURL string, filters teaui.SearchFilters) ([]teaui.SearchResult, error) {
	cmd := ytdlp.New().
		DumpSingleJSON().
		SkipDownload().
		SetSeparateProcessGroup(true)
	s.applyAuth(cmd)

	output, err := s.run(ctx, cmd, rawURL)
	if err != nil {
		return nil, err
	}

	var entry ytEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(output.Stdout)), &entry); err != nil {
		return nil, fmt.Errorf("parse yt-dlp URL output: %w", err)
	}
	return s.entryResults(entry, filters), nil
}

func (s *Source) entryResults(entry ytEntry, filters teaui.SearchFilters) []teaui.SearchResult {
	if len(entry.Entries) > 0 {
		results := make([]teaui.SearchResult, 0, len(entry.Entries))
		for _, child := range entry.Entries {
			results = append(results, s.entryResults(child, filters)...)
			if len(results) >= s.maxResults {
				return results[:s.maxResults]
			}
		}
		return results
	}

	kind := mediaKindForEntry(entry)
	if kind == teaui.MediaPlaylist || !filters.Matches(kind) {
		return nil
	}

	resolvedURL := entryURL(entry)
	if resolvedURL == "" {
		return nil
	}

	title := firstNonEmpty(entry.Title, entry.ID, resolvedURL)
	artist := firstNonEmpty(entry.Artist, entry.Uploader, entry.Channel)
	metadata := coverart.Metadata{
		Title:  title,
		Album:  entry.Album,
		Artist: artist,
	}.Normalize()

	return []teaui.SearchResult{{
		ID:       entryIDPrefix + resolvedURL,
		Title:    title,
		Subtitle: firstNonEmpty(artist, entry.Album, sourceName),
		Source:   sourceName,
		Kind:     kind,
		Duration: durationFromSeconds(entry.Duration),
		Artwork:  metadata,
	}}
}

func mediaKindForEntry(entry ytEntry) teaui.MediaKind {
	switch {
	case len(entry.Entries) > 0 || entry.Type == "playlist" || entry.Type == "multi_video":
		return teaui.MediaPlaylist
	case entry.IsLive || strings.EqualFold(entry.LiveStatus, "is_live"):
		return teaui.MediaStream
	default:
		return teaui.MediaTrack
	}
}

func entryURL(entry ytEntry) string {
	for _, candidate := range []string{entry.WebpageURL, entry.OriginalURL, entry.URL} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.HasPrefix(candidate, "http://") || strings.HasPrefix(candidate, "https://") {
			return candidate
		}
	}
	if entry.ID == "" {
		return ""
	}
	return "https://music.youtube.com/watch?v=" + url.QueryEscape(entry.ID)
}

func entryURLFromID(id string) string {
	return strings.TrimSpace(strings.TrimPrefix(id, entryIDPrefix))
}

func (s *Source) downloadAudio(ctx context.Context, rawURL, destination string) error {
	cmd := ytdlp.New().
		NoPlaylist().
		Format("bestaudio/best").
		ExtractAudio().
		AudioFormat("mp3").
		AudioQuality("0").
		Output(destination).
		SetSeparateProcessGroup(true)
	s.applyAuth(cmd)

	if _, err := s.run(ctx, cmd, rawURL); err != nil {
		_ = os.Remove(destination)
		return err
	}
	return nil
}

func (s *Source) runCommand(ctx context.Context, command *ytdlp.Command, args ...string) (commandOutput, error) {
	cmd := command.BuildCommand(ctx, args...)
	cmd.Args = mergeExtraArgs(cmd.Args, len(args), s.extraArgs)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := commandOutput{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if err != nil {
		message := strings.TrimSpace(output.Stderr)
		if message == "" {
			message = err.Error()
		}
		return output, fmt.Errorf("yt-dlp failed: %s", message)
	}
	return output, nil
}

func mergeExtraArgs(commandArgs []string, positionalArgs int, extraArgs []string) []string {
	if len(extraArgs) == 0 {
		return commandArgs
	}
	if positionalArgs <= 0 || positionalArgs >= len(commandArgs) {
		return append(commandArgs, extraArgs...)
	}

	flagsEnd := len(commandArgs) - positionalArgs
	merged := make([]string, 0, len(commandArgs)+len(extraArgs))
	merged = append(merged, commandArgs[:flagsEnd]...)
	merged = append(merged, extraArgs...)
	merged = append(merged, commandArgs[flagsEnd:]...)
	return merged
}

func (s *Source) applyAuth(cmd *ytdlp.Command) {
	if s.cookiesFile != "" {
		cmd.Cookies(s.cookiesFile)
		return
	}
	if s.cookiesFromBrowser != "" {
		cmd.CookiesFromBrowser(s.cookiesFromBrowser)
	}
}

func (s *Source) ensureYTDLPInstalled(ctx context.Context) error {
	s.installOnce.Do(func() {
		_, s.installErr = ytdlp.Install(ctx, nil)
	})
	return s.installErr
}

func (s *Source) ensurePostProcessorsInstalled(ctx context.Context) error {
	s.postProcessOnce.Do(func() {
		if _, err := ytdlp.InstallFFmpeg(ctx, nil); err != nil {
			s.postProcessErr = err
			return
		}
		_, s.postProcessErr = ytdlp.InstallFFprobe(ctx, nil)
	})
	return s.postProcessErr
}

func (s *Source) cacheFilePath(rawURL string) (string, error) {
	if strings.TrimSpace(rawURL) == "" {
		return "", errors.New("youtube entry URL is empty")
	}
	cacheDir := strings.TrimSpace(s.cacheDir)
	if cacheDir == "" {
		root, err := os.UserCacheDir()
		if err != nil || strings.TrimSpace(root) == "" {
			root = os.TempDir()
		}
		cacheDir = filepath.Join(root, "musicon", "youtube")
	}
	sum := sha1.Sum([]byte(rawURL))
	return filepath.Join(cacheDir, hex.EncodeToString(sum[:])+".mp3"), nil
}

func decodeMP3File(path string) (beep.StreamSeekCloser, beep.Format, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, beep.Format{}, err
	}
	stream, format, err := mp3.Decode(file)
	if err != nil {
		_ = file.Close()
		return nil, beep.Format{}, err
	}
	return stream, format, nil
}

func normalizeMaxResults(value int) int {
	switch {
	case value <= 0:
		return defaultMaxResults
	case value > 50:
		return 50
	default:
		return value
	}
}

func normalizeExtraArgs(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}

func looksLikeURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func isYouTubeURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimPrefix(parsed.Hostname(), "www."))
	switch host {
	case "youtube.com", "music.youtube.com", "m.youtube.com", "youtu.be", "youtube-nocookie.com":
		return true
	default:
		return strings.HasSuffix(host, ".youtube.com")
	}
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

func durationFromSeconds(seconds float64) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

var _ teaui.SearchService = (*Source)(nil)
var _ audio.Resolver = (*Source)(nil)
