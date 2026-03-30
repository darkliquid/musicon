# Public API

This node does not expose a reusable library API, but it does define the executable startup contract used by the binary entrypoint:

- load and normalize startup configuration through `internal/config`
- list usable audio backends through a CLI flag and mark the currently effective configured choice
- list usable image renderers through a CLI flag and mark the currently effective choice after applying config plus env override precedence
- construct the audio runtime with `audio.NewEngine(...)`
- construct and compose the active source/search implementation(s)
- construct and start the MPRIS bridge with `mpris.NewBridge(...)` and `Start()`
- construct the reusable cover-art provider chain and adapt it into the UI artwork service
- inject queue and playback service adapters plus typed UI keybinding options into `ui.NewApp(...)`
- start the Bubble Tea program through `ui.Run(app)`
- close the MPRIS bridge and audio runtime during shutdown
- surface any startup or runtime error on stderr before exiting non-zero

# Failure modes

- If the UI application cannot be constructed, startup fails immediately.
- If the audio runtime cannot be constructed or shut down cleanly, the process must surface that failure.
- If backend enumeration fails, the process should print nothing and exit non-zero instead of printing stale or guessed values.
- If image-renderer enumeration or config lookup fails for a listing flag, the process should print nothing and exit non-zero instead of mixing diagnostics into machine-readable output.
- If the configured local source directories are unavailable, startup should degrade clearly instead of crashing the entire UI.
- If an optional remote source such as YouTube Music is configured, startup should still succeed even when yt-dlp tooling or auth is not usable until the source is actually queried.
- If the configuration file is unreadable or invalid, the process must surface that error before startup continues.
- If the MPRIS bridge cannot connect to or claim the session bus, the process should report that explicitly but continue running the terminal player.
- If optional artwork-provider credentials or cache directories are unavailable, startup should degrade to lower-priority providers or no-art fallback rather than preventing the TUI from running.
- If the Bubble Tea program returns an error, the process must print the error and exit instead of swallowing it.
