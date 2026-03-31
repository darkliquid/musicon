package coverart

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultSpotifyBaseURL  = "https://api.spotify.com/v1"
	defaultSpotifyTokenURL = "https://accounts.spotify.com/api/token"
)

// SpotifyProvider resolves artwork from Spotify album metadata.
type SpotifyProvider struct {
	Client       *http.Client
	ClientID     string
	ClientSecret string
	Market       string
	BaseURL      string
	TokenURL     string

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// Name returns the provider's stable identifier.
func (p *SpotifyProvider) Name() string { return "spotify" }

// Lookup resolves Spotify artwork via album ID first, then search fallback.
func (p *SpotifyProvider) Lookup(ctx context.Context, metadata Metadata) (Result, error) {
	metadata = metadata.Normalize()
	if strings.TrimSpace(p.ClientID) == "" || strings.TrimSpace(p.ClientSecret) == "" {
		return Result{}, ErrNotFound
	}

	albumID := metadata.IDs.SpotifyAlbumID
	if albumID == "" && metadata.IDs.SpotifyTrackID != "" {
		var err error
		albumID, err = p.lookupAlbumForTrack(ctx, metadata.IDs.SpotifyTrackID)
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

func (p *SpotifyProvider) lookupAlbumForTrack(ctx context.Context, trackID string) (string, error) {
	if strings.TrimSpace(trackID) == "" {
		return "", ErrNotFound
	}

	values := url.Values{}
	if market := strings.TrimSpace(p.Market); market != "" {
		values.Set("market", market)
	}
	endpoint := strings.TrimRight(p.baseURL(), "/") + "/tracks/" + url.PathEscape(trackID)
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	if err := p.authorize(ctx, req); err != nil {
		return "", err
	}

	resp, err := p.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("spotify track lookup returned %s", resp.Status)
	}

	var payload struct {
		Album struct {
			ID string `json:"id"`
		} `json:"album"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.Album.ID) == "" {
		return "", ErrNotFound
	}
	return payload.Album.ID, nil
}

func (p *SpotifyProvider) searchAlbum(ctx context.Context, metadata Metadata) (string, error) {
	if metadata.Album == "" && metadata.Title == "" && metadata.Artist == "" {
		return "", ErrNotFound
	}

	values := url.Values{}
	values.Set("type", "album")
	values.Set("limit", "1")
	values.Set("q", spotifyQuery(metadata))
	if market := strings.TrimSpace(p.Market); market != "" {
		values.Set("market", market)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.baseURL(), "/")+"/search?"+values.Encode(), nil)
	if err != nil {
		return "", err
	}
	if err := p.authorize(ctx, req); err != nil {
		return "", err
	}

	resp, err := p.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("spotify search returned %s", resp.Status)
	}

	var payload struct {
		Albums struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"albums"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if len(payload.Albums.Items) == 0 {
		return "", ErrNotFound
	}
	return payload.Albums.Items[0].ID, nil
}

func (p *SpotifyProvider) fetchAlbumCover(ctx context.Context, albumID string) (Result, error) {
	if strings.TrimSpace(albumID) == "" {
		return Result{}, ErrNotFound
	}

	values := url.Values{}
	if market := strings.TrimSpace(p.Market); market != "" {
		values.Set("market", market)
	}
	url := strings.TrimRight(p.baseURL(), "/") + "/albums/" + url.PathEscape(albumID)
	if encoded := values.Encode(); encoded != "" {
		url += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{}, err
	}
	if err := p.authorize(ctx, req); err != nil {
		return Result{}, err
	}

	resp, err := p.client().Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return Result{}, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("spotify album lookup returned %s", resp.Status)
	}

	var payload struct {
		Name   string `json:"name"`
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Result{}, err
	}
	if len(payload.Images) == 0 || strings.TrimSpace(payload.Images[0].URL) == "" {
		return Result{}, ErrNotFound
	}

	imageReq, err := http.NewRequestWithContext(ctx, http.MethodGet, payload.Images[0].URL, nil)
	if err != nil {
		return Result{}, err
	}
	imageResp, err := p.client().Do(imageReq)
	if err != nil {
		return Result{}, err
	}
	defer imageResp.Body.Close()

	if imageResp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("spotify image fetch returned %s", imageResp.Status)
	}
	data, err := io.ReadAll(imageResp.Body)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Image: Image{
			Data:        data,
			MIMEType:    strings.TrimSpace(imageResp.Header.Get("Content-Type")),
			Description: "Spotify album " + albumID,
		},
		Provider: p.Name(),
	}, nil
}

func (p *SpotifyProvider) authorize(ctx context.Context, req *http.Request) error {
	token, err := p.token(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

func (p *SpotifyProvider) token(ctx context.Context) (string, error) {
	p.mu.Lock()
	if p.accessToken != "" && time.Now().Before(p.expiresAt.Add(-time.Minute)) {
		token := p.accessToken
		p.mu.Unlock()
		return token, nil
	}
	p.mu.Unlock()

	values := url.Values{}
	values.Set("grant_type", "client_credentials")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL(), strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	auth := base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(p.ClientID) + ":" + strings.TrimSpace(p.ClientSecret)))
	req.Header.Set("Authorization", "Basic "+auth)

	resp, err := p.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("spotify token request returned %s", resp.Status)
	}

	var payload struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", fmt.Errorf("spotify token response missing access_token")
	}

	p.mu.Lock()
	p.accessToken = payload.AccessToken
	p.expiresAt = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	token := p.accessToken
	p.mu.Unlock()
	return token, nil
}

func spotifyQuery(metadata Metadata) string {
	parts := make([]string, 0, 3)
	if metadata.Album != "" {
		parts = append(parts, fmt.Sprintf(`album:"%s"`, escapeSpotifyQuery(metadata.Album)))
	}
	if metadata.Artist != "" {
		parts = append(parts, fmt.Sprintf(`artist:"%s"`, escapeSpotifyQuery(metadata.Artist)))
	}
	if metadata.Title != "" {
		parts = append(parts, fmt.Sprintf(`track:"%s"`, escapeSpotifyQuery(metadata.Title)))
	}
	return strings.Join(parts, " ")
}

func escapeSpotifyQuery(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func (p *SpotifyProvider) client() *http.Client {
	if p != nil && p.Client != nil {
		return p.Client
	}
	return http.DefaultClient
}

func (p *SpotifyProvider) baseURL() string {
	if p != nil && strings.TrimSpace(p.BaseURL) != "" {
		return strings.TrimSpace(p.BaseURL)
	}
	return defaultSpotifyBaseURL
}

func (p *SpotifyProvider) tokenURL() string {
	if p != nil && strings.TrimSpace(p.TokenURL) != "" {
		return strings.TrimSpace(p.TokenURL)
	}
	return defaultSpotifyTokenURL
}
