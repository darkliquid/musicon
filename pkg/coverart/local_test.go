package coverart

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
	0x00, 0x03, 0x01, 0x01, 0x00, 0xc9, 0xfe, 0x92,
	0xef, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
	0x44, 0xae, 0x42, 0x60, 0x82,
}

func TestLocalFilesProviderPrefersExplicitCoverPath(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "song.mp3")
	explicit := filepath.Join(dir, "custom.png")
	fallback := filepath.Join(dir, "cover.jpg")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	if err := os.WriteFile(explicit, tinyPNG, 0o644); err != nil {
		t.Fatalf("write explicit cover: %v", err)
	}
	if err := os.WriteFile(fallback, []byte("fallback"), 0o644); err != nil {
		t.Fatalf("write fallback cover: %v", err)
	}

	provider := NewLocalFilesProvider()
	result, err := provider.Lookup(context.Background(), Metadata{
		Local: &LocalMetadata{
			AudioPath:     audioPath,
			CoverFilePath: explicit,
		},
	})
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if got := result.Image.Description; got != "custom.png" {
		t.Fatalf("expected explicit cover, got %q", got)
	}
}

func TestLocalFilesProviderScansSiblingCovers(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "song.flac")
	coverPath := filepath.Join(dir, "folder.png")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	if err := os.WriteFile(coverPath, tinyPNG, 0o644); err != nil {
		t.Fatalf("write cover: %v", err)
	}

	provider := NewLocalFilesProvider()
	result, err := provider.Lookup(context.Background(), Metadata{
		Title: "Song",
		Local: &LocalMetadata{AudioPath: audioPath},
	})
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if got := result.Image.Description; got != "folder.png" {
		t.Fatalf("expected scanned cover, got %q", got)
	}
	if got := result.Image.MIMEType; got != "image/png" {
		t.Fatalf("expected image/png, got %q", got)
	}
}

func TestLocalFilesProviderReturnsNotFoundWithoutLocalContext(t *testing.T) {
	provider := NewLocalFilesProvider()
	_, err := provider.Lookup(context.Background(), Metadata{Title: "Song"})
	if !IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}
