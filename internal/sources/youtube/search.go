package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	teaui "github.com/darkliquid/musicon/internal/ui"
	"github.com/darkliquid/musicon/pkg/coverart"
	youtubev2 "github.com/kkdai/youtube/v2"
)

// The search-side types in this file mirror just enough of the YouTube Music
// JSON shape to extract playable results. The API response is deeply nested and
// unstable, so the goal here is to model only the fields Musicon actually uses.
type musicSearchRequest struct {
	Context struct {
		Client struct {
			ClientName    string `json:"clientName"`
			ClientVersion string `json:"clientVersion"`
		} `json:"client"`
	} `json:"context"`
	Query string `json:"query"`
}

type musicSearchResponse struct {
	Contents struct {
		TabbedSearchResultsRenderer struct {
			Tabs []musicSearchTab `json:"tabs"`
		} `json:"tabbedSearchResultsRenderer"`
	} `json:"contents"`
}

type musicSearchTab struct {
	TabRenderer struct {
		Title   string `json:"title"`
		Content struct {
			SectionListRenderer struct {
				Contents []musicSearchSection `json:"contents"`
			} `json:"sectionListRenderer"`
		} `json:"content"`
	} `json:"tabRenderer"`
}

type musicSearchSection struct {
	MusicCardShelfRenderer struct {
		Title    musicRuns     `json:"title"`
		Subtitle musicRuns     `json:"subtitle"`
		OnTap    musicWatchTap `json:"onTap"`
	} `json:"musicCardShelfRenderer"`
	MusicShelfRenderer struct {
		Contents []musicSearchResult `json:"contents"`
	} `json:"musicShelfRenderer"`
}

type musicSearchResult struct {
	MusicResponsiveListItemRenderer struct {
		PlaylistItemData struct {
			VideoID string `json:"videoId"`
		} `json:"playlistItemData"`
		FlexColumns []struct {
			MusicResponsiveListItemFlexColumnRenderer struct {
				Text musicRuns `json:"text"`
			} `json:"musicResponsiveListItemFlexColumnRenderer"`
		} `json:"flexColumns"`
	} `json:"musicResponsiveListItemRenderer"`
}

type musicWatchTap struct {
	WatchEndpoint struct {
		VideoID string `json:"videoId"`
	} `json:"watchEndpoint"`
}

type musicRuns struct {
	Runs []struct {
		Text               string `json:"text"`
		NavigationEndpoint struct {
			BrowseEndpoint struct {
				BrowseID                              string `json:"browseId"`
				BrowseEndpointContextSupportedConfigs struct {
					BrowseEndpointContextMusicConfig struct {
						PageType string `json:"pageType"`
					} `json:"browseEndpointContextMusicConfig"`
				} `json:"browseEndpointContextSupportedConfigs"`
			} `json:"browseEndpoint"`
		} `json:"navigationEndpoint"`
	} `json:"runs"`
}

// OwnsEntryID reports whether a queued entry ID belongs to this provider.
//
// Queue IDs are namespaced so Resolve can reject entries that came from other
// sources without peeking at UI-only metadata.
func OwnsEntryID(id string) bool {
	return strings.HasPrefix(id, entryIDPrefix)
}

// searchQuery performs the raw YouTube Music HTTP search.
//
// Unlike playback, search does not use yt-dlp. That keeps queue interactions
// responsive and avoids starting a subprocess on every keystroke.
func (s *Source) searchQuery(ctx context.Context, query string, filters teaui.SearchFilters) ([]teaui.SearchResult, error) {
	requestBody := musicSearchRequest{Query: query}
	requestBody.Context.Client.ClientName = musicClientName
	requestBody.Context.Client.ClientVersion = musicClientVersion

	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("encode youtube music search request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.searchEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build youtube music search request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://music.youtube.com")
	req.Header.Set("Referer", "https://music.youtube.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("youtube music search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("youtube music search failed: %s", strings.TrimSpace(firstNonEmpty(string(message), resp.Status)))
	}

	var payload musicSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode youtube music search response: %w", err)
	}
	return musicSearchResults(payload, filters, s.maxResults), nil
}

// inspectURL handles pasted YouTube / YouTube Music URLs.
//
// Playlist URLs expand into multiple queueable results, while single video URLs
// become one queueable result.
func (s *Source) inspectURL(ctx context.Context, rawURL string, filters teaui.SearchFilters) ([]teaui.SearchResult, error) {
	if looksLikePlaylistURL(rawURL) {
		playlist, err := s.yt.GetPlaylistContext(ctx, rawURL)
		if err != nil {
			return nil, fmt.Errorf("inspect youtube playlist: %w", err)
		}
		return playlistResults(playlist, filters, s.maxResults), nil
	}

	video, err := s.yt.GetVideoContext(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("inspect youtube url: %w", err)
	}
	result := videoResult(video)
	if result.ID == "" || !filters.Matches(result.Kind) {
		return nil, nil
	}
	return []teaui.SearchResult{result}, nil
}

// videoResult maps youtube/v2 metadata onto Musicon's source-agnostic search
// result type.
func videoResult(video *youtubev2.Video) teaui.SearchResult {
	if video == nil || strings.TrimSpace(video.ID) == "" {
		return teaui.SearchResult{}
	}
	kind := teaui.MediaTrack
	if video.Duration <= 0 && strings.TrimSpace(video.HLSManifestURL) != "" {
		kind = teaui.MediaStream
	}
	metadata := coverart.Metadata{Title: strings.TrimSpace(video.Title), Artist: strings.TrimSpace(video.Author)}.Normalize()
	return teaui.SearchResult{
		ID:       entryIDPrefix + videoURL(video.ID),
		Title:    firstNonEmpty(video.Title, video.ID),
		Subtitle: firstNonEmpty(video.Author, sourceName),
		Source:   sourceName,
		Kind:     kind,
		Duration: video.Duration,
		Artwork:  metadata,
	}
}

// playlistResults flattens a playlist into track-like search results so the UI
// does not need to understand YouTube playlist expansion.
func playlistResults(playlist *youtubev2.Playlist, filters teaui.SearchFilters, maxResults int) []teaui.SearchResult {
	if playlist == nil || !filters.Matches(teaui.MediaTrack) {
		return nil
	}
	results := make([]teaui.SearchResult, 0, min(len(playlist.Videos), maxResults))
	for _, entry := range playlist.Videos {
		if entry == nil || strings.TrimSpace(entry.ID) == "" {
			continue
		}
		metadata := coverart.Metadata{Title: strings.TrimSpace(entry.Title), Artist: strings.TrimSpace(entry.Author), Album: strings.TrimSpace(playlist.Title)}.Normalize()
		results = append(results, teaui.SearchResult{
			ID:       entryIDPrefix + videoURL(entry.ID),
			Title:    firstNonEmpty(entry.Title, entry.ID),
			Subtitle: firstNonEmpty(entry.Author, playlist.Title, sourceName),
			Source:   sourceName,
			Kind:     teaui.MediaTrack,
			Duration: entry.Duration,
			Artwork:  metadata,
		})
		if len(results) >= maxResults {
			return results[:maxResults]
		}
	}
	return results
}

// musicSearchResults prefers shelf items first and falls back to top-card
// results only when shelves did not yield playable tracks.
//
// This matches the package goal of producing queueable items, not faithfully
// reproducing every visual grouping in the upstream API.
func musicSearchResults(response musicSearchResponse, filters teaui.SearchFilters, maxResults int) []teaui.SearchResult {
	contents := musicSearchContents(response)
	results := make([]teaui.SearchResult, 0, maxResults)
	seen := make(map[string]struct{})

	appendUnique := func(result teaui.SearchResult) {
		if result.ID == "" {
			return
		}
		if _, exists := seen[result.ID]; exists {
			return
		}
		seen[result.ID] = struct{}{}
		results = append(results, result)
	}

	for _, section := range contents {
		for _, item := range section.MusicShelfRenderer.Contents {
			result := item.ToSearchResult()
			if result.ID == "" || !filters.Matches(result.Kind) {
				continue
			}
			appendUnique(result)
			if len(results) >= maxResults {
				return results
			}
		}
	}

	if len(results) > 0 {
		return results
	}

	for _, section := range contents {
		result := section.TopResult()
		if result.ID == "" || !filters.Matches(result.Kind) {
			continue
		}
		appendUnique(result)
		if len(results) >= maxResults {
			return results
		}
	}

	return results
}

// musicSearchContents picks the "YT Music" tab from the multi-tab search
// response. Other tabs can contain generic YouTube results that are less useful
// for the music-first queue workflow.
func musicSearchContents(response musicSearchResponse) []musicSearchSection {
	for _, tab := range response.Contents.TabbedSearchResultsRenderer.Tabs {
		if strings.EqualFold(strings.TrimSpace(tab.TabRenderer.Title), "YT Music") {
			return tab.TabRenderer.Content.SectionListRenderer.Contents
		}
	}
	return nil
}

// TopResult extracts the card-style top hit when the shelf results did not
// produce anything playable.
func (s musicSearchSection) TopResult() teaui.SearchResult {
	videoID := strings.TrimSpace(s.MusicCardShelfRenderer.OnTap.WatchEndpoint.VideoID)
	if videoID == "" {
		return teaui.SearchResult{}
	}
	title := firstNonEmpty(s.MusicCardShelfRenderer.Title.FirstText(), videoID)
	artist := s.MusicCardShelfRenderer.Subtitle.ArtistsText()
	album := s.MusicCardShelfRenderer.Subtitle.AlbumText()
	return newMusicSearchResult(videoID, title, artist, album, s.MusicCardShelfRenderer.Subtitle.Duration())
}

// ToSearchResult extracts one shelf row into Musicon's normalized result type.
func (r musicSearchResult) ToSearchResult() teaui.SearchResult {
	videoID := strings.TrimSpace(r.MusicResponsiveListItemRenderer.PlaylistItemData.VideoID)
	if videoID == "" {
		return teaui.SearchResult{}
	}
	title := ""
	if len(r.MusicResponsiveListItemRenderer.FlexColumns) > 0 {
		title = r.MusicResponsiveListItemRenderer.FlexColumns[0].MusicResponsiveListItemFlexColumnRenderer.Text.FirstText()
	}
	metaRuns := musicRuns{}
	if len(r.MusicResponsiveListItemRenderer.FlexColumns) > 1 {
		metaRuns = r.MusicResponsiveListItemRenderer.FlexColumns[1].MusicResponsiveListItemFlexColumnRenderer.Text
	}
	return newMusicSearchResult(videoID, firstNonEmpty(title, videoID), metaRuns.ArtistsText(), metaRuns.AlbumText(), metaRuns.Duration())
}

// newMusicSearchResult centralizes the final YouTube-search-to-Musicon mapping
// so card and shelf paths stay consistent.
func newMusicSearchResult(videoID, title, artist, album string, duration time.Duration) teaui.SearchResult {
	metadata := coverart.Metadata{Title: title, Artist: artist, Album: album}.Normalize()
	return teaui.SearchResult{ID: entryIDPrefix + videoURL(videoID), Title: title, Subtitle: firstNonEmpty(artist, album, sourceName), Source: sourceName, Kind: teaui.MediaTrack, Duration: duration, Artwork: metadata}
}

// The musicRuns helpers interpret YouTube Music's loosely typed text runs into
// the specific pieces Musicon cares about.
func (r musicRuns) FirstText() string {
	for _, run := range r.Runs {
		if text := strings.TrimSpace(run.Text); text != "" {
			return text
		}
	}
	return ""
}

func (r musicRuns) ArtistsText() string {
	names := make([]string, 0, len(r.Runs))
	seen := make(map[string]struct{})
	for _, run := range r.Runs {
		pageType := run.NavigationEndpoint.BrowseEndpoint.BrowseEndpointContextSupportedConfigs.BrowseEndpointContextMusicConfig.PageType
		if pageType != "MUSIC_PAGE_TYPE_ARTIST" && pageType != "MUSIC_PAGE_TYPE_USER_CHANNEL" {
			continue
		}
		name := strings.TrimSpace(run.Text)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

func (r musicRuns) AlbumText() string {
	for _, run := range r.Runs {
		pageType := run.NavigationEndpoint.BrowseEndpoint.BrowseEndpointContextSupportedConfigs.BrowseEndpointContextMusicConfig.PageType
		if pageType == "MUSIC_PAGE_TYPE_ALBUM" {
			return strings.TrimSpace(run.Text)
		}
	}
	return ""
}

func (r musicRuns) Duration() time.Duration {
	for _, run := range r.Runs {
		if run.NavigationEndpoint.BrowseEndpoint.BrowseID != "" {
			continue
		}
		text := strings.TrimSpace(run.Text)
		if !strings.Contains(text, ":") {
			continue
		}
		duration, ok := parseClockDuration(text)
		if ok {
			return duration
		}
	}
	return 0
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

func looksLikePlaylistURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if strings.TrimSpace(parsed.Query().Get("list")) != "" {
		return true
	}
	return strings.Contains(parsed.Path, "/playlist")
}

func videoURL(id string) string {
	return "https://music.youtube.com/watch?v=" + url.QueryEscape(strings.TrimSpace(id))
}

func entryURLFromID(id string) string {
	return strings.TrimSpace(strings.TrimPrefix(id, entryIDPrefix))
}

func videoTitle(video *youtubev2.Video) string {
	if video == nil {
		return ""
	}
	return video.Title
}

func videoAuthor(video *youtubev2.Video) string {
	if video == nil {
		return ""
	}
	return video.Author
}

func videoDuration(video *youtubev2.Video) time.Duration {
	if video == nil {
		return 0
	}
	return video.Duration
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

func firstDuration(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func parseClockDuration(value string) (time.Duration, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	total := 0
	for _, part := range parts {
		if part == "" {
			return 0, false
		}
		n := 0
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return 0, false
			}
			n = n*10 + int(ch-'0')
		}
		total = total*60 + n
	}
	return time.Duration(total) * time.Second, true
}
