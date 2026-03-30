package coverart

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const defaultAppleMusicBaseURL = "https://api.music.apple.com/v1"

// AppleMusicProvider resolves artwork from Apple Music catalog album metadata.
type AppleMusicProvider struct {
	Client         *http.Client
	DeveloperToken string
	Storefront     string
	BaseURL        string
}

func (p *AppleMusicProvider) Name() string { return "apple-music" }

// Lookup resolves Apple Music artwork via album ID first, then metadata search fallback.
func (p *AppleMusicProvider) Lookup(ctx context.Context, metadata Metadata) (Result, error) {
	metadata = metadata.Normalize()
	if strings.TrimSpace(p.DeveloperToken) == "" || strings.TrimSpace(p.Storefront) == "" {
		return Result{}, ErrNotFound
	}

	albumID := metadata.IDs.AppleMusicAlbumID
	if albumID == "" && metadata.IDs.AppleMusicSongID != "" {
		var err error
		albumID, err = p.lookupAlbumForSong(ctx, metadata.IDs.AppleMusicSongID)
		if err != nil {
			return Result{}, err
		}
	}
	if albumID == "" {
		var err error
		albumID, err = p.searchAlbum(ctx, metadata)
		if err != nil {
			return Result{}, err
		}
	}
	return p.fetchAlbumCover(ctx, albumID)
}

func (p *AppleMusicProvider) lookupAlbumForSong(ctx context.Context, songID string) (string, error) {
	if strings.TrimSpace(songID) == "" {
		return "", ErrNotFound
	}

	values := url.Values{}
	values.Set("include", "albums")

	endpoint := fmt.Sprintf("%s/catalog/%s/songs/%s?%s", strings.TrimRight(p.baseURL(), "/"), url.PathEscape(strings.TrimSpace(p.Storefront)), url.PathEscape(songID), values.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	p.authorize(req)

	resp, err := p.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("apple music song lookup returned %s", resp.Status)
	}

	var payload struct {
		Data []struct {
			Relationships struct {
				Albums struct {
					Data []struct {
						ID string `json:"id"`
					} `json:"data"`
				} `json:"albums"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if len(payload.Data) == 0 || len(payload.Data[0].Relationships.Albums.Data) == 0 {
		return "", ErrNotFound
	}
	albumID := strings.TrimSpace(payload.Data[0].Relationships.Albums.Data[0].ID)
	if albumID == "" {
		return "", ErrNotFound
	}
	return albumID, nil
}

func (p *AppleMusicProvider) searchAlbum(ctx context.Context, metadata Metadata) (string, error) {
	if metadata.Album == "" && metadata.Title == "" && metadata.Artist == "" {
		return "", ErrNotFound
	}

	values := url.Values{}
	values.Set("term", appleSearchTerm(metadata))
	values.Set("types", "albums")
	values.Set("limit", "1")

	endpoint := fmt.Sprintf("%s/catalog/%s/search?%s", strings.TrimRight(p.baseURL(), "/"), url.PathEscape(strings.TrimSpace(p.Storefront)), values.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	p.authorize(req)

	resp, err := p.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("apple music search returned %s", resp.Status)
	}

	var payload struct {
		Results struct {
			Albums struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			} `json:"albums"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if len(payload.Results.Albums.Data) == 0 {
		return "", ErrNotFound
	}
	return payload.Results.Albums.Data[0].ID, nil
}

func (p *AppleMusicProvider) fetchAlbumCover(ctx context.Context, albumID string) (Result, error) {
	if strings.TrimSpace(albumID) == "" {
		return Result{}, ErrNotFound
	}

	endpoint := fmt.Sprintf("%s/catalog/%s/albums/%s", strings.TrimRight(p.baseURL(), "/"), url.PathEscape(strings.TrimSpace(p.Storefront)), url.PathEscape(albumID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Result{}, err
	}
	p.authorize(req)

	resp, err := p.client().Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return Result{}, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("apple music album lookup returned %s", resp.Status)
	}

	var payload struct {
		Data []struct {
			Attributes struct {
				Artwork struct {
					URL    string `json:"url"`
					Width  int    `json:"width"`
					Height int    `json:"height"`
				} `json:"artwork"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Result{}, err
	}
	if len(payload.Data) == 0 {
		return Result{}, ErrNotFound
	}
	art := payload.Data[0].Attributes.Artwork
	imageURL := appleArtworkURL(art.URL, art.Width, art.Height)
	if imageURL == "" {
		return Result{}, ErrNotFound
	}

	imageReq, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return Result{}, err
	}
	imageResp, err := p.client().Do(imageReq)
	if err != nil {
		return Result{}, err
	}
	defer imageResp.Body.Close()

	if imageResp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("apple music image fetch returned %s", imageResp.Status)
	}
	data, err := io.ReadAll(imageResp.Body)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Image: Image{
			Data:        data,
			MIMEType:    strings.TrimSpace(imageResp.Header.Get("Content-Type")),
			Description: "Apple Music album " + albumID,
		},
		Provider: p.Name(),
	}, nil
}

func appleSearchTerm(metadata Metadata) string {
	parts := make([]string, 0, 3)
	if metadata.Album != "" {
		parts = append(parts, metadata.Album)
	}
	if metadata.Artist != "" {
		parts = append(parts, metadata.Artist)
	}
	if metadata.Title != "" {
		parts = append(parts, metadata.Title)
	}
	return strings.Join(parts, " ")
}

func appleArtworkURL(template string, width, height int) string {
	template = strings.TrimSpace(template)
	if template == "" {
		return ""
	}
	size := minPositive(width, height)
	if size <= 0 {
		if width > 0 {
			size = width
		} else if height > 0 {
			size = height
		} else {
			size = 1000
		}
	}
	if size > 1000 {
		size = 1000
	}
	value := strconv.Itoa(size)
	template = strings.ReplaceAll(template, "{w}", value)
	template = strings.ReplaceAll(template, "{h}", value)
	return template
}

func minPositive(values ...int) int {
	min := 0
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if min == 0 || value < min {
			min = value
		}
	}
	return min
}

func (p *AppleMusicProvider) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(p.DeveloperToken))
}

func (p *AppleMusicProvider) client() *http.Client {
	if p != nil && p.Client != nil {
		return p.Client
	}
	return http.DefaultClient
}

func (p *AppleMusicProvider) baseURL() string {
	if p != nil && strings.TrimSpace(p.BaseURL) != "" {
		return strings.TrimSpace(p.BaseURL)
	}
	return defaultAppleMusicBaseURL
}
