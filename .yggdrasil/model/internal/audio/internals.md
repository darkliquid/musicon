# Logic

`internal/audio` is the concrete playback runtime for Musicon.

It bridges the UI-facing playback and queue contracts to an actual output pipeline built on `github.com/gopxl/beep` and a small mago-backed speaker wrapper that can honor configured backend selection.

The expected shape is:

- queue entries describe what should be played
- an injected resolver turns a queue entry into a playable `beep.StreamSeekCloser`
- the runtime manages speaker initialization, active streamer lifecycle, pause/resume, seek, volume, and queue progression
- backend selection is normalized before playback starts so configured choices such as PulseAudio, ALSA, JACK, or CoreAudio can be requested explicitly while `auto` keeps platform-default behavior
- thin queue/playback adapter wrappers expose the UI contracts over one shared engine state object
- queue mutation includes relative move operations so queue mode can reorder entries while the engine remains the single source of truth for playback order
- queue-carried artwork metadata is merged with resolver-provided track info so the UI artwork path keeps local paths, embedded-art hints, and external IDs even when different layers know different parts of the metadata
- resolved track info can carry richer cover-art metadata forward to the UI artwork path without forcing the runtime itself to fetch or render artwork

This node should own concurrency, lifecycle, and cleanup concerns so `internal/ui` stays presentation-focused.

## Decisions

- Chose a dedicated `internal/audio` service over embedding playback state in `internal/ui` so runtime concerns stay separated from the TUI layer.
- Chose a small local mago-backed speaker wrapper over a hard-wired `mago/speaker` initialization path because the user wanted backend selection to move into config and the runtime needed to honor that choice explicitly.
- Chose resolver-based playback over direct file decoding in the runtime so actual source implementations can be added later without rewriting the engine core.
