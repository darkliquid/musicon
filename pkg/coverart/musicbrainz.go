package coverart

import (
	"context"
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
	defaultMusicBrainzBaseURL = "https://musicbrainz.org/ws/2"
	defaultCoverArtBaseURL    = "https://coverartarchive.org"
	defaultMusicBrainzRate    = time.Second
)

// MusicBrainzProvider resolves artwork via MusicBrainz IDs or search plus the Cover Art Archive.
type MusicBrainzProvider struct {
	Client          *http.Client
	UserAgent       string
	BaseURL         string
	CoverArtBaseURL string
	RateLimit       time.Duration

	mu          sync.Mutex
	lastRequest time.Time
}

// NewMusicBrainzProvider constructs a provider with sensible defaults.
func NewMusicBrainzProvider(userAgent string) *MusicBrainzProvider {
	return &MusicBrainzProvider{
		Client:          http.DefaultClient,
		UserAgent:       strings.TrimSpace(userAgent),
		BaseURL:         defaultMusicBrainzBaseURL,
		CoverArtBaseURL: defaultCoverArtBaseURL,
		RateLimit:       defaultMusicBrainzRate,
	}
}

// Name returns the provider's stable identifier.
func (p *MusicBrainzProvider) Name() string { return "musicbrainz" }

// Lookup resolves cover art using explicit MusicBrainz IDs first, then search fallback.
func (p *MusicBrainzProvider) Lookup(ctx context.Context, metadata Metadata) (Result, error) {
	metadata = metadata.Normalize()

	if id := metadata.IDs.MusicBrainzReleaseID; id != "" {
		return p.fetchReleaseCover(ctx, id)
	}
	if id := metadata.IDs.MusicBrainzReleaseGroupID; id != "" {
		return p.fetchReleaseGroupCover(ctx, id)
	}
	if id := metadata.IDs.MusicBrainzRecordingID; id != "" {
		releaseID, groupID, err := p.lookupRecording(ctx, id)
		if err != nil {
			return Result{}, err
		}
		if releaseID != "" {
			return p.fetchReleaseCover(ctx, releaseID)
		}
		if groupID != "" {
			return p.fetchReleaseGroupCover(ctx, groupID)
		}
	}

	releaseID, groupID, err := p.searchRelease(ctx, metadata)
	if err != nil {
		return Result{}, err
	}
	if releaseID != "" {
		return p.fetchReleaseCover(ctx, releaseID)
	}
	if groupID != "" {
		return p.fetchReleaseGroupCover(ctx, groupID)
	}
	return Result{}, ErrNotFound
}

func (p *MusicBrainzProvider) lookupRecording(ctx context.Context, recordingID string) (string, string, error) {
	if strings.TrimSpace(recordingID) == "" {
		return "", "", ErrNotFound
	}

	values := url.Values{}
	values.Set("fmt", "json")
	values.Set("inc", "releases+release-groups")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.baseURL(), "/")+"/recording/"+url.PathEscape(recordingID)+"?"+values.Encode(), nil)
	if err != nil {
		return "", "", err
	}
	if p.UserAgent != "" {
		req.Header.Set("User-Agent", p.UserAgent)
	}

	if err := p.waitForRateLimit(ctx); err != nil {
		return "", "", err
	}

	resp, err := p.client().Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", "", ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("musicbrainz recording lookup returned %s", resp.Status)
	}

	var payload struct {
		Releases []struct {
			ID           string `json:"id"`
			ReleaseGroup struct {
				ID string `json:"id"`
			} `json:"release-group"`
		} `json:"releases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", err
	}
	if len(payload.Releases) == 0 {
		return "", "", ErrNotFound
	}
	return payload.Releases[0].ID, payload.Releases[0].ReleaseGroup.ID, nil
}

func (p *MusicBrainzProvider) fetchReleaseCover(ctx context.Context, releaseID string) (Result, error) {
	return p.fetchCover(ctx, fmt.Sprintf("%s/release/%s/front-500", strings.TrimRight(p.coverArtBaseURL(), "/"), url.PathEscape(releaseID)), "MusicBrainz release "+releaseID)
}

func (p *MusicBrainzProvider) fetchReleaseGroupCover(ctx context.Context, groupID string) (Result, error) {
	return p.fetchCover(ctx, fmt.Sprintf("%s/release-group/%s/front-500", strings.TrimRight(p.coverArtBaseURL(), "/"), url.PathEscape(groupID)), "MusicBrainz release group "+groupID)
}

func (p *MusicBrainzProvider) fetchCover(ctx context.Context, url, description string) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
		return Result{}, fmt.Errorf("cover art archive returned %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Image: Image{
			Data:        data,
			MIMEType:    strings.TrimSpace(resp.Header.Get("Content-Type")),
			Description: description,
		},
		Provider: p.Name(),
	}, nil
}

func (p *MusicBrainzProvider) searchRelease(ctx context.Context, metadata Metadata) (string, string, error) {
	if metadata.Album == "" && metadata.Title == "" && metadata.Artist == "" {
		return "", "", ErrNotFound
	}

	values := url.Values{}
	values.Set("fmt", "json")
	values.Set("limit", "1")
	values.Set("query", musicBrainzQuery(metadata))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.baseURL(), "/")+"/release?"+values.Encode(), nil)
	if err != nil {
		return "", "", err
	}
	if p.UserAgent != "" {
		req.Header.Set("User-Agent", p.UserAgent)
	}

	if err := p.waitForRateLimit(ctx); err != nil {
		return "", "", err
	}

	resp, err := p.client().Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", "", ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("musicbrainz returned %s", resp.Status)
	}

	var payload struct {
		Releases []struct {
			ID           string `json:"id"`
			ReleaseGroup struct {
				ID string `json:"id"`
			} `json:"release-group"`
		} `json:"releases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", err
	}
	if len(payload.Releases) == 0 {
		return "", "", ErrNotFound
	}
	return payload.Releases[0].ID, payload.Releases[0].ReleaseGroup.ID, nil
}

func musicBrainzQuery(metadata Metadata) string {
	parts := make([]string, 0, 3)
	if metadata.Album != "" {
		parts = append(parts, fmt.Sprintf(`release:"%s"`, escapeMusicBrainzQuery(metadata.Album)))
	}
	if metadata.Artist != "" {
		parts = append(parts, fmt.Sprintf(`artist:"%s"`, escapeMusicBrainzQuery(metadata.Artist)))
	}
	if metadata.Title != "" {
		parts = append(parts, fmt.Sprintf(`recording:"%s"`, escapeMusicBrainzQuery(metadata.Title)))
	}
	return strings.Join(parts, " AND ")
}

func escapeMusicBrainzQuery(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func (p *MusicBrainzProvider) waitForRateLimit(ctx context.Context) error {
	if p.RateLimit <= 0 {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	wait := p.lastRequest.Add(p.RateLimit).Sub(now)
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			p.mu.Lock()
			return ctx.Err()
		case <-timer.C:
		}
		p.mu.Lock()
	}
	p.lastRequest = time.Now()
	return nil
}

func (p *MusicBrainzProvider) client() *http.Client {
	if p != nil && p.Client != nil {
		return p.Client
	}
	return http.DefaultClient
}

func (p *MusicBrainzProvider) baseURL() string {
	if p != nil && strings.TrimSpace(p.BaseURL) != "" {
		return strings.TrimSpace(p.BaseURL)
	}
	return defaultMusicBrainzBaseURL
}

func (p *MusicBrainzProvider) coverArtBaseURL() string {
	if p != nil && strings.TrimSpace(p.CoverArtBaseURL) != "" {
		return strings.TrimSpace(p.CoverArtBaseURL)
	}
	return defaultCoverArtBaseURL
}
