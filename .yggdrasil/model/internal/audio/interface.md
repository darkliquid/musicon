# Public API

The node should expose a small runtime-facing construction surface to the rest of the app:

- a way to list config-compatible backend names that are usable on the current machine
- a way to canonicalize configured backend names so aliases can be compared against listed backend labels
- a constructor for the playback engine/service
- a restore surface that seeds the runtime with a previously saved queue and playback snapshot without auto-starting audio
- queue, playback, and visualization service adapters that satisfy the contracts defined in `internal/ui`
- a way to resolve queue entries into playable streams through an injected resolver
- playback options that can include a requested output backend name

# Contracts

- The runtime should depend on an injected resolver rather than embedding source-specific loading logic.
- Backend enumeration should produce canonical names that are valid in config files, not internal enum labels or user-hostile debug identifiers.
- Queue and playback contracts should be implemented through thin adapters over a shared engine so both services can share state without forcing incompatible method signatures onto one exported type.
- The runtime should be able to restore queue order, current queue index, repeat/stream flags, volume, and a caller-supplied playback snapshot so the UI can reopen in the same context without making audio playback start automatically.
- Playback state returned to `internal/ui` must be snapshot-friendly and safe to poll frequently.
- Visualization state returned to `internal/ui` must also be safe to poll frequently and cheap to render during ordinary TUI redraws.
- The runtime should accept a normalized backend selection from application config and use it when initializing mago playback, while treating `auto` as the default backend policy.
- Queue adapters must support moving an existing queued item by a relative delta so the UI can reorder pinned rows without rebuilding queue state manually.
- Resolved track metadata should be rich enough for downstream artwork sourcing to use album, artist, external IDs, and local-file context when available.
- When queue items already carry artwork metadata, the runtime should preserve and merge that metadata into resolved track info instead of discarding source-derived local paths or IDs.
- Live EQ/visualizer output should stay inside the existing Go audio pipeline instead of requiring an external `cava`/`cavacore` process, so runtime wiring and deployment stay simple.
- The runtime should tolerate an empty queue or unresolvable entries without crashing the UI.

# Failure modes

- Speaker initialization failures must surface as explicit errors.
- Decoder/resolver failures should stop or skip playback cleanly and surface status back to callers.
- Closing the runtime must release active stream and speaker resources.
- Visualization surfaces must degrade to empty output when no track is active instead of crashing the playback screen.
