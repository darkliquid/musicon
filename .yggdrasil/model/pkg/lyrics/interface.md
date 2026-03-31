# Public API

This node is expected to expose:

- normalized lyrics request and document types
- provider and chain contracts for reusable lyrics lookup
- LRC parsing helpers and synced-line representations
- disk-backed caching helpers for successful lyrics results
- concrete primary providers for local `.lrc` discovery and `lrclib.net`

# Contracts

- Requests must be source-agnostic and safe to derive from playback metadata.
- Request normalization must trim noisy metadata and carry the local audio path when available so local sidecar lookup can reuse existing playback metadata instead of requiring a separate file-discovery seam.
- The reusable model must preserve synced LRC timestamps even if callers choose to render only plain lines.
- Documents must expose a plain display projection (`DisplayLines`) so basic UIs can render one stable text view without discarding richer synced data.
- Providers must treat strong title+artist matching as the minimum success bar; weak or ambiguous matches should return not-found instead of likely-wrong lyrics.
- Local `.lrc` sidecars and `lrclib.net` are the primary providers for the first implementation pass.
- Persistent cache identity should derive from normalized song metadata rather than transient UI state.
- The remote `lrclib.net` path should try exact `/get` lookup when title, artist, album, and duration are all known, then fall back to `/search` with the same strong-match gate.

# Failure modes

- Empty or insufficient request metadata should resolve as not-found rather than panicking.
- Missing sidecar files or unmatched remote responses should degrade to not-found so later fallback providers can still run.
- Cache misses or empty cached documents should degrade to not-found so the wrapped provider can still run.
- Hard provider or cache failures should remain explicit to callers.
