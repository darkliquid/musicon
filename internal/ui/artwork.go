package ui

import (
	"context"

	"github.com/darkliquid/musicon/pkg/components"
	"github.com/darkliquid/musicon/pkg/coverart"
)

// CoverArtResolver matches reusable cover-art resolvers such as pkg/coverart.Chain.
type CoverArtResolver interface {
	Resolve(ctx context.Context, metadata coverart.Metadata) (coverart.Result, error)
}

type coverArtProvider struct {
	resolver CoverArtResolver
}

// NewCoverArtProvider adapts a reusable cover-art resolver to the UI artwork contract.
func NewCoverArtProvider(resolver CoverArtResolver) ArtworkProvider {
	return coverArtProvider{resolver: resolver}
}

func (p coverArtProvider) Artwork(metadata coverart.Metadata) (*components.ImageSource, error) {
	if p.resolver == nil {
		return nil, nil
	}
	result, err := p.resolver.Resolve(context.Background(), metadata)
	if err != nil {
		if coverart.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &components.ImageSource{
		Data:        append([]byte(nil), result.Image.Data...),
		Description: result.Image.Description,
	}, nil
}
