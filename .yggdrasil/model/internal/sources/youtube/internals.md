# Logic

`internal/sources/youtube` splits search from playback: free-text search uses the YouTube Music HTTP API for low-latency results, while pasted-URL inspection relies on `youtube/v2` and playback uses `yt-dlp` as an extractor before decoding ranged WebM reads into a buffered in-process streamer.

The implementation is now physically split by responsibility as well: `source.go` holds the top-level source surface, `search.go` owns YouTube Music search and URL-inspection helpers, `media.go` owns yt-dlp extraction plus ranged media IO, and `stream.go` owns WebM/Opus decode and streamer implementations.

The package source now also carries richer inline documentation around the non-obvious parts of that pipeline: why search avoids yt-dlp, why playback uses yt-dlp only as an extractor, how ranged reads emulate `io.ReadSeeker`, and how the bounded PCM window maps decoded packets into beep's streaming contract.

Its expected shape is:

- call the YouTube Music search endpoint for free-text queries and use `youtube/v2` for pasted video or playlist inspection
- treat YouTube Music search shelves as playable whenever an item exposes a `videoId`, instead of assuming the API always labels sections as "Songs" or "Videos"
- flatten pasted playlist URLs into queueable track entries rather than forcing the UI to understand playlist expansion
- map YouTube Music search metadata and `youtube/v2` metadata into Musicon search results and playback track info
- honor caller cancellation in search so stale queue queries can terminate superseded HTTP requests promptly
- inspect YouTube metadata with `youtube/v2`, then call `yt-dlp -j` to obtain the final media URL and request headers
- read the selected WebM media through a custom HTTP range-backed `io.ReadSeeker` instead of streaming audio bytes over `yt-dlp` stdout
- persist fetched range blocks in a per-stream temp-directory cache so revisiting an earlier aligned byte range can reuse already-downloaded media locally instead of forcing another network request
- hand that ranged reader to the WebM parser so cue-based seeking can reopen near the target time without requiring ffmpeg
- keep playback on its own cancellable stream context so the resolver timeout only bounds metadata/extraction and does not cancel long-lived playback reads after `Resolve` returns
- support cheap seeks only inside the currently retained PCM window; if a seek falls outside that buffered window, fail it immediately and leave playback state unchanged instead of entering any long-lived pending seek or replacement-stream state
- decode Opus packets into a fixed-size PCM window with a retained back-buffer and forward read-ahead, allowing cheap seeks inside the window and controlled rebuffering for farther seeks
- clear the per-stream block cache when the transport closes so disk-backed reuse only survives for the lifetime of one active resolved stream

## Decisions

- Chose a dedicated `internal/sources/youtube` node over extending `internal/sources/local` because the provider has different auth, metadata, and playback behavior from local filesystem discovery.
- Chose to replace yt-dlp-based free-text search with direct YouTube Music HTTP requests after comparing Qusic's approach, because shelling out to yt-dlp remained much slower than the queue UX could tolerate.
- Chose to replace yt-dlp-based resolution with `youtube/v2` plus pure-Go WebM/Opus decode because the user explicitly asked to drop the old yt-dlp package and avoid cgo-backed Opus decoders.
- Chose to decode the full track into a seekable PCM buffer before handing it to the audio engine because Musicon's playback runtime expects a `beep.StreamSeekCloser` with reliable `Len`, `Position`, and `Seek` behavior.
- Chose to make `yt-dlp` the default playback fetch path after observing live `youtube/v2` audio URLs consistently returning HTTP 403, because the direct media path added complexity without delivering reliable playback while `yt-dlp` still produced usable media URLs and request headers.
- Chose a buffered background decode model over full-track upfront decode because the user reported sluggish playback startup and a small prebuffer preserves quicker start-up while keeping Musicon's existing seek-oriented `beep.StreamSeekCloser` contract.
- Chose `yt-dlp -j` plus direct HTTP range requests over piping media bytes through `yt-dlp` stdout because the extractor already exposes the final media URL and headers, ranged reads avoid the sluggish long-lived stdout transport, and WebM cue-based seeking becomes practical without reintroducing ffmpeg.
- Chose a per-stream on-disk range-block cache over the previous single in-memory block because the user explicitly wanted repeated reseeks to reuse previously downloaded ranges instead of invalidating old blocks and triggering fresh HTTP range downloads; the cache is still cleared on `Close` so stream lifetime remains the cleanup boundary.
