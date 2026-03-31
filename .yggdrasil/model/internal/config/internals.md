# Logic

`internal/config` gives Musicon one typed place to describe startup tunables that affect runtime wiring.

The expected shape is:

- look for an explicit config path first
- otherwise load the global XDG config and then overlay the user XDG config if it is present
- start from code defaults, then overlay TOML values
- normalize values such as audio backend names, image-renderer backend names, start mode, fill mode, local directories, configurable keybinding lists, and YouTube source paths/args
- carry queue-mode bindings for source switching, visible search-kind slot selection, search-kind cycling, and collection expansion so the richer multi-source queue workflow stays configurable from TOML instead of hardcoded in the UI
- apply the shared fallback cell ratio when the config does not pin one explicitly
- pass typed options into `internal/audio`, `internal/ui`, `internal/sources/local`, and `internal/sources/youtube`

The package source now also carries package-level and exported-symbol documentation so the TOML surface and normalization helpers stay understandable through Go tooling as the configuration surface grows.

Path handling belongs here so the rest of the application can work with cleaned, expanded filesystem paths instead of user-facing shorthand.

## Decisions

- Chose a central TOML-backed config service over continuing to add one-off env variables because the user said Musicon now has enough tunables that configuration should move into a file.
- Chose to keep config parsing in `internal/config` and pass typed options into runtime/UI/source constructors over letting each package read files or env for itself because startup policy belongs in the application wiring layer.
- Chose defaults-plus-overlay loading over requiring a fully populated config file because startup should still work out of the box when no config file is present.
- Chose XDG global config plus user overlay over a single first-match config search because the user explicitly wanted site-wide defaults that can still be overridden per user.
- Chose to accept both `backend` and legacy `protocol` keys for album-art renderer selection so the config surface can use clearer language without breaking earlier config files.
- Chose to reuse the shared fixed fallback ratio from `pkg/components` when `ui.cell_width_ratio` is omitted because the user explicitly asked to keep configured values only when set and otherwise use the default fallback during the Chafa migration.
- Chose cookie-file and browser-cookie settings plus optional raw yt-dlp args for the YouTube source because the user wanted authenticated access to private playlists and uploaded music without forcing the source layer to invent a separate auth protocol.
- Chose a TOML-backed `[keybinds]` section over leaving key handling hardcoded in the UI because the user wanted different terminals and personal habits to support custom shortcuts without code changes.
