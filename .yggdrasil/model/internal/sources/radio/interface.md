# Public API

This node exposes a Musicon-internal construction surface:

- `NewSource(Options) *Source`

The resulting type implements:

- `internal/ui.SearchService`
- `internal/audio.Resolver`

# Contracts

- Free-text queries should search Radio Browser for internet radio stations without requiring pasted stream URLs.
- Search should honor caller cancellation so the queue UI can abandon superseded station searches promptly.
- Search results should preserve station identity, source metadata, and favicon-derived artwork metadata for downstream playback and artwork rendering.
- Search should not exclude HLS-backed stations solely because they are HLS when the source has a viable playback path for them.
- Resolver output should return a `beep`-compatible live stream with non-seekable behavior, preserving station title, subtitle, source, and artwork metadata.
- When a station uses a format Musicon can decode directly in-process, the resolver should prefer that simpler path.
- When a station uses HLS AAC / Opus or raw ADTS AAC that Musicon cannot decode through the direct MP3/Ogg/WAV path, the resolver should use the native Go streaming pipeline and still behave as a live stream.

# Failure modes

- Radio Browser lookup failures, click-resolution failures, direct HTTP stream request failures, unsupported-codec failures, or native HLS/AAC pipeline startup failures must surface as explicit source/runtime errors.
- Live radio streams must reject seek attempts clearly instead of pretending to support random access.
