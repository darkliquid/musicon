# Logic

`pkg/coverart/core` holds the reusable control plane for cover-art resolution:

- metadata normalization
- metadata merging
- ordered provider dispatch
- explicit not-found semantics
- disk-backed caching wrappers

This node exists so other consumers can reuse the resolution primitives without inheriting every concrete provider implementation.

## Decisions

- Split core contracts from concrete providers because the package grew beyond the node-size guideline and the abstractions are reusable independently.
- Kept cache logic with the resolution core because cache policy composes around providers rather than belonging to any one source.
- Kept metadata merge rules in the reusable core so queue items, playback snapshots, and future source integrations preserve artwork identifiers and local hints consistently.
