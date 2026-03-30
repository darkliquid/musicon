# Responsibility

This node owns concrete remote-service cover-art providers.

It is responsible for:

- MusicBrainz and Cover Art Archive lookups
- Spotify artwork lookup
- Apple Music artwork lookup
- Last.fm artwork lookup

# Boundaries

This node is not responsible for:

- local file discovery
- terminal rendering
- Musicon playback orchestration
