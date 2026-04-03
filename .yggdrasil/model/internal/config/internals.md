# Logic

`internal/config` gives Musicon one typed place to describe startup tunables that affect runtime wiring.

The expected shape is:

- look for an explicit config path first
- otherwise resolve the user's XDG config file path and load that single file when it exists
- start from code defaults, then overlay TOML values
- normalize values such as audio backend names, semantic UI theme colors, image-renderer backend names, start mode, compact-mode preference, fill mode, local directories, configurable keybinding lists, and YouTube source paths/args
- when a config file defines `ui.theme.file`, resolve the path relative to the config file that declared it, decode the external TOML palette, and then let any inline `[ui.theme]` role values override the external palette
- if a config still uses the legacy scalar `theme = "..."` form under `[ui]`, treat it as a compatibility alias for the built-in default palette instead of failing TOML decoding
- carry global compact-mode toggles plus queue-mode bindings for source switching, visible search-kind slot selection, search-kind cycling, and collection expansion so the richer multi-source queue workflow stays configurable from TOML instead of hardcoded in the UI
- apply the shared fallback cell ratio when the config does not pin one explicitly
- pass typed options into `internal/audio`, `internal/ui`, `internal/sources/local`, and `internal/sources/youtube`, including a resolved semantic UI palette instead of a preset theme string

The package source now also carries package-level and exported-symbol documentation so the TOML surface and normalization helpers stay understandable through Go tooling as the configuration surface grows.

Path handling belongs here so the rest of the application can work with cleaned, expanded filesystem paths instead of user-facing shorthand.

## Decisions

- Chose a central TOML-backed config service over continuing to add one-off env variables because the user said Musicon now has enough tunables that configuration should move into a file.
- Chose to keep config parsing in `internal/config` and pass typed options into runtime/UI/source constructors over letting each package read files or env for itself because startup policy belongs in the application wiring layer.
- Chose defaults-plus-single-user-config loading over requiring a fully populated config file because startup should still work out of the box when no config file is present.
- Chose the user XDG config file as the default implicit location over layering global XDG config directories because the user explicitly asked for one predictable default config path, while still allowing explicit overrides through higher-precedence startup inputs.
- Chose to accept both `backend` and legacy `protocol` keys for album-art renderer selection so the config surface can use clearer language without breaking earlier config files.
- Chose to reuse the shared fixed fallback ratio from `pkg/components` when `ui.cell_width_ratio` is omitted because the user explicitly asked to keep configured values only when set and otherwise use the default fallback during the Chafa migration.
- Chose cookie-file and browser-cookie settings plus optional raw yt-dlp args for the YouTube source because the user wanted authenticated access to private playlists and uploaded music without forcing the source layer to invent a separate auth protocol.
- Chose a TOML-backed `[keybinds]` section over leaving key handling hardcoded in the UI because the user wanted different terminals and personal habits to support custom shortcuts without code changes.
- Chose a semantic `[ui.theme]` palette plus optional external TOML theme file over a single preset-name string because the user wanted matugen-style color coordination while still being able to keep everything app-owned and startup-friendly.
- Chose a boolean `ui.compact_mode` startup setting plus a configurable global compact toggle key over a playback-screen-specific ad hoc flag because compact mode is startup policy that still needs a runtime override.
