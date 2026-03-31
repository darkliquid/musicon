package coverart

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const defaultLastFMBaseURL = "https://ws.audioscrobbler.com/2.0"

// LastFMProvider resolves artwork via the Last.fm album API.
type LastFMProvider struct {
	Client  *http.Client
	APIKey  string
	BaseURL string
}

// Name returns the provider's stable identifier.
func (p *LastFMProvider) Name() string { return "lastfm" }

// Lookup resolves Last.fm artwork by MusicBrainz album ID when available, then
// falls back to artist+album metadata search.
func (p *LastFMProvider) Lookup(ctx context.Context, metadata Metadata) (Result, error) {
	metadata = metadata.Normalize()
	if strings.TrimSpace(p.APIKey) == "" {
		return Result{}, ErrNotFound
	}

	values := url.Values{}
	values.Set("method", "album.getinfo")
	values.Set("api_key", strings.TrimSpace(p.APIKey))
	values.Set("format", "json")
	values.Set("autocorrect", "1")

	switch {
	case metadata.IDs.MusicBrainzReleaseID != "":
		values.Set("mbid", metadata.IDs.MusicBrainzReleaseID)
	case metadata.Artist != "" && metadata.Album != "":
		values.Set("artist", metadata.Artist)
		values.Set("album", metadata.Album)
	default:
		return Result{}, ErrNotFound
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.baseURL(), "/")+"/?"+values.Encode(), nil)
	if err != nil {
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
		return Result{}, fmt.Errorf("last.fm album lookup returned %s", resp.Status)
	}

	var payload struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
		Album   struct {
			Image []struct {
				URL  string `json:"#text"`
				Size string `json:"size"`
			} `json:"image"`
		} `json:"album"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Result{}, err
	}
	if payload.Error != 0 {
		if payload.Error == 6 || payload.Error == 7 {
			return Result{}, ErrNotFound
		}
		if payload.Message == "" {
			return Result{}, fmt.Errorf("last.fm returned error %d", payload.Error)
		}
		return Result{}, fmt.Errorf("last.fm returned error %d: %s", payload.Error, payload.Message)
	}

	imageURL := lastFMImageURL(payload.Album.Image)
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

	if imageResp.StatusCode == http.StatusNotFound {
		return Result{}, ErrNotFound
	}
	if imageResp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("last.fm image fetch returned %s", imageResp.Status)
	}

	data, err := io.ReadAll(imageResp.Body)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Image: Image{
			Data:        data,
			MIMEType:    strings.TrimSpace(imageResp.Header.Get("Content-Type")),
			Description: "Last.fm album artwork",
		},
		Provider: p.Name(),
	}, nil
}

func lastFMImageURL(images []struct {
	URL  string `json:"#text"`
	Size string `json:"size"`
}) string {
	bestRank := -1
	bestURL := ""
	for _, image := range images {
		imageURL := strings.TrimSpace(image.URL)
		if imageURL == "" {
			continue
		}
		rank := lastFMImageRank(strings.TrimSpace(image.Size))
		if rank > bestRank {
			bestRank = rank
			bestURL = imageURL
		}
	}
	return bestURL
}

func lastFMImageRank(size string) int {
	switch strings.ToLower(strings.TrimSpace(size)) {
	case "mega":
		return 6
	case "extralarge":
		return 5
	case "large":
		return 4
	case "medium":
		return 3
	case "small":
		return 2
	default:
		return 1
	}
}

func (p *LastFMProvider) client() *http.Client {
	if p != nil && p.Client != nil {
		return p.Client
	}
	return http.DefaultClient
}

func (p *LastFMProvider) baseURL() string {
	if p != nil && strings.TrimSpace(p.BaseURL) != "" {
		return strings.TrimSpace(p.BaseURL)
	}
	return defaultLastFMBaseURL
}
