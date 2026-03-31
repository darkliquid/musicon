# Logic

`main.go` is intentionally thin.

The current implementation:

- loads a TOML-backed config surface before constructing runtime services
- supports a startup flag that prints usable backend names for the current environment, marks the currently effective configured backend, and exits before normal startup; if discovery fails, it exits silently rather than mixing diagnostics into machine-readable output
- supports a startup flag that prints usable image-renderer backends for the current terminal, marks the currently effective configured renderer, and exits before normal startup
- supports debug logging to stderr and/or a caller-selected file, with the executable owning logger setup before runtime construction so later background work such as artwork lookup can emit trace lines safely from multiple goroutines
- passes the same CLI-owned debug sink into the radio source so transport-level HLS diagnostics can be recorded without printing directly into the Bubble Tea viewport
- imports `internal/audio` and `internal/ui`
- constructs the concrete local-file source implementation from configured directories
- constructs the yt-dlp-backed YouTube Music source from typed config, including cookie-based auth and cache settings
- composes the source layer into one search service plus one resolver so the UI and audio runtime can stay source-agnostic
- constructs the concrete audio runtime
- injects queue, playback, and visualization services from the runtime into `ui.Services` plus typed UI options derived from config, including the `[keybinds]` section and queue bindings for visible search-kind slot selection, search-kind cycling, and collection expansion
- constructs the cover-art chain in local-first order, including a metadata-url fast path ahead of the heavier remote lookup providers, and wraps that resolver with an app-owned debug adapter when debug logging is enabled so each artwork request records normalized metadata, provider order, per-provider attempt counts, provider/cache attempt events, elapsed time, and final success or failure without pushing file-I/O concerns into `internal/ui` or `pkg/coverart`
- constructs the lyrics chain in local-first order, using neighboring `.lrc` sidecars before a cached `lrclib.net` resolver rooted in the user cache directory, then adapts that reusable chain into the UI-facing lyrics service
- calls `ui.Run(app)`
- defers `engine.Close()` to release audio resources on exit
- writes failures to stderr and exits with a non-zero status

The package source now also carries package-level and exported-symbol documentation so CLI wiring, source aggregation, and resolver routing remain legible through Go tooling without tracing every startup path manually.

It deliberately avoids accumulating view logic, component composition, or backend adapters.

## Decisions

- Chose a thin executable entrypoint over placing UI logic in `main.go` because the user explicitly requested that all UI code live in `internal/ui`.
- Chose CLI-owned runtime wiring over having `internal/ui` construct audio services so the UI package stays presentation-focused and backend selection remains an application concern.
- Chose to construct the first concrete local source in the CLI layer so future source combinations can be composed without making `internal/audio` or `internal/ui` own source selection policy.
- Chose to keep multi-source composition in the CLI layer rather than pushing routing into `internal/audio` or `internal/ui` because the executable already owns startup policy and concrete service wiring.
- Chose to centralize startup tunables in a TOML config file because the user said Musicon now has enough tunables that environment-variable-only configuration is no longer comfortable.
- Chose to expose backend discovery as a CLI flag in the app layer because it is an operational/startup concern that should not require booting the full TUI.
- Chose to expose image-renderer discovery as a parallel CLI flag because renderer capability depends on the current terminal and the user explicitly wanted the selected renderer to be inspectable without launching playback.
- Chose to translate `[keybinds]` config into typed UI options in the CLI layer instead of letting `internal/ui` read TOML directly because startup policy still belongs in application wiring.
- Chose to keep the new artwork debug-log sink in the CLI layer instead of `pkg/coverart` because selecting stderr vs file output is application policy, while the reusable cover-art package should stay transport-agnostic.
- Chose to build the lyrics provider chain in the CLI layer instead of `internal/ui` because provider ordering, cache-root selection, and network-vs-local fallback policy are application wiring concerns rather than screen logic.
- Chose to inject the EQ/visualizer provider from the audio runtime in the CLI layer rather than letting `internal/ui` construct it because analysis taps are part of playback wiring, not screen composition.
