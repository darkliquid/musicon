# Public API

This node exposes:

- `Metadata`, `IDs`, and `LocalMetadata`
- `Image` and `Result`
- `Provider` and `Chain`
- `Cache`, `DiskCache`, `CachedProvider`, and `NewCachedProvider(...)`
- `ErrNotFound` and `IsNotFound(...)`

# Contracts

- Metadata must be reusable and renderer-agnostic.
- IDs must support both album-level and track-level external identifiers so callers can resolve album artwork even when only per-track metadata is available.
- The chain preserves provider priority and miss-vs-failure semantics.
- Cache wrappers may be composed around providers without changing caller contracts.

# Failure modes

- Empty metadata resolves as not-found rather than panicking.
- Hard provider or cache failures surface explicitly.
