# Public API

The node should expose a small runtime-facing construction surface to the rest of the app:

- a constructor for the playback engine/service
- queue and playback service adapters that satisfy the contracts defined in `internal/ui`
- a way to resolve queue entries into playable streams through an injected resolver

# Contracts

- The runtime should depend on an injected resolver rather than embedding source-specific loading logic.
- Queue and playback contracts should be implemented through thin adapters over a shared engine so both services can share state without forcing incompatible method signatures onto one exported type.
- Playback state returned to `internal/ui` must be snapshot-friendly and safe to poll frequently.
- Resolved track metadata should be rich enough for downstream artwork sourcing to use album, artist, external IDs, and local-file context when available.
- The runtime should tolerate an empty queue or unresolvable entries without crashing the UI.

# Failure modes

- Speaker initialization failures must surface as explicit errors.
- Decoder/resolver failures should stop or skip playback cleanly and surface status back to callers.
- Closing the runtime must release active stream and speaker resources.
