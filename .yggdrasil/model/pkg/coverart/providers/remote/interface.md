# Public API

This node exposes:

- `MusicBrainzProvider` / `NewMusicBrainzProvider(...)`
- `SpotifyProvider`
- `AppleMusicProvider`
- `LastFMProvider`

# Contracts

- Remote providers must preserve the chain order chosen by callers.
- Remote providers return `ErrNotFound` for misses and degrade cleanly when credentials are unavailable.
- Where supported, remote providers should derive album IDs from track/song/recording IDs before falling back to text search.
