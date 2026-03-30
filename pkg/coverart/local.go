package coverart

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var defaultLocalCoverBaseNames = []string{
	"cover",
	"folder",
	"front",
	"album",
	"artwork",
}

var defaultLocalCoverExtensions = []string{
	".jpg",
	".jpeg",
	".png",
	".bmp",
	".gif",
	".webp",
}

// LocalFilesProvider looks for sidecar artwork files next to local audio files.
type LocalFilesProvider struct {
	BaseNames  []string
	Extensions []string
}

// NewLocalFilesProvider creates a local sidecar cover provider with default
// base-name and extension fallbacks.
func NewLocalFilesProvider() LocalFilesProvider {
	return LocalFilesProvider{
		BaseNames:  append([]string(nil), defaultLocalCoverBaseNames...),
		Extensions: append([]string(nil), defaultLocalCoverExtensions...),
	}
}

func (p LocalFilesProvider) Name() string { return "local-files" }

// Lookup resolves artwork from an explicit sidecar path or common neighboring filenames.
func (p LocalFilesProvider) Lookup(ctx context.Context, metadata Metadata) (Result, error) {
	_ = ctx
	metadata = metadata.Normalize()
	if metadata.Local == nil {
		return Result{}, ErrNotFound
	}

	candidates := p.candidates(metadata.Local)
	for _, candidate := range candidates {
		image, err := readLocalImage(candidate)
		switch {
		case err == nil:
			return Result{
				Image: Image{
					Data:        image.Data,
					MIMEType:    image.MIMEType,
					Description: image.Description,
				},
				Provider: p.Name(),
			}, nil
		case os.IsNotExist(err):
			continue
		default:
			return Result{}, err
		}
	}

	return Result{}, ErrNotFound
}

func (p LocalFilesProvider) candidates(local *LocalMetadata) []string {
	if local == nil {
		return nil
	}

	seen := map[string]struct{}{}
	candidates := make([]string, 0, 1+len(p.BaseNames)*len(p.Extensions))
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		candidates = append(candidates, clean)
	}

	add(local.CoverFilePath)
	if strings.TrimSpace(local.AudioPath) == "" {
		return candidates
	}

	dir := filepath.Dir(local.AudioPath)
	for _, base := range withDefaults(p.BaseNames, defaultLocalCoverBaseNames) {
		base = strings.TrimSpace(base)
		if base == "" {
			continue
		}
		for _, ext := range withDefaults(p.Extensions, defaultLocalCoverExtensions) {
			ext = normalizeExt(ext)
			if ext == "" {
				continue
			}
			add(filepath.Join(dir, base+ext))
			add(filepath.Join(dir, strings.ToUpper(base)+ext))
			add(filepath.Join(dir, strings.Title(base)+ext))
		}
	}

	return candidates
}

func readLocalImage(path string) (Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Image{}, err
	}

	mimeType := strings.TrimSpace(http.DetectContentType(data))
	if mimeType == "" {
		mimeType = mimeFromExt(filepath.Ext(path))
	}

	return Image{
		Data:        data,
		MIMEType:    mimeType,
		Description: filepath.Base(path),
	}, nil
}

func withDefaults(values, defaults []string) []string {
	if len(values) == 0 {
		return defaults
	}
	return values
}

func normalizeExt(ext string) string {
	ext = strings.TrimSpace(ext)
	if ext == "" {
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return strings.ToLower(ext)
}

func mimeFromExt(ext string) string {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".bmp":
		return "image/bmp"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return ""
	}
}
