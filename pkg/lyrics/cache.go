package lyrics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// Cache stores successful lyrics results for reuse.
type Cache interface {
	Get(string) (Document, error)
	Put(string, Document) error
}

// DiskCache stores lyrics documents as JSON payloads in a directory.
type DiskCache struct {
	Dir string
}

// NewDiskCache constructs a disk cache rooted at dir.
func NewDiskCache(dir string) *DiskCache {
	return &DiskCache{Dir: dir}
}

// Get loads a cached document for key.
func (c *DiskCache) Get(key string) (Document, error) {
	if c == nil || c.Dir == "" {
		return Document{}, ErrNotFound
	}
	data, err := os.ReadFile(c.pathFor(key))
	if err != nil {
		if os.IsNotExist(err) {
			return Document{}, ErrNotFound
		}
		return Document{}, err
	}
	var document Document
	if err := json.Unmarshal(data, &document); err != nil {
		return Document{}, err
	}
	if document.Empty() {
		return Document{}, ErrNotFound
	}
	return document, nil
}

// Put stores document in the cache under key.
func (c *DiskCache) Put(key string, document Document) error {
	if c == nil || c.Dir == "" {
		return nil
	}
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(document)
	if err != nil {
		return err
	}
	return os.WriteFile(c.pathFor(key), data, 0o644)
}

func (c *DiskCache) pathFor(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(c.Dir, hex.EncodeToString(sum[:])+".json")
}

// CachedProvider wraps a provider with cache reads and writes.
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
func (p CachedProvider) Lookup(ctx context.Context, request Request) (Document, error) {
	if p.Provider == nil {
		return Document{}, ErrNotFound
	}
	key, err := cacheKey(p.Provider.Name(), request.Normalize())
	if err != nil {
		return Document{}, err
	}
	if document, err := p.Cache.Get(key); err == nil {
		return document, nil
	} else if !IsNotFound(err) {
		return Document{}, err
	}
	document, err := p.Provider.Lookup(ctx, request)
	if err != nil {
		return Document{}, err
	}
	if document.Empty() {
		return Document{}, ErrNotFound
	}
	if err := p.Cache.Put(key, document); err != nil {
		return Document{}, err
	}
	return document, nil
}

func cacheKey(provider string, request Request) (string, error) {
	payload := struct {
		Provider string  `json:"provider"`
		Request  Request `json:"request"`
	}{
		Provider: provider,
		Request:  request.Normalize(),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
