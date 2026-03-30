# Responsibility

This node owns the executable entrypoint for Musicon.

It is responsible for creating the root UI application, starting the Bubble Tea program, and surfacing startup failures.

It also owns optional runtime wiring that should exist outside the TUI itself, including starting the desktop MPRIS bridge and degrading cleanly when that integration is unavailable.

# Boundaries

This node is not responsible for queue management logic, playback-screen rendering, component styling, or implementing source, audio, or D-Bus runtime internals.

Those concerns belong to `internal/ui` and the contracts it exposes.
