package coverart

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// This file contains cache wrappers for cover-art providers. The cache sits at
// the reusable package layer so callers can benefit from persistence without
// reimplementing provider-specific storage policy.

// Cache stores successful cover-art results for reuse.
type Cache interface {
	Get(key string) (Image, error)
	Put(key string, image Image) error
}

// DiskCache stores cover-art images as JSON payloads in a directory.
type DiskCache struct {
	Dir string
}

// NewDiskCache constructs a disk cache rooted at dir.
func NewDiskCache(dir string) *DiskCache {
	return &DiskCache{Dir: dir}
}

// Get loads a cached image for key.
func (c *DiskCache) Get(key string) (Image, error) {
	if c == nil || c.Dir == "" {
		return Image{}, ErrNotFound
	}
	data, err := os.ReadFile(c.pathFor(key))
	if err != nil {
		if os.IsNotExist(err) {
			return Image{}, ErrNotFound
		}
		return Image{}, err
	}
	var image Image
	if err := json.Unmarshal(data, &image); err != nil {
		return Image{}, err
	}
	if len(image.Data) == 0 {
		return Image{}, ErrNotFound
	}
	return image, nil
}

// Put stores image in the cache under key.
func (c *DiskCache) Put(key string, image Image) error {
	if c == nil || c.Dir == "" {
		return nil
	}
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(image)
	if err != nil {
		return err
	}
	return os.WriteFile(c.pathFor(key), data, 0o644)
}

func (c *DiskCache) pathFor(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(c.Dir, hex.EncodeToString(sum[:])+".json")
}

// CachedProvider wraps a provider with cache reads/writes.
type CachedProvider struct {
	Provider Provider
	Cache    Cache
}

// NewCachedProvider wraps provider with cache support. If either input is nil,
// the original provider is returned unchanged.
func NewCachedProvider(provider Provider, cache Cache) Provider {
	if provider == nil || cache == nil {
		return provider
	}
	return CachedProvider{Provider: provider, Cache: cache}
}

// Name returns the wrapped provider's stable identifier.
func (p CachedProvider) Name() string {
	if p.Provider == nil {
		return ""
	}
	return p.Provider.Name()
}

// Lookup consults the cache before delegating to the wrapped provider.
func (p CachedProvider) Lookup(ctx context.Context, metadata Metadata) (Result, error) {
	return p.LookupObserved(ctx, metadata, nil)
}

// LookupObserved consults the cache before delegating to the wrapped provider and reports cache/provider events.
func (p CachedProvider) LookupObserved(ctx context.Context, metadata Metadata, report func(AttemptEvent)) (Result, error) {
	if p.Provider == nil {
		return Result{}, ErrNotFound
	}
	key, err := cacheKey(p.Provider.Name(), metadata.Normalize())
	if err != nil {
		reportAttempt(report, AttemptEvent{Provider: p.Provider.Name(), Status: AttemptError, Message: err.Error()})
		return Result{}, err
	}
	if image, err := p.Cache.Get(key); err == nil {
		reportAttempt(report, AttemptEvent{Provider: p.Provider.Name(), Status: AttemptCacheHit, Message: "loaded from disk cache"})
		return Result{Image: image, Provider: p.Provider.Name()}, nil
	} else if !IsNotFound(err) {
		reportAttempt(report, AttemptEvent{Provider: p.Provider.Name(), Status: AttemptError, Message: err.Error()})
		return Result{}, err
	}
	reportAttempt(report, AttemptEvent{Provider: p.Provider.Name(), Status: AttemptCacheMiss, Message: "cache miss"})

	result, err := lookupProviderObserved(ctx, p.Provider, metadata, report)
	if err != nil {
		return Result{}, err
	}
	if err := p.Cache.Put(key, result.Image); err != nil {
		reportAttempt(report, AttemptEvent{Provider: p.Provider.Name(), Status: AttemptError, Message: err.Error()})
		return Result{}, err
	}
	return result, nil
}

func cacheKey(provider string, metadata Metadata) (string, error) {
	payload := struct {
		Provider string   `json:"provider"`
		Metadata Metadata `json:"metadata"`
	}{
		Provider: provider,
		Metadata: cacheableMetadata(metadata),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func cacheableMetadata(metadata Metadata) Metadata {
	metadata = metadata.Normalize()
	metadata.Local = nil
	return metadata
}
