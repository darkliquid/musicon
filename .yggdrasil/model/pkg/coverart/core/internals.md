# Logic

`pkg/coverart/core` holds the reusable control plane for cover-art resolution:

- metadata normalization
- metadata merging
- metadata-supplied remote-artwork URL fetching
- ordered provider dispatch
- optional observed provider-attempt reporting
- explicit not-found semantics
- disk-backed caching wrappers

The chain can now execute in either a quiet mode (`Resolve`) or an observed mode that reports cache hits, cache misses, provider starts, misses, success, and hard failures as structured attempt events. Cache wrappers preserve the wrapped provider's public identity while still reporting whether the result came from disk cache or from the underlying provider lookup. Remote-cache keys now intentionally ignore `LocalMetadata` so artwork already found through a remote provider can be reused later from normalized title/album/artist/external-ID data without requiring the same local file path or embedded-art hints to be present. The reusable core also includes a lightweight `MetadataURLProvider` fast path for sources that already know a concrete artwork URL (for example, search responses that include thumbnails); callers can place that provider before heavier cross-service lookup providers and still reuse the same cache/observation machinery.

The package source now also carries package-level and exported-symbol documentation so reusable metadata, cache, and chain contracts remain understandable from Go tooling without reopening every provider implementation.

This node exists so other consumers can reuse the resolution primitives without inheriting every concrete provider implementation.

## Decisions

- Split core contracts from concrete providers because the package grew beyond the node-size guideline and the abstractions are reusable independently.
- Kept cache logic with the resolution core because cache policy composes around providers rather than belonging to any one source.
- Kept metadata merge rules in the reusable core so queue items, playback snapshots, and future source integrations preserve artwork identifiers and local hints consistently.
- Chose observed resolution events in the reusable core over a UI-only logging shim because cache hits/misses and provider outcomes belong to the resolution pipeline itself, not to any one renderer.
- Chose a reusable metadata-URL provider over a YouTube-specific artwork special case because direct thumbnail URLs are a generic capability that other sources can reuse without depending on YouTube parsing code.
