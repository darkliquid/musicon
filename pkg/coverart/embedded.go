package coverart

import (
	"context"
	"errors"
	"os"

	"github.com/dhowden/tag"
)

// EmbeddedProvider resolves artwork from embedded local metadata.
type EmbeddedProvider struct{}

// Name returns the provider's stable identifier.
func (EmbeddedProvider) Name() string { return "embedded" }

// Lookup returns already-supplied embedded artwork when present, then falls back
// to best-effort local tag parsing from the audio file.
func (EmbeddedProvider) Lookup(ctx context.Context, metadata Metadata) (Result, error) {
	_ = ctx
	metadata = metadata.Normalize()
	if metadata.Local == nil {
		return Result{}, ErrNotFound
	}
	if embedded := metadata.Local.Embedded; embedded != nil && len(embedded.Data) > 0 {
		return Result{Image: *embedded, Provider: "embedded"}, nil
	}
	if metadata.Local.AudioPath == "" {
		return Result{}, ErrNotFound
	}

	file, err := os.Open(metadata.Local.AudioPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{}, ErrNotFound
		}
		return Result{}, err
	}
	defer file.Close()

	parsed, err := tag.ReadFrom(file)
	if err != nil {
		if errors.Is(err, tag.ErrNoTagsFound) {
			return Result{}, ErrNotFound
		}
		return Result{}, err
	}

	picture := parsed.Picture()
	if picture == nil || len(picture.Data) == 0 {
		return Result{}, ErrNotFound
	}

	return Result{
		Image: Image{
			Data:        picture.Data,
			MIMEType:    picture.MIMEType,
			Description: picture.Description,
		},
		Provider: "embedded",
	}, nil
}
