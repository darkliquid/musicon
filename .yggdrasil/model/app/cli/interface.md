# Public API

This node does not expose a reusable library API, but it does define the executable startup contract used by the binary entrypoint:

- construct the audio runtime with `audio.NewEngine(...)`
- inject queue and playback service adapters into `ui.NewApp(...)`
- start the Bubble Tea program through `ui.Run(app)`
- close the audio runtime during shutdown
- surface any startup or runtime error on stderr before exiting non-zero

# Failure modes

- If the UI application cannot be constructed, startup fails immediately.
- If the audio runtime cannot be constructed or shut down cleanly, the process must surface that failure.
- If the Bubble Tea program returns an error, the process must print the error and exit instead of swallowing it.
