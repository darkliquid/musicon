# Responsibility

This node owns the executable entrypoint for Musicon.

It is responsible for creating the root UI application, starting the Bubble Tea program, and surfacing startup failures.

# Boundaries

This node is not responsible for queue management logic, playback-screen rendering, component styling, or source and playback backends.

Those concerns belong to `internal/ui` and the contracts it exposes.
