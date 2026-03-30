# Public API

The node should expose a small lifecycle-oriented surface:

- a constructor that accepts playback control/status dependencies
- a start or export step that claims the MPRIS bus name and object path
- a close step that releases D-Bus resources cleanly
- exported MPRIS root and player methods for transport control and property-backed desktop integration

# Contracts

- The bridge should depend on injected playback contracts rather than importing concrete UI logic.
- MPRIS property updates should tolerate missing track metadata and idle playback.
- Desktop transport requests must map to Musicon playback controls without blocking the terminal UI loop.
- Writable MPRIS properties such as loop status and volume must flow back into the same playback runtime used by the TUI.

# Failure modes

- Session-bus connection failures must surface as explicit errors.
- Export/name-claim failures should not silently pretend MPRIS is active.
- Shutdown must release any claimed bus name and stop background update work.
