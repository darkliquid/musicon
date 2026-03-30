# Logic

`pkg/coverart/providers/remote` implements the remote fallback stages in the cover-art chain:

3. MusicBrainz + Cover Art Archive
4. Spotify
5. Apple Music
6. Last.fm

Each provider is independently cacheable and reusable.

## Decisions

- Split remote providers from local providers so the graph stays within node-size limits and the local-first policy remains explicit.
- Added Last.fm after the existing remote providers to extend the fallback chain without silently reordering the user-approved source priority.
- Prefer deriving album IDs from track/song/recording IDs before text search because individual tracks are more likely to carry track-level external identifiers than album-level IDs.
