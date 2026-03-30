# Logic

`internal/audio` is the concrete playback runtime for Musicon.

It bridges the UI-facing playback and queue contracts to an actual output pipeline built on `github.com/gopxl/beep` and `github.com/darkliquid/mago/speaker`.

The expected shape is:

- queue entries describe what should be played
- an injected resolver turns a queue entry into a playable `beep.StreamSeekCloser`
- the runtime manages speaker initialization, active streamer lifecycle, pause/resume, seek, volume, and queue progression
- thin queue/playback adapter wrappers expose the UI contracts over one shared engine state object
- resolved track info can carry richer cover-art metadata forward to the UI artwork path without forcing the runtime itself to fetch or render artwork

This node should own concurrency, lifecycle, and cleanup concerns so `internal/ui` stays presentation-focused.

## Decisions

- Chose a dedicated `internal/audio` service over embedding playback state in `internal/ui` so runtime concerns stay separated from the TUI layer.
- Chose `mago/speaker` over `beep/speaker` because it provides a beep-compatible playback surface backed by the requested mago output driver.
- Chose resolver-based playback over direct file decoding in the runtime so actual source implementations can be added later without rewriting the engine core.
