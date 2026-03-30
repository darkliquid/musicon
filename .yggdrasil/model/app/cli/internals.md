# Logic

`main.go` is intentionally thin.

The current implementation:

- loads a TOML-backed config surface before constructing runtime services
- supports a startup flag that prints usable backend names for the current environment, marks the currently effective configured backend, and exits before normal startup; if discovery fails, it exits silently rather than mixing diagnostics into machine-readable output
- supports a startup flag that prints usable image-renderer backends for the current terminal, marks the currently effective configured renderer, and exits before normal startup
- imports `internal/audio` and `internal/ui`
- constructs the concrete local-file source implementation from configured directories
- constructs the concrete audio runtime
- injects queue and playback services from the runtime into `ui.Services` plus typed UI options derived from config
- calls `ui.Run(app)`
- defers `engine.Close()` to release audio resources on exit
- writes failures to stderr and exits with a non-zero status

It deliberately avoids accumulating view logic, component composition, or backend adapters.

## Decisions

- Chose a thin executable entrypoint over placing UI logic in `main.go` because the user explicitly requested that all UI code live in `internal/ui`.
- Chose CLI-owned runtime wiring over having `internal/ui` construct audio services so the UI package stays presentation-focused and backend selection remains an application concern.
- Chose to construct the first concrete local source in the CLI layer so future source combinations can be composed without making `internal/audio` or `internal/ui` own source selection policy.
- Chose to centralize startup tunables in a TOML config file because the user said Musicon now has enough tunables that environment-variable-only configuration is no longer comfortable.
- Chose to expose backend discovery as a CLI flag in the app layer because it is an operational/startup concern that should not require booting the full TUI.
- Chose to expose image-renderer discovery as a parallel CLI flag because renderer capability depends on the current terminal and the user explicitly wanted the selected renderer to be inspectable without launching playback.
