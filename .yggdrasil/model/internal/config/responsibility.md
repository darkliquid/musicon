# Responsibility

This node owns typed application configuration for Musicon.

It is responsible for:

- defining the supported TOML configuration surface for startup-time behavior
- loading configuration from a file path or the default search locations
- applying stable defaults for audio, UI, and local source settings
- expanding user-facing path shorthand such as `~/Music` before runtime wiring consumes it
- validating and normalizing config values so invalid startup settings fail explicitly

# Boundaries

This node is not responsible for:

- rendering UI directly
- parsing music metadata or scanning audio libraries
- owning playback queue state
- choosing per-screen layout behavior beyond exposing configuration values
- fetching credentials or remote provider data itself
