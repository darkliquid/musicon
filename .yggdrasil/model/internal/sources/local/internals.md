# Logic

`internal/sources/local` bridges the filesystem to Musicon's existing UI and audio seams.

Its expected shape is:

- scan one or more configured root directories for supported audio files
- rescan those roots on a bounded refresh interval so search results do not become stale during a session
- derive user-facing search metadata from filenames and best-effort local tags
- index normalized absolute paths, library-relative paths, and slash-separated path variants alongside title/artist/album metadata so path fragments can be searched directly
- attach local artwork metadata such as audio path and embedded-art bytes when available so the downstream local cover-art provider can discover sibling images next to the selected track
- resolve queued local files to the matching `beep` decoder based on file extension

The package source now also carries package-level and exported-symbol documentation so discovery, search, and resolve behavior remain readable from Go docs without re-deriving how the library bridges UI and playback contracts.

## Decisions

- Chose a single local source type that implements both search and resolve contracts so queue discovery and playback refer to the same filesystem-backed library.
- Chose best-effort tag enrichment over a hard dependency on tags because local playback should still work for plain filenames.
- Chose bounded periodic refresh over a more invasive watcher integration so the first live-library behavior stays simple, portable, and cheap between rescans.
- Chose to normalize and index multiple path representations instead of searching only the absolute path string because users commonly think in pasted filesystem fragments or library-relative paths, not only in canonical absolute paths.
- Chose multi-root local discovery over one hard-coded root because the new TOML config surface is meant to collect startup tunables and local library search paths are one of the first settings the user asked to centralize there.
