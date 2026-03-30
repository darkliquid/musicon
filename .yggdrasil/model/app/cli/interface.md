# Public API

This node does not expose a reusable library API, but it does define the executable startup contract used by the binary entrypoint:

- load and normalize startup configuration through `internal/config`
- construct the audio runtime with `audio.NewEngine(...)`
- construct the active source/search implementation(s)
- construct and start the MPRIS bridge with `mpris.NewBridge(...)` and `Start()`
- construct the reusable cover-art provider chain and adapt it into the UI artwork service
- inject queue and playback service adapters into `ui.NewApp(...)`
- start the Bubble Tea program through `ui.Run(app)`
- close the MPRIS bridge and audio runtime during shutdown
- surface any startup or runtime error on stderr before exiting non-zero

# Failure modes

- If the UI application cannot be constructed, startup fails immediately.
- If the audio runtime cannot be constructed or shut down cleanly, the process must surface that failure.
- If the configured local source directories are unavailable, startup should degrade clearly instead of crashing the entire UI.
- If the configuration file is unreadable or invalid, the process must surface that error before startup continues.
- If the MPRIS bridge cannot connect to or claim the session bus, the process should report that explicitly but continue running the terminal player.
- If optional artwork-provider credentials or cache directories are unavailable, startup should degrade to lower-priority providers or no-art fallback rather than preventing the TUI from running.
- If the Bubble Tea program returns an error, the process must print the error and exit instead of swallowing it.
