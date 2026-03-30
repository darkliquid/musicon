# Public API

This node exposes a Musicon-internal construction surface:

- `NewSource(Options) *Source`

The resulting type implements:

- `internal/ui.SearchService`
- `internal/audio.Resolver`

# Contracts

- Unqualified text queries should search YouTube-backed music candidates without requiring the user to paste a URL first.
- Pasted YouTube or YouTube Music URLs should resolve into queueable items, including playlist entries when the URL targets a playlist.
- Authentication should prefer cookie-based mechanisms compatible with yt-dlp, especially cookie files and browser-cookie import strings.
- Playback should materialize remote media into a deterministic cache location before decode so Musicon keeps stable seeking, duration, and replay behavior.
- Resolver output must decode into a `beep`-compatible stream and preserve title, artist, album, duration, and downstream cover-art metadata when available.

# Failure modes

- Missing yt-dlp or ffmpeg dependencies must surface as explicit source/runtime errors.
- Invalid auth configuration must surface explicitly instead of silently falling back to anonymous requests.
- Unresolvable or unavailable YouTube entries must fail clearly at search or resolve time.
