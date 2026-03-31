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

const musicBrowseEndpoint = "https://music.youtube.com/youtubei/v1/browse?prettyPrint=false"

// The search-side types in this file mirror just enough of the YouTube Music
// JSON shape to extract focused rows for songs, artists, albums, and playlists.
// The API response is deeply nested and unstable, so the goal here is to model
// only the fields Musicon actually uses.
type musicSearchRequest struct {
	Context struct {
		Client struct {
			ClientName    string `json:"clientName"`
			ClientVersion string `json:"clientVersion"`
		} `json:"client"`
	} `json:"context"`
	Query  string `json:"query"`
	Params string `json:"params,omitempty"`
}

type musicBrowseRequest struct {
	Context struct {
		Client struct {
			ClientName    string `json:"clientName"`
			ClientVersion string `json:"clientVersion"`
		} `json:"client"`
	} `json:"context"`
	BrowseID string `json:"browseId"`
}

type musicSearchResponse struct {
	Contents struct {
		TabbedSearchResultsRenderer struct {
			Tabs []musicSearchTab `json:"tabs"`
		} `json:"tabbedSearchResultsRenderer"`
		SectionListRenderer struct {
			Contents []musicSearchSection `json:"contents"`
		} `json:"sectionListRenderer"`
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
		Title     musicRuns          `json:"title"`
		Subtitle  musicRuns          `json:"subtitle"`
		OnTap     musicWatchTap      `json:"onTap"`
		Thumbnail musicThumbnailNode `json:"thumbnail"`
	} `json:"musicCardShelfRenderer"`
	MusicShelfRenderer struct {
		Title    musicRuns           `json:"title"`
		Contents []musicSearchResult `json:"contents"`
	} `json:"musicShelfRenderer"`
}

type musicSearchResult struct {
	MusicResponsiveListItemRenderer struct {
		PlaylistItemData struct {
			VideoID string `json:"videoId"`
		} `json:"playlistItemData"`
		NavigationEndpoint musicNavigationEndpoint `json:"navigationEndpoint"`
		Thumbnail          musicThumbnailNode      `json:"thumbnail"`
		FlexColumns        []struct {
			MusicResponsiveListItemFlexColumnRenderer struct {
				Text musicRuns `json:"text"`
			} `json:"musicResponsiveListItemFlexColumnRenderer"`
		} `json:"flexColumns"`
	} `json:"musicResponsiveListItemRenderer"`
}

type musicNavigationEndpoint struct {
	BrowseEndpoint struct {
		BrowseID                              string `json:"browseId"`
		BrowseEndpointContextSupportedConfigs struct {
			BrowseEndpointContextMusicConfig struct {
				PageType string `json:"pageType"`
			} `json:"browseEndpointContextMusicConfig"`
		} `json:"browseEndpointContextSupportedConfigs"`
	} `json:"browseEndpoint"`
	WatchEndpoint struct {
		VideoID    string `json:"videoId"`
		PlaylistID string `json:"playlistId"`
	} `json:"watchEndpoint"`
}

type musicWatchTap struct {
	WatchEndpoint struct {
		VideoID string `json:"videoId"`
	} `json:"watchEndpoint"`
}

type musicRuns struct {
	Runs []struct {
		Text               string                  `json:"text"`
		NavigationEndpoint musicNavigationEndpoint `json:"navigationEndpoint"`
	} `json:"runs"`
}

type musicThumbnailNode struct {
	MusicThumbnailRenderer struct {
		Thumbnail musicThumbnailList `json:"thumbnail"`
	} `json:"musicThumbnailRenderer"`
	CroppedSquareThumbnailRenderer struct {
		Thumbnail musicThumbnailList `json:"thumbnail"`
	} `json:"croppedSquareThumbnailRenderer"`
	Thumbnail  musicThumbnailList `json:"thumbnail"`
	Thumbnails []musicThumbnail   `json:"thumbnails"`
}

type musicThumbnailList struct {
	Thumbnails []musicThumbnail `json:"thumbnails"`
}

type musicThumbnail struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
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
func (s *Source) searchQuery(ctx context.Context, query string, request teaui.SearchRequest) ([]teaui.SearchResult, error) {
	requestBody := musicSearchRequest{Query: query}
	requestBody.Context.Client.ClientName = musicClientName
	requestBody.Context.Client.ClientVersion = musicClientVersion
	requestBody.Params = searchParamsForMode(request.Mode)

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
	return musicSearchResults(payload, request, s.maxResults), nil
}

// inspectURL handles pasted YouTube / YouTube Music URLs.
//
// Playlist URLs now become collection rows so the queue UI can either enqueue the
// whole collection or expand it into child songs on demand.
func (s *Source) inspectURL(ctx context.Context, rawURL string, request teaui.SearchRequest) ([]teaui.SearchResult, error) {
	if looksLikePlaylistURL(rawURL) {
		playlist, err := s.yt.GetPlaylistContext(ctx, rawURL)
		if err != nil {
			return nil, fmt.Errorf("inspect youtube playlist: %w", err)
		}
		result := playlistCollectionResult(playlist, rawURL)
		if result.ID == "" {
			return nil, nil
		}
		if request.Mode == teaui.SearchModeArtists {
			return nil, nil
		}
		return []teaui.SearchResult{result}, nil
	}

	video, err := s.yt.GetVideoContext(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("inspect youtube url: %w", err)
	}
	result := videoResult(video)
	if result.ID == "" {
		return nil, nil
	}
	if request.Mode == teaui.SearchModeArtists || request.Mode == teaui.SearchModeAlbums || request.Mode == teaui.SearchModePlaylists {
		return nil, nil
	}
	return []teaui.SearchResult{result}, nil
}

func (s *Source) expandCollection(ctx context.Context, result teaui.SearchResult) ([]teaui.SearchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultSearchTimeout)
	defer cancel()

	switch {
	case strings.TrimSpace(result.PlaylistID) != "":
		playlist, err := s.yt.GetPlaylistContext(ctx, playlistURL(result.PlaylistID))
		if err != nil {
			return nil, fmt.Errorf("inspect youtube playlist: %w", err)
		}
		return playlistEntries(playlist, result), nil
	case strings.TrimSpace(result.BrowseID) != "":
		return s.browseCollection(ctx, result)
	default:
		return nil, nil
	}
}

func (s *Source) browseCollection(ctx context.Context, result teaui.SearchResult) ([]teaui.SearchResult, error) {
	requestBody := musicBrowseRequest{BrowseID: strings.TrimSpace(result.BrowseID)}
	requestBody.Context.Client.ClientName = musicClientName
	requestBody.Context.Client.ClientVersion = musicClientVersion

	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("encode youtube music browse request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, musicBrowseEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build youtube music browse request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://music.youtube.com")
	req.Header.Set("Referer", "https://music.youtube.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("youtube music browse request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("youtube music browse failed: %s", strings.TrimSpace(firstNonEmpty(string(message), resp.Status)))
	}

	var payload any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode youtube music browse response: %w", err)
	}
	return browseCollectionTracks(payload, result), nil
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

func playlistCollectionResult(playlist *youtubev2.Playlist, rawURL string) teaui.SearchResult {
	if playlist == nil {
		return teaui.SearchResult{}
	}
	playlistID := strings.TrimSpace(playlist.ID)
	if playlistID == "" {
		playlistID = playlistIDFromURL(rawURL)
	}
	if playlistID == "" {
		playlistID = strings.TrimSpace(rawURL)
	}
	title := firstNonEmpty(playlist.Title, playlistID)
	artist := firstNonEmpty(playlist.Author, sourceName)
	metadata := coverart.Metadata{Title: title, Artist: artist, Album: title}.Normalize()
	return teaui.SearchResult{
		ID:              entryIDPrefix + "playlist:" + playlistID,
		Title:           title,
		Subtitle:        artist,
		Source:          sourceName,
		Kind:            teaui.MediaPlaylist,
		Artwork:         metadata,
		PlaylistID:      playlistID,
		CollectionCount: len(playlist.Videos),
	}
}

func playlistEntries(playlist *youtubev2.Playlist, parent teaui.SearchResult) []teaui.SearchResult {
	if playlist == nil {
		return nil
	}
	results := make([]teaui.SearchResult, 0, len(playlist.Videos))
	for _, entry := range playlist.Videos {
		if entry == nil || strings.TrimSpace(entry.ID) == "" {
			continue
		}
		metadata := coverart.Metadata{Title: strings.TrimSpace(entry.Title), Artist: strings.TrimSpace(entry.Author), Album: strings.TrimSpace(parent.Title)}.Normalize()
		results = append(results, teaui.SearchResult{
			ID:       entryIDPrefix + videoURL(entry.ID),
			Title:    firstNonEmpty(entry.Title, entry.ID),
			Subtitle: firstNonEmpty(entry.Author, parent.Title, sourceName),
			Source:   sourceName,
			Kind:     teaui.MediaTrack,
			Duration: entry.Duration,
			Artwork:  metadata,
		})
	}
	return results
}

func musicSearchResults(response musicSearchResponse, request teaui.SearchRequest, maxResults int) []teaui.SearchResult {
	mode := normalizedSearchMode(request.Mode)
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
		if mode == teaui.SearchModeSongs && request.ArtistFilter.Name != "" && !matchesArtistFilter(result, request.ArtistFilter) {
			return
		}
		seen[result.ID] = struct{}{}
		results = append(results, result)
	}

	for _, section := range contents {
		sectionMode := section.searchMode()
		allowUntitledSection := sectionMode == teaui.SearchModeDefault && (mode == teaui.SearchModeSongs || requestTargetsSectionlessFilteredMode(request.Mode, mode))
		if sectionMode != mode && !allowUntitledSection {
			continue
		}
		for _, item := range section.MusicShelfRenderer.Contents {
			result := item.resultForMode(mode)
			appendUnique(result)
			if len(results) >= maxResults {
				return results
			}
		}
	}

	if mode == teaui.SearchModeSongs && len(results) == 0 {
		for _, section := range contents {
			result := section.TopResult()
			appendUnique(result)
			if len(results) >= maxResults {
				return results
			}
		}
	}

	return results
}

func matchesArtistFilter(result teaui.SearchResult, filter teaui.SearchArtistFilter) bool {
	needle := strings.ToLower(strings.TrimSpace(filter.Name))
	if needle == "" {
		return true
	}
	haystacks := []string{result.Subtitle, result.Artwork.Artist, result.Title}
	for _, haystack := range haystacks {
		if strings.Contains(strings.ToLower(strings.TrimSpace(haystack)), needle) {
			return true
		}
	}
	return false
}

func normalizedSearchMode(mode teaui.SearchMode) teaui.SearchMode {
	switch mode {
	case teaui.SearchModeArtists, teaui.SearchModeAlbums, teaui.SearchModePlaylists:
		return mode
	default:
		return teaui.SearchModeSongs
	}
}

func musicSearchContents(response musicSearchResponse) []musicSearchSection {
	for _, tab := range response.Contents.TabbedSearchResultsRenderer.Tabs {
		if strings.EqualFold(strings.TrimSpace(tab.TabRenderer.Title), "YT Music") {
			return tab.TabRenderer.Content.SectionListRenderer.Contents
		}
	}
	return response.Contents.SectionListRenderer.Contents
}

func searchParamsForMode(mode teaui.SearchMode) string {
	switch mode {
	case teaui.SearchModeSongs:
		return "EgWKAQIIAWoMEA4QChADEAQQCRAF"
	case teaui.SearchModeArtists:
		return "EgWKAQIgAWoMEA4QChADEAQQCRAF"
	case teaui.SearchModeAlbums:
		return "EgWKAQIYAWoMEA4QChADEAQQCRAF"
	case teaui.SearchModePlaylists:
		return "Eg-KAQwIABAAGAAgACgBMABqChAEEAMQCRAFEAo%3D"
	default:
		return ""
	}
}

func requestTargetsSectionlessFilteredMode(requestMode, normalizedMode teaui.SearchMode) bool {
	if normalizedMode == teaui.SearchModeSongs {
		return requestMode == teaui.SearchModeSongs
	}
	return requestMode == normalizedMode
}

func (s musicSearchSection) searchMode() teaui.SearchMode {
	title := strings.ToLower(strings.TrimSpace(s.MusicShelfRenderer.Title.FirstText()))
	switch {
	case strings.Contains(title, "artist"):
		return teaui.SearchModeArtists
	case strings.Contains(title, "album"):
		return teaui.SearchModeAlbums
	case strings.Contains(title, "playlist"):
		return teaui.SearchModePlaylists
	case strings.Contains(title, "song"), strings.Contains(title, "video"):
		return teaui.SearchModeSongs
	default:
		return teaui.SearchModeDefault
	}
}

func (s musicSearchSection) TopResult() teaui.SearchResult {
	videoID := strings.TrimSpace(s.MusicCardShelfRenderer.OnTap.WatchEndpoint.VideoID)
	if videoID == "" {
		return teaui.SearchResult{}
	}
	title := firstNonEmpty(s.MusicCardShelfRenderer.Title.FirstText(), videoID)
	artist := s.MusicCardShelfRenderer.Subtitle.ArtistsText()
	album := s.MusicCardShelfRenderer.Subtitle.AlbumText()
	return newMusicTrackResult(videoID, title, artist, album, s.MusicCardShelfRenderer.Subtitle.Duration(), s.MusicCardShelfRenderer.Thumbnail.bestURL())
}

func (r musicSearchResult) resultForMode(mode teaui.SearchMode) teaui.SearchResult {
	switch mode {
	case teaui.SearchModeArtists:
		return r.ToArtistResult()
	case teaui.SearchModeAlbums:
		return r.ToAlbumResult()
	case teaui.SearchModePlaylists:
		return r.ToPlaylistResult()
	default:
		return r.ToTrackResult()
	}
}

func (r musicSearchResult) ToTrackResult() teaui.SearchResult {
	videoID := strings.TrimSpace(firstNonEmpty(r.MusicResponsiveListItemRenderer.PlaylistItemData.VideoID, r.MusicResponsiveListItemRenderer.NavigationEndpoint.WatchEndpoint.VideoID))
	if videoID == "" {
		return teaui.SearchResult{}
	}
	title := r.titleRuns().FirstText()
	metaRuns := r.metaRuns()
	return newMusicTrackResult(videoID, firstNonEmpty(title, videoID), metaRuns.ArtistsText(), metaRuns.AlbumText(), metaRuns.Duration(), r.MusicResponsiveListItemRenderer.Thumbnail.bestURL())
}

func (r musicSearchResult) ToArtistResult() teaui.SearchResult {
	titleRuns := r.titleRuns()
	browseID, name := titleRuns.BrowseIDForPageTypes("MUSIC_PAGE_TYPE_ARTIST", "MUSIC_PAGE_TYPE_USER_CHANNEL")
	if browseID == "" {
		browseID = strings.TrimSpace(r.MusicResponsiveListItemRenderer.NavigationEndpoint.BrowseEndpoint.BrowseID)
		name = firstNonEmpty(name, titleRuns.FirstText())
	}
	name = firstNonEmpty(name, titleRuns.FirstText())
	if name == "" {
		return teaui.SearchResult{}
	}
	return teaui.SearchResult{
		ID:           entryIDPrefix + "artist:" + firstNonEmpty(browseID, sanitizeIdentifier(name)),
		Title:        name,
		Subtitle:     "Apply artist filter to songs",
		Source:       sourceName,
		Kind:         teaui.MediaArtist,
		BrowseID:     browseID,
		ArtistFilter: teaui.SearchArtistFilter{ID: browseID, Name: name},
		Artwork:      coverart.Metadata{Artist: name, RemoteURL: r.MusicResponsiveListItemRenderer.Thumbnail.bestURL()}.Normalize(),
	}
}

func (r musicSearchResult) ToAlbumResult() teaui.SearchResult {
	titleRuns := r.titleRuns()
	browseID, _ := titleRuns.BrowseIDForPageTypes("MUSIC_PAGE_TYPE_ALBUM")
	if browseID == "" {
		browseID = strings.TrimSpace(r.MusicResponsiveListItemRenderer.NavigationEndpoint.BrowseEndpoint.BrowseID)
	}
	title := titleRuns.FirstText()
	if title == "" || browseID == "" {
		return teaui.SearchResult{}
	}
	artists := r.metaRuns().ArtistsText()
	metadata := coverart.Metadata{Title: title, Artist: artists, Album: title, RemoteURL: r.MusicResponsiveListItemRenderer.Thumbnail.bestURL()}.Normalize()
	return teaui.SearchResult{
		ID:              entryIDPrefix + "album:" + browseID,
		Title:           title,
		Subtitle:        firstNonEmpty(artists, "Album"),
		Source:          sourceName,
		Kind:            teaui.MediaAlbum,
		BrowseID:        browseID,
		CollectionCount: r.metaRuns().CollectionCount(),
		Artwork:         metadata,
	}
}

func (r musicSearchResult) ToPlaylistResult() teaui.SearchResult {
	title := r.titleRuns().FirstText()
	playlistID := firstNonEmpty(r.titleRuns().PlaylistID(), r.metaRuns().PlaylistID(), strings.TrimSpace(r.MusicResponsiveListItemRenderer.NavigationEndpoint.WatchEndpoint.PlaylistID))
	browseID := strings.TrimSpace(r.MusicResponsiveListItemRenderer.NavigationEndpoint.BrowseEndpoint.BrowseID)
	if title == "" || (playlistID == "" && browseID == "") {
		return teaui.SearchResult{}
	}
	artists := r.metaRuns().ArtistsText()
	metadata := coverart.Metadata{Title: title, Artist: artists, Album: title, RemoteURL: r.MusicResponsiveListItemRenderer.Thumbnail.bestURL()}.Normalize()
	return teaui.SearchResult{
		ID:              entryIDPrefix + "playlist:" + firstNonEmpty(playlistID, browseID),
		Title:           title,
		Subtitle:        firstNonEmpty(artists, "Playlist"),
		Source:          sourceName,
		Kind:            teaui.MediaPlaylist,
		BrowseID:        browseID,
		PlaylistID:      playlistID,
		CollectionCount: r.metaRuns().CollectionCount(),
		Artwork:         metadata,
	}
}

func (r musicSearchResult) titleRuns() musicRuns {
	if len(r.MusicResponsiveListItemRenderer.FlexColumns) == 0 {
		return musicRuns{}
	}
	return r.MusicResponsiveListItemRenderer.FlexColumns[0].MusicResponsiveListItemFlexColumnRenderer.Text
}

func (r musicSearchResult) metaRuns() musicRuns {
	if len(r.MusicResponsiveListItemRenderer.FlexColumns) < 2 {
		return musicRuns{}
	}
	return r.MusicResponsiveListItemRenderer.FlexColumns[1].MusicResponsiveListItemFlexColumnRenderer.Text
}

func newMusicTrackResult(videoID, title, artist, album string, duration time.Duration, artworkURL string) teaui.SearchResult {
	metadata := coverart.Metadata{Title: title, Artist: artist, Album: album, RemoteURL: artworkURL}.Normalize()
	return teaui.SearchResult{ID: entryIDPrefix + videoURL(videoID), Title: title, Subtitle: firstNonEmpty(artist, album, sourceName), Source: sourceName, Kind: teaui.MediaTrack, Duration: duration, Artwork: metadata}
}

func (n musicThumbnailNode) bestURL() string {
	best := func(thumbnails []musicThumbnail) string {
		bestURL := ""
		bestArea := -1
		for _, thumbnail := range thumbnails {
			url := strings.TrimSpace(thumbnail.URL)
			if url == "" {
				continue
			}
			area := thumbnail.Width * thumbnail.Height
			if area >= bestArea {
				bestArea = area
				bestURL = url
			}
		}
		return bestURL
	}

	for _, thumbnails := range [][]musicThumbnail{
		n.MusicThumbnailRenderer.Thumbnail.Thumbnails,
		n.CroppedSquareThumbnailRenderer.Thumbnail.Thumbnails,
		n.Thumbnail.Thumbnails,
		n.Thumbnails,
	} {
		if url := best(thumbnails); url != "" {
			return url
		}
	}
	return ""
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

// ArtistsText extracts a comma-separated artist list from YouTube Music text runs.
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

// AlbumText extracts the album label from YouTube Music text runs.
func (r musicRuns) AlbumText() string {
	for _, run := range r.Runs {
		pageType := run.NavigationEndpoint.BrowseEndpoint.BrowseEndpointContextSupportedConfigs.BrowseEndpointContextMusicConfig.PageType
		if pageType == "MUSIC_PAGE_TYPE_ALBUM" {
			return strings.TrimSpace(run.Text)
		}
	}
	return ""
}

// BrowseIDForPageTypes extracts the first browse id matching one of the supplied page types.
func (r musicRuns) BrowseIDForPageTypes(pageTypes ...string) (string, string) {
	allowed := make(map[string]struct{}, len(pageTypes))
	for _, pageType := range pageTypes {
		allowed[pageType] = struct{}{}
	}
	for _, run := range r.Runs {
		pageType := run.NavigationEndpoint.BrowseEndpoint.BrowseEndpointContextSupportedConfigs.BrowseEndpointContextMusicConfig.PageType
		if _, ok := allowed[pageType]; !ok {
			continue
		}
		browseID := strings.TrimSpace(run.NavigationEndpoint.BrowseEndpoint.BrowseID)
		if browseID == "" {
			continue
		}
		return browseID, strings.TrimSpace(run.Text)
	}
	return "", ""
}

// PlaylistID extracts a playlist identifier from watch endpoints embedded in the text runs.
func (r musicRuns) PlaylistID() string {
	for _, run := range r.Runs {
		if playlistID := strings.TrimSpace(run.NavigationEndpoint.WatchEndpoint.PlaylistID); playlistID != "" {
			return playlistID
		}
	}
	return ""
}

// CollectionCount parses a loose item count such as "12 songs" from the text runs.
func (r musicRuns) CollectionCount() int {
	for _, run := range r.Runs {
		text := strings.ToLower(strings.TrimSpace(run.Text))
		if !strings.Contains(text, "song") && !strings.Contains(text, "track") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) == 0 {
			continue
		}
		count := 0
		for _, ch := range fields[0] {
			if ch < '0' || ch > '9' {
				count = 0
				break
			}
			count = count*10 + int(ch-'0')
		}
		if count > 0 {
			return count
		}
	}
	return 0
}

// Duration parses the first clock-style duration found in the text runs.
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

func browseCollectionTracks(payload any, parent teaui.SearchResult) []teaui.SearchResult {
	items := collectResponsiveSearchItems(payload)
	results := make([]teaui.SearchResult, 0, len(items))
	seen := make(map[string]struct{})
	for _, item := range items {
		result := item.ToTrackResult()
		if result.ID == "" {
			continue
		}
		if _, exists := seen[result.ID]; exists {
			continue
		}
		seen[result.ID] = struct{}{}
		if parent.Kind == teaui.MediaAlbum && result.Artwork.Album == "" {
			result.Artwork = result.Artwork.Merge(coverart.Metadata{Album: parent.Title, RemoteURL: parent.Artwork.RemoteURL})
		}
		if strings.TrimSpace(result.Subtitle) == "" {
			result.Subtitle = firstNonEmpty(parent.Subtitle, sourceName)
		}
		results = append(results, result)
	}
	return results
}

func collectResponsiveSearchItems(node any) []musicSearchResult {
	items := make([]musicSearchResult, 0, 32)
	collectResponsiveSearchItemsInto(node, &items)
	return items
}

func collectResponsiveSearchItemsInto(node any, items *[]musicSearchResult) {
	switch typed := node.(type) {
	case map[string]any:
		if renderer, ok := typed["musicResponsiveListItemRenderer"]; ok {
			wrapped := map[string]any{"musicResponsiveListItemRenderer": renderer}
			body, err := json.Marshal(wrapped)
			if err == nil {
				var item musicSearchResult
				if err := json.Unmarshal(body, &item); err == nil {
					*items = append(*items, item)
				}
			}
		}
		for _, child := range typed {
			collectResponsiveSearchItemsInto(child, items)
		}
	case []any:
		for _, child := range typed {
			collectResponsiveSearchItemsInto(child, items)
		}
	}
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

func playlistIDFromURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Query().Get("list"))
}

func playlistURL(id string) string {
	return "https://music.youtube.com/playlist?list=" + url.QueryEscape(strings.TrimSpace(id))
}

func sanitizeIdentifier(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")
	if value == "" {
		return "unknown"
	}
	return value
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
