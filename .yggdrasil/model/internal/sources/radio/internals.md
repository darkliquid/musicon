# Logic

`internal/sources/radio` splits discovery from playback:

- search uses Radio Browser station endpoints to find candidates and normalize them into Musicon search rows
- playback resolves the selected station through Radio Browser's click-count endpoint to obtain the actual stream URL
- direct MP3/Ogg/Vorbis/WAV streams use in-process decode where possible
- HLS stations use `gohlslib` for playlist / segment handling, then decode supported audio tracks in-process
- HLS MPEG-TS AAC stations that fail under `gohlslib`'s generic leading-track handling use a source-owned pure-Go fallback that polls the media playlist, downloads `.ts` segments directly, extracts ADTS-framed AAC out of PES payloads with `astits`, and decodes frames with `go-aac`
- raw ADTS AAC stations use `go-aac` directly without shelling out to external tools
- live stream opener contexts must stay alive for the full playback lifetime and only be canceled when the returned stream closes; startup timeouts are only for short-lived Radio Browser discovery calls
- HLS transport-level playlist / segment / decode diagnostics are routed through the app-owned debug sink instead of `gohlslib`'s default stdout logger so the TUI surface stays clean during playback

The implementation is expected to keep radio playback modeled as live, open-ended audio:

- queue rows remain `MediaStream`
- playback duration remains open-ended
- the returned stream must reject seeks
- native stream selection remains an implementation detail behind the resolver contract, not a UI concern
- the live decode path keeps a bounded PCM buffer in memory and presents a non-seekable `beep.StreamSeekCloser`

## Constraints

- Radio Browser marks stations with `hls=1` when they use HTTP Live Streaming playlists rather than a single direct audio stream.
- Radio Browser response booleans are not consistently encoded; click responses can use JSON booleans while station-search payloads may use numeric or string-like truthy values.
- HLS support is not just one more codec; it implies playlist parsing, segment fetching, and buffering before audio reaches the playback engine.
- Some HLS AAC transports emit occasional boundary-corrupted access units even while subsequent access units decode correctly, so the live decoder must tolerate syncword / frame-length garbage around segment edges instead of aborting playback on the first bad access unit.
- Musicon's audio engine expects a `beep.StreamSeekCloser`, so any native stream path still has to present a live stream that behaves sensibly under `Len`, `Position`, `Seek`, and `Close`.
- If the resolver ties the live stream opener to a short resolve timeout, otherwise-healthy radio playback will start decoding and then terminate with `context canceled` shortly after startup.
- `go-aac` expects ADTS-framed AAC, so HLS MPEG-4 Audio access units must be wrapped into ADTS packets before decode.

## Decisions

- Chose a dedicated `internal/sources/radio` node over folding internet radio into `internal/sources/local` or `internal/sources/youtube` because radio search, station metadata, and live-stream playback have distinct external dependencies and failure modes.
- Chose to record the product rationale as "if we are going to support internet radio, we should support all of it" because this requirement came directly from the user and explains why HLS support matters beyond a narrow codec checklist.
- Chose to preserve direct in-process decoding for simple radio streams while adding a native Go HLS / AAC pipeline because direct decode keeps the common path small and dependency-light, while the native path expands compatibility toward the user's "support all of it" goal without depending on `ffmpeg`.
- Rejected keeping the existing `hls=1` filter once broader playback is available because that would continue hiding a large class of stations from users after the product decision was to support internet radio comprehensively.
- Rejected keeping the `ffmpeg` transcoder fallback because the user reported it was not working reliably and explicitly requested a native `gohlslib`-based implementation instead.
