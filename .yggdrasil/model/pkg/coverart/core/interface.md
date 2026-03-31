# Public API

This node exposes:

- `Metadata`, `IDs`, and `LocalMetadata`
- `Image` and `Result`
- `Provider`, `MetadataURLProvider`, and `Chain`
- `AttemptEvent`, observed chain/provider resolution, and provider-attempt status reporting
- `Cache`, `DiskCache`, `CachedProvider`, and `NewCachedProvider(...)`
- `ErrNotFound` and `IsNotFound(...)`

# Contracts

- Metadata must be reusable and renderer-agnostic.
- IDs must support both album-level and track-level external identifiers so callers can resolve album artwork even when only per-track metadata is available.
- Metadata must support safe merging so queue/runtime/display layers can preserve local paths, embedded-art hints, direct remote artwork URLs, and external IDs without each layer reimplementing merge logic.
- The chain preserves provider priority and miss-vs-failure semantics.
- Cache wrappers may be composed around providers without changing caller contracts.
- `MetadataURLProvider` must treat metadata-supplied artwork URLs as an optional fast path, returning `ErrNotFound` when no URL is present so callers can place it before slower fallback providers.
- Cache identity for remote lookups should be derived from reusable track metadata rather than local-only file hints so later sessions can reuse remote artwork by normalized song/album/artist/ID information alone.

# Failure modes

- Empty metadata resolves as not-found rather than panicking.
- Hard provider or cache failures surface explicitly.
