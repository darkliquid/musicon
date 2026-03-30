# Logic

`internal/sources/youtube` bridges yt-dlp metadata and downloads into Musicon's queue and playback seams.

Its expected shape is:

- run yt-dlp searches for free-text queries and JSON inspection for pasted URLs
- flatten pasted playlist URLs into queueable track entries rather than forcing the UI to understand playlist expansion
- map yt-dlp metadata into Musicon search results and playback track info
- lazily ensure yt-dlp is available before provider actions and ensure ffmpeg is available before audio extraction
- cache resolved audio files under a deterministic path so repeated playback does not require a fresh download every time

## Decisions

- Chose a dedicated `internal/sources/youtube` node over extending `internal/sources/local` because the provider has different auth, metadata, and playback behavior from local filesystem discovery.
- Chose cookie-file and browser-cookie auth over username/password fields because the user asked for private playlist and uploaded-music access, and yt-dlp supports cookie-based auth directly.
- Chose cached-file playback over piping remote stdout directly into decoders because Musicon's playback runtime expects stable seek and duration behavior.
- Chose to use yt-dlp under the hood because the user personally uses YouTube Music and wants a popular streaming source integrated into the same queue/playback workflow.
