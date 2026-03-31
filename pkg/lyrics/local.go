package lyrics

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalFileProvider resolves lyrics from a neighboring .lrc sidecar file.
type LocalFileProvider struct{}

// Name returns the provider's stable identifier.
func (LocalFileProvider) Name() string { return "local-lrc" }

// Lookup resolves lyrics from a local .lrc sidecar file when present.
func (LocalFileProvider) Lookup(ctx context.Context, request Request) (Document, error) {
	_ = ctx
	request = request.Normalize()
	if request.LocalAudioPath == "" {
		return Document{}, ErrNotFound
	}
	path := strings.TrimSuffix(request.LocalAudioPath, filepath.Ext(request.LocalAudioPath)) + ".lrc"
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Document{}, ErrNotFound
		}
		return Document{}, err
	}
	document := ParseLRC(string(data))
	document.Provider = "local-lrc"
	document.Source = firstNonEmpty(document.Source, "local")
	document.TrackName = firstNonEmpty(document.TrackName, request.Title)
	document.ArtistName = firstNonEmpty(document.ArtistName, request.Artist)
	document.AlbumName = firstNonEmpty(document.AlbumName, request.Album)
	document.Duration = maxDuration(document.Duration, request.Duration)
	if document.Empty() {
		return Document{}, ErrNotFound
	}
	return document, nil
}

func maxDuration(values ...time.Duration) time.Duration {
	var max time.Duration
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}
