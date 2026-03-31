package lyrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultLRCLibBaseURL = "https://lrclib.net/api"

// LRCLibProvider resolves lyrics from lrclib.net.
type LRCLibProvider struct {
	Client  *http.Client
	BaseURL string
}

// Name returns the provider's stable identifier.
func (p *LRCLibProvider) Name() string { return "lrclib" }

// Lookup resolves lyrics from lrclib.net using exact and search fallbacks.
func (p *LRCLibProvider) Lookup(ctx context.Context, request Request) (Document, error) {
	request = request.Normalize()
	if request.Title == "" || request.Artist == "" {
		return Document{}, ErrNotFound
	}

	if exact, ok, err := p.lookupExact(ctx, request); err != nil {
		return Document{}, err
	} else if ok {
		return exact, nil
	}
	return p.lookupSearch(ctx, request)
}

func (p *LRCLibProvider) lookupExact(ctx context.Context, request Request) (Document, bool, error) {
	if request.Title == "" || request.Artist == "" || request.Album == "" || request.Duration <= 0 {
		return Document{}, false, nil
	}
	values := url.Values{}
	values.Set("track_name", request.Title)
	values.Set("artist_name", request.Artist)
	values.Set("album_name", request.Album)
	values.Set("duration", fmt.Sprintf("%d", int(request.Duration.Round(time.Second)/time.Second)))

	var payload lrclibResponse
	status, err := p.getJSON(ctx, "/get", values, &payload)
	switch {
	case err != nil:
		return Document{}, false, err
	case status == http.StatusNotFound:
		return Document{}, false, nil
	case status != http.StatusOK:
		return Document{}, false, fmt.Errorf("lrclib exact lookup returned %d", status)
	}
	document, ok := lrclibDocument(payload, request)
	if !ok {
		return Document{}, false, nil
	}
	return document, true, nil
}

func (p *LRCLibProvider) lookupSearch(ctx context.Context, request Request) (Document, error) {
	values := url.Values{}
	values.Set("track_name", request.Title)
	values.Set("artist_name", request.Artist)
	if request.Album != "" {
		values.Set("album_name", request.Album)
	}

	var payload []lrclibResponse
	status, err := p.getJSON(ctx, "/search", values, &payload)
	switch {
	case err != nil:
		return Document{}, err
	case status == http.StatusNotFound:
		return Document{}, ErrNotFound
	case status != http.StatusOK:
		return Document{}, fmt.Errorf("lrclib search returned %d", status)
	}

	best := Document{}
	bestScore := -1
	for _, candidate := range payload {
		document, ok := lrclibDocument(candidate, request)
		if !ok {
			continue
		}
		score := lrclibScore(candidate, request)
		if score > bestScore {
			bestScore = score
			best = document
		}
	}
	if bestScore < 0 {
		return Document{}, ErrNotFound
	}
	return best, nil
}

func (p *LRCLibProvider) getJSON(ctx context.Context, path string, values url.Values, target any) (int, error) {
	endpoint := strings.TrimRight(p.baseURL(), "/") + path
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := p.client().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return resp.StatusCode, nil
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return 0, err
	}
	return resp.StatusCode, nil
}

type lrclibResponse struct {
	ID           int     `json:"id"`
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

func lrclibDocument(payload lrclibResponse, request Request) (Document, bool) {
	if !strongTitleMatch(request.Title, payload.TrackName) || !strongArtistMatch(request.Artist, payload.ArtistName) {
		return Document{}, false
	}
	if request.Duration > 0 && !durationMatches(request.Duration, payload.Duration) {
		return Document{}, false
	}

	document := ParseLRC(payload.SyncedLyrics)
	document.Provider = "lrclib"
	document.Source = "lrclib"
	document.TrackName = firstNonEmpty(payload.TrackName, request.Title, document.TrackName)
	document.ArtistName = firstNonEmpty(payload.ArtistName, request.Artist, document.ArtistName)
	document.AlbumName = firstNonEmpty(payload.AlbumName, request.Album, document.AlbumName)
	document.Duration = time.Duration(payload.Duration * float64(time.Second))
	document.Instrumental = payload.Instrumental
	if strings.TrimSpace(payload.PlainLyrics) != "" {
		document.PlainLyrics = payload.PlainLyrics
	}
	if strings.TrimSpace(payload.SyncedLyrics) != "" {
		document.SyncedLyrics = payload.SyncedLyrics
	}
	if document.Empty() && !document.Instrumental {
		return Document{}, false
	}
	return document, true
}

func lrclibScore(payload lrclibResponse, request Request) int {
	score := 0
	if strings.TrimSpace(payload.SyncedLyrics) != "" {
		score += 10
	}
	if request.Album != "" && comparableText(request.Album) == comparableText(payload.AlbumName) {
		score += 3
	}
	if request.Duration > 0 {
		diff := request.Duration - time.Duration(payload.Duration*float64(time.Second))
		if diff < 0 {
			diff = -diff
		}
		if diff <= 2*time.Second {
			score += 5
		}
	}
	return score
}

func strongTitleMatch(request, candidate string) bool {
	return comparableText(request) != "" && comparableText(request) == comparableText(candidate)
}

func strongArtistMatch(request, candidate string) bool {
	req := comparableText(request)
	cand := comparableText(candidate)
	if req == "" || cand == "" {
		return false
	}
	if req == cand {
		return true
	}
	reqParts := splitArtists(request)
	candParts := splitArtists(candidate)
	for _, left := range reqParts {
		for _, right := range candParts {
			if left != "" && left == right {
				return true
			}
		}
	}
	return false
}

func splitArtists(value string) []string {
	replacer := strings.NewReplacer(" feat. ", ",", " feat ", ",", " featuring ", ",", " ft. ", ",", " ft ", ",", " & ", ",", ";", ",", " with ", ",")
	value = strings.ToLower(strings.TrimSpace(value))
	value = replacer.Replace(value)
	rawParts := strings.Split(value, ",")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		part = comparableText(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func comparableText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastSpace := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSpace = false
		default:
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func durationMatches(request time.Duration, candidateSeconds float64) bool {
	if request <= 0 || candidateSeconds <= 0 {
		return true
	}
	candidate := time.Duration(candidateSeconds * float64(time.Second))
	diff := request - candidate
	if diff < 0 {
		diff = -diff
	}
	return diff <= 2*time.Second
}

func (p *LRCLibProvider) client() *http.Client {
	if p != nil && p.Client != nil {
		return p.Client
	}
	return http.DefaultClient
}

func (p *LRCLibProvider) baseURL() string {
	if p != nil && strings.TrimSpace(p.BaseURL) != "" {
		return strings.TrimSpace(p.BaseURL)
	}
	return defaultLRCLibBaseURL
}
