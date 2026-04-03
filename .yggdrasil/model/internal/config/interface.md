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
- Default config loading should resolve to the user's XDG config file path (`$XDG_CONFIG_HOME/musicon/config.toml` or `~/.config/musicon/config.toml`) and fall back to built-in defaults when that file is absent.
- The loader should still honor explicit config paths from higher-precedence startup inputs such as `MUSICON_CONFIG` or an app-layer CLI flag.
- The config surface should centralize tunables that were previously scattered across env-based startup behavior, especially audio backend selection, UI startup defaults such as compact mode, semantic UI theme colors, album-art rendering mode, local source directories, and YouTube source auth/cache settings.
- The config surface should also centralize configurable UI keybindings under `[keybinds]`, covering global shell actions such as mode/help/compact toggles plus queue and playback screen shortcuts.
- Album-art renderer configuration should accept a user-facing `backend` key while still tolerating the older `protocol` spelling as a compatibility alias.
- UI theme configuration should accept semantic color roles inline under `[ui.theme]` and may also load those same roles from an external TOML theme file, with inline values overriding file-provided values when both are present.
- The loader should also keep accepting the legacy `theme = "default"` string form as a compatibility alias for the built-in palette so existing user configs do not break at startup.
- YouTube source configuration should support cookie-file auth, browser-cookie import, bounded search-result counts, deterministic cache locations, and advanced raw yt-dlp arguments without pushing TOML parsing into the source implementation.
- When `ui.cell_width_ratio` is omitted or non-positive, the loader should fall back to the shared fixed ratio used by `pkg/components` instead of attempting terminal font-metric auto-detection.
- UI configuration should remain app-owned: `internal/config` may normalize values, but `internal/ui` should still receive typed options instead of parsing TOML itself.

# Failure modes

- Invalid TOML must surface as a startup error.
- Invalid explicit config paths must surface as a startup error.
- Invalid UI theme colors or unreadable explicit theme files must surface as startup errors.
- Blank or malformed optional values should normalize back to safe defaults where possible.
