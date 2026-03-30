# Public API

This node exposes a small startup-facing configuration surface:

- `Default() Config`
- `Load(path string) (Config, error)`
- `LoadDefault() (LoadResult, error)`
- `ResolvedLocalDirs() []string` on the loaded config

# Contracts

- Configuration must be TOML-backed and startup-friendly rather than interactive.
- When no config file is present, the loader should return sane defaults instead of failing startup.
- When the user explicitly points Musicon at a config file and that file is unreadable or invalid, startup must fail explicitly.
- Default config loading should layer the global XDG config first and then overlay the user XDG config on top when both are present.
- The config surface should centralize tunables that were previously scattered across env-based startup behavior, especially audio backend selection, UI startup defaults, album-art rendering mode, and local source directories.
- Album-art renderer configuration should accept a user-facing `backend` key while still tolerating the older `protocol` spelling as a compatibility alias.
- When `ui.cell_width_ratio` is omitted or non-positive, the loader should derive a startup default from `go-termimg` terminal font metrics instead of pinning a universal hardcoded ratio.
- UI configuration should remain app-owned: `internal/config` may normalize values, but `internal/ui` should still receive typed options instead of parsing TOML itself.

# Failure modes

- Invalid TOML must surface as a startup error.
- Invalid explicit config paths must surface as a startup error.
- Blank or malformed optional values should normalize back to safe defaults where possible.
