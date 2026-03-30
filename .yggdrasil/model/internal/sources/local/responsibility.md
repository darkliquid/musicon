# Responsibility

This node owns the concrete local-file music source.

It is responsible for:

- recursively discovering supported local audio files
- exposing local-file search results to queue management
- resolving queued local files into playable decoded streams
- extracting local track metadata that can feed downstream artwork lookup

# Boundaries

This node is not responsible for:

- rendering queue or playback UI
- outputting audio to speakers
- directly fetching artwork from local or remote providers
