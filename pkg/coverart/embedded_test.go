package coverart

import (
	"context"
	"testing"
)

func TestEmbeddedProviderUsesInlineEmbeddedImage(t *testing.T) {
	provider := EmbeddedProvider{}
	result, err := provider.Lookup(context.Background(), Metadata{
		Local: &LocalMetadata{
			Embedded: &Image{
				Data:        []byte("image"),
				MIMEType:    "image/png",
				Description: "embedded",
			},
		},
	})
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if result.Provider != "embedded" {
		t.Fatalf("expected embedded provider, got %q", result.Provider)
	}
	if got := string(result.Image.Data); got != "image" {
		t.Fatalf("expected embedded data, got %q", got)
	}
}

func TestEmbeddedProviderWithoutLocalMetadataIsNotFound(t *testing.T) {
	provider := EmbeddedProvider{}
	_, err := provider.Lookup(context.Background(), Metadata{Title: "Song"})
	if !IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}
