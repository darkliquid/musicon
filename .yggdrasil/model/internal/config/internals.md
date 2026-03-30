# Logic

`internal/config` gives Musicon one typed place to describe startup tunables that affect runtime wiring.

The expected shape is:

- look for an explicit config path first
- otherwise load the global XDG config and then overlay the user XDG config if it is present
- start from code defaults, then overlay TOML values
- normalize values such as backend names, start mode, fill mode, and local directories
- pass typed options into `internal/audio`, `internal/ui`, and `internal/sources/local`

Path handling belongs here so the rest of the application can work with cleaned, expanded filesystem paths instead of user-facing shorthand.

## Decisions

- Chose a central TOML-backed config service over continuing to add one-off env variables because the user said Musicon now has enough tunables that configuration should move into a file.
- Chose to keep config parsing in `internal/config` and pass typed options into runtime/UI/source constructors over letting each package read files or env for itself because startup policy belongs in the application wiring layer.
- Chose defaults-plus-overlay loading over requiring a fully populated config file because startup should still work out of the box when no config file is present.
- Chose XDG global config plus user overlay over a single first-match config search because the user explicitly wanted site-wide defaults that can still be overridden per user.
