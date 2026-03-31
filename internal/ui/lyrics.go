package ui

import (
	"context"

	"github.com/darkliquid/musicon/pkg/lyrics"
)

// LyricsResolver matches reusable lyrics resolvers such as pkg/lyrics.Chain.
type LyricsResolver interface {
	Resolve(context.Context, lyrics.Request) (lyrics.Document, error)
}

type lyricsProvider struct {
	resolver LyricsResolver
}

// NewLyricsProvider adapts a reusable lyrics resolver to the UI lyrics contract.
func NewLyricsProvider(resolver LyricsResolver) LyricsProvider {
	return lyricsProvider{resolver: resolver}
}

// Lyrics resolves lyrics for the supplied request.
func (p lyricsProvider) Lyrics(request lyrics.Request) (*lyrics.Document, error) {
	if p.resolver == nil {
		return nil, nil
	}
	document, err := p.resolver.Resolve(context.Background(), request)
	if err != nil {
		if lyrics.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &document, nil
}
