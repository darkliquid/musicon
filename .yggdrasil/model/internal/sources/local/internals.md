# Logic

`internal/sources/local` bridges the filesystem to Musicon's existing UI and audio seams.

Its expected shape is:

- scan a configured root directory for supported audio files
- derive user-facing search metadata from filenames and best-effort local tags
- attach local artwork metadata such as audio path and embedded-art bytes when available
- resolve queued local files to the matching `beep` decoder based on file extension

## Decisions

- Chose a single local source type that implements both search and resolve contracts so queue discovery and playback refer to the same filesystem-backed library.
- Chose best-effort tag enrichment over a hard dependency on tags because local playback should still work for plain filenames.
