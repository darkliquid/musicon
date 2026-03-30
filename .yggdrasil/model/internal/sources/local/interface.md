# Public API

This node exposes a Musicon-internal construction surface:

- `NewLibrary(root string) *Library`

The resulting type implements:

- `internal/ui.SearchService`
- `internal/audio.Resolver`

# Contracts

- Local discovery must stay internal to the source layer rather than leaking filesystem traversal into the UI or audio runtime.
- Local discovery should refresh over time so newly added or removed files can appear without restarting Musicon.
- Search results must preserve local file paths and best-effort embedded-art metadata for downstream cover-art resolution, so sibling artwork can still be found when playback begins.
- Local discovery searches should match path-style queries against absolute and library-relative file paths as well as title/artist/album metadata, so users can paste or type filesystem fragments directly.
- Resolver output must decode supported local files into playable `beep` streams and provide track info rich enough for the playback UI.

# Failure modes

- Missing or unreadable roots should surface search or resolve errors explicitly.
- Unsupported file extensions must fail clearly at resolve time.
- Decode errors must surface through the resolver contract rather than crashing the runtime.
