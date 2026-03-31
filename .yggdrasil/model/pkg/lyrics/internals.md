# Logic

`pkg/lyrics` holds Musicon's reusable lyrics control plane:

- request normalization
- synced/plain lyrics document modeling
- LRC parsing
- ordered provider dispatch
- persistent disk-backed caching
- concrete local-sidecar and `lrclib.net` providers

The first implementation pass is intentionally LRC-first. Local `.lrc` sidecar files should win for local tracks, and `lrclib.net` should be the primary remote provider because the user explicitly asked for synced LRC support as the main path. Additional fallback providers inspired by `lyrics-api` can be layered on later without changing the core request, cache, or UI adapter contracts.

Local lookup is intentionally simple: `LocalFileProvider` uses the normalized request's `LocalAudioPath`, swaps the existing audio extension for `.lrc`, and parses the sidecar through the shared LRC parser. Parsed tags such as `[ti:]`, `[ar:]`, and `[al:]` enrich the final document, but request metadata still backfills any missing title, artist, album, or duration fields so sidecar files can stay minimal.

Remote lookup intentionally prefers correctness over recall. `LRCLibProvider` first attempts `/api/get` only when title, artist, album, and duration are all known; that gives the narrowest match and avoids search-result ranking noise. If exact lookup is not possible or returns no usable document, the provider falls back to `/api/search`, scores candidates by synced-lyrics availability plus album/duration agreement, and still rejects anything that fails strong title or artist comparison. Artist comparison is deliberately tolerant of common collaboration separators such as `feat.`, `ft.`, `&`, and `with`, but it still requires at least one comparable normalized artist token to overlap so unrelated covers or karaoke versions do not leak through.

Caching is provider-scoped and request-derived. `CachedProvider` hashes a JSON payload containing the provider name plus the normalized request, so identical song metadata can be reused across sessions without mixing results between providers or between different versions of the same title by different artists.

Timed LRC documents now also centralize playback-position mapping in the reusable package instead of making each UI binary-search `TimedLines` independently. `Document.ActiveTimedLineIndex` returns the latest synced row whose start time is at or before the playback position, while still reporting "no active line yet" before the first timestamp. This keeps karaoke-style follow-along behavior consistent across any future TUI or alternate renderer that consumes the same lyrics package.

## Decisions

- Chose a dedicated reusable lyrics package over embedding lookup logic in `internal/ui` because provider chaining, parsing, and cache behavior should be reusable independently of Bubble Tea rendering.
- Chose local `.lrc` plus `lrclib.net` as the primary providers because the user explicitly asked for LRC support first rather than plain-text-only scraping.
- Chose to preserve synced timestamps in the reusable model even if the first UI pass only renders plain lines because timed lyrics are part of the data contract, not a rendering-only concern.
- Chose explicit provider boundaries for higher-risk unofficial sources because the user wants broader fallback options, but brittle scraping integrations should not poison the primary lyrics path.
