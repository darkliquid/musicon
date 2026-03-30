# Responsibility

This node owns the concrete YouTube and YouTube Music source backed by `yt-dlp`.

It is responsible for:

- exposing YouTube-backed search results to queue management
- interpreting pasted YouTube and YouTube Music URLs, including authenticated playlist URLs
- resolving queued YouTube entries into playable cached audio files
- translating yt-dlp metadata into Musicon search, queue, playback, and cover-art metadata
- honoring optional cookie-based authentication so users can access private playlists and uploaded music

# Boundaries

This node is not responsible for:

- rendering queue or playback UI
- outputting audio to speakers
- implementing reusable cover-art resolution providers
- managing TOML parsing or startup configuration discovery
