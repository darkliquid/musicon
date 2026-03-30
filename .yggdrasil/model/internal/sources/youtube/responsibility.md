# Responsibility

This node owns the concrete YouTube and YouTube Music source backed by direct HTTP search plus `youtube/v2` metadata inspection and `yt-dlp`-assisted playback resolution.

It is responsible for:

- exposing YouTube-backed search results to queue management
- interpreting pasted YouTube and YouTube Music URLs, including public playlist URLs
- resolving queued YouTube entries into playable pure-Go decoded audio streams
- translating YouTube Music and `youtube/v2` metadata into Musicon search, queue, playback, and cover-art metadata
- using `yt-dlp -j` to extract a direct media URL plus HTTP headers, then reading that WebM/Opus media through Musicon's own ranged HTTP reader into a buffered seekable `beep` stream without routing playback bytes through ffmpeg or a cgo decoder

# Boundaries

This node is not responsible for:

- rendering queue or playback UI
- outputting audio to speakers
- implementing reusable cover-art resolution providers
- managing TOML parsing or startup configuration discovery
