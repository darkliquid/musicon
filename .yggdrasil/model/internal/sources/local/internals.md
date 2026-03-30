# Logic

`internal/sources/local` bridges the filesystem to Musicon's existing UI and audio seams.

Its expected shape is:

- scan a configured root directory for supported audio files
- rescan that root on a bounded refresh interval so search results do not become stale during a session
- derive user-facing search metadata from filenames and best-effort local tags
- index normalized absolute paths, library-relative paths, and slash-separated path variants alongside title/artist/album metadata so path fragments can be searched directly
- attach local artwork metadata such as audio path and embedded-art bytes when available
- resolve queued local files to the matching `beep` decoder based on file extension

## Decisions

- Chose a single local source type that implements both search and resolve contracts so queue discovery and playback refer to the same filesystem-backed library.
- Chose best-effort tag enrichment over a hard dependency on tags because local playback should still work for plain filenames.
- Chose bounded periodic refresh over a more invasive watcher integration so the first live-library behavior stays simple, portable, and cheap between rescans.
- Chose to normalize and index multiple path representations instead of searching only the absolute path string because users commonly think in pasted filesystem fragments or library-relative paths, not only in canonical absolute paths.
