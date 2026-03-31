# Responsibility

This node owns Musicon's reusable lyrics-resolution layer.

It is responsible for normalized lyrics lookup requests, LRC parsing, provider chaining, persistent cache behavior, and concrete provider implementations that can be wired into the UI without forcing playback code to know about HTTP APIs or local sidecar-file discovery.

# Boundaries

This node is not responsible for Bubble Tea rendering, pane switching, or direct playback-screen state management.

Those concerns belong to `internal/ui` and the application wiring in `app/cli`.
