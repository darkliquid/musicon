package radio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/darkliquid/musicon/internal/audio"
	teaui "github.com/darkliquid/musicon/internal/ui"
	"github.com/darkliquid/musicon/pkg/coverart"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/mp3"
	"github.com/gopxl/beep/vorbis"
	"github.com/gopxl/beep/wav"
)

const (
	sourceID              = "radio"
	sourceName            = "Radio"
	entryIDPrefix         = "radio:"
	defaultBaseURL        = "https://all.api.radio-browser.info"
	defaultUserAgent      = "musicon/0.1 (+https://github.com/darkliquid/musicon)"
	defaultMaxResults     = 20
	defaultSearchTimeout  = 15 * time.Second
	defaultResolveTimeout = 15 * time.Second
	maxSearchFetch        = 100
	searchSlackFactor     = 4
)

var errUnsupportedCodec = errors.New("radio stream codec is unsupported")

// Options configures the Radio Browser source.
type Options struct {
	Enabled    bool
	MaxResults int
	BaseURL    string
	Logf       func(string, ...interface{})
}

// Source searches Radio Browser for live stations and resolves them into playable streams.
type Source struct {
	enabled    bool
	maxResults int
	baseURL    string
	userAgent  string
	httpClient *http.Client
	logf       func(string, ...interface{})

	decodeMP3    func(io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error)
	decodeVorbis func(io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error)
	decodeWAV    func(io.Reader) (beep.StreamSeekCloser, beep.Format, error)
	openFallback func(context.Context, string) (beep.StreamSeekCloser, beep.Format, error)
}

type station struct {
	StationUUID string    `json:"stationuuid"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	URLResolved string    `json:"url_resolved"`
	Homepage    string    `json:"homepage"`
	Favicon     string    `json:"favicon"`
	Tags        string    `json:"tags"`
	Country     string    `json:"country"`
	CountryCode string    `json:"countrycode"`
	Language    string    `json:"language"`
	Codec       string    `json:"codec"`
	Bitrate     int       `json:"bitrate"`
	Votes       int       `json:"votes"`
	HLS         radioBool `json:"hls"`
	LastCheckOK radioBool `json:"lastcheckok"`
}

type stationClick struct {
	OK          radioBool `json:"ok"`
	Message     string    `json:"message"`
	StationUUID string    `json:"stationuuid"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
}

type radioBool bool

func (b *radioBool) UnmarshalJSON(data []byte) error {
	value := strings.TrimSpace(strings.ToLower(string(data)))
	switch value {
	case "1", "true", `"true"`:
		*b = true
	case "0", "false", `"false"`, "", "null":
		*b = false
	default:
		return fmt.Errorf("unsupported radio-browser boolean %q", value)
	}
	return nil
}

// NewSource constructs a Radio Browser-backed source.
func NewSource(options Options) *Source {
	source := &Source{
		enabled:      options.Enabled,
		maxResults:   normalizeMaxResults(options.MaxResults),
		baseURL:      normalizeBaseURL(options.BaseURL),
		userAgent:    defaultUserAgent,
		httpClient:   http.DefaultClient,
		logf:         options.Logf,
		decodeMP3:    mp3.Decode,
		decodeVorbis: vorbis.Decode,
		decodeWAV:    wav.Decode,
	}
	source.openFallback = source.openNativeStream
	return source
}

// Sources reports the internet-radio source descriptor exposed to the UI.
func (s *Source) Sources() []teaui.SourceDescriptor {
	if s == nil || !s.enabled {
		return nil
	}
	return []teaui.SourceDescriptor{{
		ID:          sourceID,
		Name:        sourceName,
		Description: "Search Radio Browser for live stations, including HLS-backed streams through native Go playback.",
		SearchModes: []teaui.SearchModeDescriptor{
			{ID: teaui.SearchModeAll, Name: teaui.SearchModeAll.String()},
			{ID: teaui.SearchModeStreams, Name: teaui.SearchModeStreams.String()},
		},
		DefaultMode: teaui.SearchModeStreams,
	}}
}

// Search finds healthy stations by name and tag, leaving playback-path selection to Resolve.
func (s *Source) Search(ctx context.Context, request teaui.SearchRequest) ([]teaui.SearchResult, error) {
	if s == nil || !s.enabled {
		return nil, nil
	}
	if request.SourceID != "" && request.SourceID != "all" && request.SourceID != sourceID {
		return nil, nil
	}
	if !request.Filters.Streams || !searchModeMatches(request.Mode) {
		return nil, nil
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, defaultSearchTimeout)
	defer cancel()

	fetchLimit := min(maxSearchFetch, max(s.maxResults*searchSlackFactor, s.maxResults))
	results := make([]teaui.SearchResult, 0, s.maxResults)
	seen := make(map[string]struct{}, fetchLimit)
	for _, endpoint := range s.searchEndpoints(query, fetchLimit) {
		stations, err := s.fetchStations(ctx, endpoint)
		if err != nil {
			return nil, err
		}
		for _, station := range stations {
			result, ok := stationResult(station)
			if !ok {
				continue
			}
			if _, exists := seen[result.ID]; exists {
				continue
			}
			seen[result.ID] = struct{}{}
			results = append(results, result)
			if len(results) >= s.maxResults {
				return results, nil
			}
		}
	}
	return results, nil
}

// ExpandCollection reports no child rows because radio stations are already playable streams.
func (s *Source) ExpandCollection(context.Context, teaui.SearchResult) ([]teaui.SearchResult, error) {
	return nil, nil
}

// Resolve turns a queued station entry into a playable live stream.
func (s *Source) Resolve(entry teaui.QueueEntry) (audio.ResolvedTrack, error) {
	if s == nil || !s.enabled {
		return audio.ResolvedTrack{}, errors.New("radio source is disabled")
	}
	uuid, codecHint, mode, ok := parseEntryID(entry.ID)
	if !ok {
		return audio.ResolvedTrack{}, fmt.Errorf("radio source cannot resolve %q", entry.ID)
	}

	clickCtx, cancel := context.WithTimeout(context.Background(), defaultResolveTimeout)
	defer cancel()

	streamURL, stationName, err := s.stationStreamURL(clickCtx, uuid)
	if err != nil {
		return audio.ResolvedTrack{}, err
	}

	openCtx, openCancel := context.WithCancel(context.Background())

	stream, format, err := s.openResolvedStream(openCtx, streamURL, codecHint, mode)
	if err != nil {
		openCancel()
		return audio.ResolvedTrack{}, err
	}

	info := teaui.TrackInfo{
		ID:       entry.ID,
		Title:    firstNonEmpty(stationName, entry.Title),
		Artist:   entry.Subtitle,
		Source:   firstNonEmpty(entry.Source, sourceName),
		Duration: 0,
		Artwork:  entry.Artwork.Normalize(),
	}
	return audio.ResolvedTrack{
		Info:   info,
		Format: format,
		Stream: &liveStream{next: stream, cancel: openCancel},
	}, nil
}

// OwnsEntryID reports whether a queue entry belongs to this provider.
func OwnsEntryID(id string) bool {
	return strings.HasPrefix(id, entryIDPrefix)
}

func (s *Source) searchEndpoints(query string, limit int) []string {
	escaped := url.PathEscape(query)
	params := url.Values{}
	params.Set("hidebroken", "true")
	params.Set("order", "votes")
	params.Set("reverse", "true")
	params.Set("limit", strconv.Itoa(limit))
	return []string{
		s.baseURL + "/json/stations/byname/" + escaped + "?" + params.Encode(),
		s.baseURL + "/json/stations/bytag/" + escaped + "?" + params.Encode(),
	}
}

func (s *Source) fetchStations(ctx context.Context, endpoint string) ([]station, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build radio-browser search request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("radio-browser search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("radio-browser search failed: %s", strings.TrimSpace(firstNonEmpty(string(message), resp.Status)))
	}

	var payload []station
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode radio-browser search response: %w", err)
	}
	return payload, nil
}

func (s *Source) stationStreamURL(ctx context.Context, stationUUID string) (streamURL string, stationName string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/json/url/"+url.PathEscape(stationUUID), nil)
	if err != nil {
		return "", "", fmt.Errorf("build radio-browser click request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("radio-browser click request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", "", fmt.Errorf("radio-browser click request failed: %s", strings.TrimSpace(firstNonEmpty(string(message), resp.Status)))
	}

	var payload stationClick
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", fmt.Errorf("decode radio-browser click response: %w", err)
	}
	streamURL = strings.TrimSpace(payload.URL)
	if streamURL == "" {
		return "", "", errors.New("radio-browser did not return a station stream URL")
	}
	return streamURL, strings.TrimSpace(payload.Name), nil
}

func (s *Source) decodeResponse(resp *http.Response, codecHint string) (beep.StreamSeekCloser, beep.Format, error) {
	kind := decoderKind(codecHint, resp.Header.Get("Content-Type"), resp.Request.URL.String())
	switch kind {
	case "mp3":
		stream, format, err := s.decodeMP3(resp.Body)
		if err != nil {
			return nil, beep.Format{}, fmt.Errorf("decode mp3 radio stream: %w", err)
		}
		return stream, format, nil
	case "vorbis":
		stream, format, err := s.decodeVorbis(resp.Body)
		if err != nil {
			return nil, beep.Format{}, fmt.Errorf("decode vorbis radio stream: %w", err)
		}
		return stream, format, nil
	case "wav":
		stream, format, err := s.decodeWAV(resp.Body)
		if err != nil {
			return nil, beep.Format{}, fmt.Errorf("decode wav radio stream: %w", err)
		}
		return stream, format, nil
	default:
		return nil, beep.Format{}, errUnsupportedCodec
	}
}

func stationResult(station station) (teaui.SearchResult, bool) {
	if !station.LastCheckOK || strings.TrimSpace(station.StationUUID) == "" {
		return teaui.SearchResult{}, false
	}
	kind := codecToken(station.Codec, firstNonEmpty(station.URLResolved, station.URL))
	mode := playbackMode(station, kind)

	title := normalizeStationName(station.Name, station.StationUUID)
	subtitle := describeStation(station, kind, mode)
	artwork := coverart.Metadata{
		Title:     title,
		RemoteURL: strings.TrimSpace(station.Favicon),
	}.Normalize()

	return teaui.SearchResult{
		ID:        formatEntryID(station.StationUUID, kind, mode),
		Title:     title,
		Subtitle:  subtitle,
		Source:    sourceName,
		Kind:      teaui.MediaStream,
		Artwork:   artwork,
		QueueHint: strings.TrimSpace(station.Homepage),
	}, true
}

func describeStation(station station, kind, mode string) string {
	parts := make([]string, 0, 4)
	if country := strings.TrimSpace(firstNonEmpty(station.CountryCode, station.Country)); country != "" {
		parts = append(parts, country)
	}
	if language := strings.TrimSpace(station.Language); language != "" {
		parts = append(parts, language)
	}
	codec := strings.ToUpper(kind)
	if codec == "" {
		codec = "UNKNOWN"
	}
	if station.Bitrate > 0 {
		codec = fmt.Sprintf("%s %dk", codec, station.Bitrate)
	}
	if mode == playbackModeFallback {
		codec += " via native stream"
	}
	parts = append(parts, codec)
	return strings.Join(parts, " · ")
}

func normalizeStationName(name, fallback string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return fallback
	}
	return strings.Join(strings.Fields(name), " ")
}

func searchModeMatches(mode teaui.SearchMode) bool {
	switch mode {
	case teaui.SearchModeDefault, teaui.SearchModeAll, teaui.SearchModeStreams:
		return true
	default:
		return false
	}
}

func decoderKind(codec, contentType, rawURL string) string {
	joined := strings.ToLower(strings.Join([]string{codec, contentType, path.Ext(rawURL)}, " "))
	switch {
	case strings.Contains(joined, ".m3u8"), strings.Contains(joined, "mpegurl"), strings.Contains(joined, "application/x-mpegurl"), strings.Contains(joined, "application/vnd.apple.mpegurl"):
		return ""
	case strings.Contains(joined, "mp3"), strings.Contains(joined, "mpeg"):
		return "mp3"
	case strings.Contains(joined, "vorbis"), strings.Contains(joined, ".ogg"):
		return "vorbis"
	case strings.Contains(joined, "wav"), strings.Contains(joined, "wave"):
		return "wav"
	default:
		return ""
	}
}

func codecToken(codec, rawURL string) string {
	if kind := decoderKind(codec, "", rawURL); kind != "" {
		return kind
	}
	codec = strings.TrimSpace(strings.ToLower(codec))
	if codec != "" {
		return codec
	}
	if ext := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(path.Ext(rawURL))), "."); ext != "" {
		return ext
	}
	return "unknown"
}

const (
	playbackModeDirect   = "direct"
	playbackModeFallback = "fallback"
)

func playbackMode(station station, codec string) string {
	if station.HLS || decoderKind(codec, "", "") == "" {
		return playbackModeFallback
	}
	return playbackModeDirect
}

func normalizeMaxResults(value int) int {
	if value <= 0 {
		return defaultMaxResults
	}
	return value
}

func normalizeBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = defaultBaseURL
	}
	return strings.TrimRight(raw, "/")
}

func formatEntryID(stationUUID, codec, mode string) string {
	return entryIDPrefix + strings.TrimSpace(stationUUID) + ":" + strings.TrimSpace(codec) + ":" + strings.TrimSpace(mode)
}

func parseEntryID(id string) (stationUUID string, codec string, mode string, ok bool) {
	if !OwnsEntryID(id) {
		return "", "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(id, entryIDPrefix), ":")
	switch len(parts) {
	case 2:
		if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return "", "", "", false
		}
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), playbackModeDirect, true
	case 3:
		if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[2]) == "" {
			return "", "", "", false
		}
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2]), true
	default:
		return "", "", "", false
	}
}

func (s *Source) openResolvedStream(ctx context.Context, streamURL, codecHint, mode string) (beep.StreamSeekCloser, beep.Format, error) {
	if mode == playbackModeFallback {
		return s.openFallbackStream(ctx, streamURL)
	}

	resp, err := s.openStreamRequest(ctx, streamURL)
	if err != nil {
		return nil, beep.Format{}, err
	}
	stream, format, decodeErr := s.decodeResponse(resp, codecHint)
	if decodeErr == nil {
		return stream, format, nil
	}
	_ = resp.Body.Close()

	fallbackStream, fallbackFormat, fallbackErr := s.openFallbackStream(ctx, streamURL)
	if fallbackErr == nil {
		return fallbackStream, fallbackFormat, nil
	}
	if errors.Is(decodeErr, errUnsupportedCodec) {
		return nil, beep.Format{}, fmt.Errorf("%w: %s; native stream open failed: %v", errUnsupportedCodec, codecHint, fallbackErr)
	}
	return nil, beep.Format{}, fmt.Errorf("decode radio stream: %v; native stream open failed: %w", decodeErr, fallbackErr)
}

func (s *Source) openStreamRequest(ctx context.Context, streamURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build radio stream request: %w", err)
	}
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open radio stream: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("open radio stream: %s", strings.TrimSpace(firstNonEmpty(string(message), resp.Status)))
	}
	return resp, nil
}

func (s *Source) openFallbackStream(ctx context.Context, streamURL string) (beep.StreamSeekCloser, beep.Format, error) {
	if s == nil || s.openFallback == nil {
		return nil, beep.Format{}, fmt.Errorf("%w: no native stream playback path configured", errUnsupportedCodec)
	}
	stream, format, err := s.openFallback(ctx, streamURL)
	if err != nil {
		return nil, beep.Format{}, err
	}
	return stream, format, nil
}

func (s *Source) debugf(format string, args ...interface{}) {
	if s == nil || s.logf == nil {
		return
	}
	s.logf(format, args...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

type liveStream struct {
	next   beep.StreamSeekCloser
	cancel context.CancelFunc
	pos    int
}

func (s *liveStream) Stream(samples [][2]float64) (int, bool) {
	n, ok := s.next.Stream(samples)
	s.pos += n
	return n, ok
}

func (s *liveStream) Err() error { return s.next.Err() }

func (s *liveStream) Len() int { return 0 }

func (s *liveStream) Position() int { return s.pos }

func (s *liveStream) Seek(int) error {
	return errors.New("radio streams do not support seeking")
}

func (s *liveStream) Close() error {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	return s.next.Close()
}

var _ teaui.SearchService = (*Source)(nil)
var _ audio.Resolver = (*Source)(nil)
