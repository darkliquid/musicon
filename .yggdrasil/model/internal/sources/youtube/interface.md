# Public API

This node exposes a Musicon-internal construction surface:

- `NewSource(Options) *Source`

The resulting type implements:

- `internal/ui.SearchService`
- `internal/audio.Resolver`

# Contracts

- Unqualified text queries should search YouTube Music candidates through the lightweight music.youtube.com HTTP API without requiring the user to paste a URL first.
- Pasted YouTube or YouTube Music URLs should resolve into queueable items, including flattened public playlist entries when the URL targets a playlist.
- Search should honor caller cancellation so the queue UI can abandon superseded YouTube Music HTTP requests instead of leaving stale searches running.
- Resolver output should use `yt-dlp` for extraction, fetch playback bytes through Musicon's own ranged HTTP reader, decode the resulting WebM/Opus bytes in pure Go into a buffered seekable stream, and return a `beep`-compatible stream with title, artist, album, duration, and downstream cover-art metadata preserved.

# Failure modes

- `yt-dlp` extraction failures, direct ranged media request failures, initial-buffer timeouts, unsupported out-of-window seek attempts, cue-based startup seek failures, or WebM/Opus decode failures must surface as explicit source/runtime errors.
- Unresolvable or unavailable YouTube entries must fail clearly at search or resolve time.
