# Logic

`internal/audio` is the concrete playback runtime for Musicon.

It bridges the UI-facing playback and queue contracts to an actual output pipeline built on `github.com/gopxl/beep` and a small mago-backed speaker wrapper that can honor configured backend selection.

The expected shape is:

- queue entries describe what should be played
- an injected resolver turns a queue entry into a playable `beep.StreamSeekCloser`
- the runtime manages speaker initialization, active streamer lifecycle, pause/resume, volume, and queue progression
- backend selection is normalized before playback starts so configured choices such as PulseAudio, ALSA, JACK, or CoreAudio can be requested explicitly while `auto` keeps platform-default behavior
- backend discovery now probes the current platform's likely backend set and reports canonical config strings such as `pulse`, `alsa`, or `coreaudio` for CLI use
- backend-name canonicalization collapses config aliases such as `pulseaudio` and `directsound` onto the same labels used by backend discovery so CLI annotation and runtime selection agree
- thin queue/playback adapter wrappers expose the UI contracts over one shared engine state object
- queue mutation includes relative move operations so queue mode can reorder entries while the engine remains the single source of truth for playback order
- queue mutation can now add grouped album/playlist collections and remove those groups as one unit, while still storing the playable queue as ordinary child track entries
- startup restore can seed the linear queue, selected queue index, repeat/stream flags, volume, and a restorable playback snapshot without activating a live stream until the user explicitly resumes playback
- queue-carried artwork metadata is merged with resolver-provided track info so the UI artwork path keeps local paths, embedded-art hints, and external IDs even when different layers know different parts of the metadata
- resolved track info can carry richer cover-art metadata forward to the UI artwork path without forcing the runtime itself to fetch or render artwork
- snapshot reads should remain fast even while the runtime is busy resolving or swapping tracks, so UI polling can fall back to the last published playback snapshot instead of blocking the Bubble Tea render loop on the engine mutex
- seek now exposes an absolute `SeekTo(time.Duration)` runtime surface: it first tries an in-place `StreamSeekCloser.Seek`, and if the active stream reports that the target is outside its cheap local seek window, the runtime asks that stream to prepare a replacement in the background, then atomically swaps the replacement stream in while preserving the prior paused/playing state
- replacement-stream activation still flows through the runtime's normal controller/volume wiring, so seek swaps reuse the same queue metadata, now-playing callbacks, volume state, and cleanup rules as an ordinary track activation instead of introducing a special speaker path
- the runtime now also owns a lightweight analysis path for playback visualization: after resampling and volume control are wired, it wraps the outbound stream in a tap that copies the already-playing stereo samples into a small in-memory spectrum analyzer, computes a 1024-sample Hann-window FFT at a throttled cadence, smooths the resulting logarithmic bands, and exposes those bands through a UI-facing visualization adapter that renders the EQ and mirrored visualizer panes without leaving the Go process
- visualization output is intentionally derived from the existing playback stream instead of a second decoder or subprocess, so the EQ pane stays aligned with the active track while keeping memory and CPU overhead bounded to one recent sample buffer plus infrequent FFT work
- the EQ and mirrored visualizer renderers now rasterize the smoothed band levels into a 2×4 subcell grid and emit Unicode braille characters, letting each terminal cell carry eight addressable dots instead of a single block ramp step while still coloring rows through the existing gradient palette
- the mirrored visualizer still maps color symmetrically from the playback centerline outward, but its shape is now represented through the braille raster instead of separate upper/lower block-orientation rules
- tests that exercise real speaker initialization should pin the `null` backend so CI can verify resume/seek behavior without depending on ALSA or another host audio device being available

The package source now also carries package-level and exported-symbol documentation so the engine, adapters, and speaker helpers can be understood from Go docs without reopening every runtime implementation detail.

This node should own concurrency, lifecycle, cleanup concerns, and restorable playback context so `internal/ui` stays presentation-focused.

Session restore now uses the runtime as the playback-state source of truth even before any stream is active. The engine can reopen with a restored queue and a paused snapshot representing the previously active track and position; `PlaybackSnapshot()` returns that restorable track context while no live stream exists yet, and the next play/resume request uses the saved queue index and seeks to the saved position before unpausing when the resolver supports it.

## Decisions

- Chose a dedicated `internal/audio` service over embedding playback state in `internal/ui` so runtime concerns stay separated from the TUI layer.
- Chose a small local mago-backed speaker wrapper over a hard-wired `mago/speaker` initialization path because the user wanted backend selection to move into config and the runtime needed to honor that choice explicitly.
- Chose resolver-based playback over direct file decoding in the runtime so actual source implementations can be added later without rewriting the engine core.
- Chose to probe only the current platform's canonical backend candidates and return config-safe names because the user wanted `--list-backends` output to be directly usable as config values, not a dump of every alias the parser accepts.
- Chose to serve playback snapshot polling from the latest published snapshot when the engine mutex is already busy because the user reported rapid playback key input appearing to lock the input thread, and stale-but-fast UI state is safer than blocking the render loop behind slow resolver work.
- Chose replacement-stream swapping over forcing all providers to implement far seeks in place because the user explicitly wanted seek preparation to happen away from the UI/input path while the old audio continues until the new stream is ready.
- Chose flat child-track queue storage plus group metadata over introducing a second nested runtime queue model because playback still needs a linear play order even when the UI exposes whole-collection removal.
- Chose runtime-owned queue/playback restoration over making `internal/ui` fake the current track after restart because the user wanted reopen behavior to feel seamless while transport state still needs one authoritative owner.
- Chose a built-in FFT-based analyzer over `cava`/`cavacore` because the user prioritized compatibility with the existing Go audio/rendering pipeline plus low CPU/memory overhead, and a tapped in-process analyzer avoids subprocess orchestration, IPC, and duplicated decoding work.
- Chose row-based gradient coloring plus a braille raster over block glyph ramps because the user wanted smoother EQ/visualizer visuals and braille gives each cell a native 2×4 resolution without changing the analyzer pipeline.
- Reused the same braille-and-gradient strategy for the mirrored visualizer instead of inventing a second renderer so both visualization panes stay visually coherent and the renderer stays simple to maintain.
- Chose neutral empty visualization/artwork backgrounds over explanatory placeholder copy because the user wanted the playback panes to feel like real surfaces even before providers or content are available.
- Chose the `null` output backend in speaker-initializing tests over the platform default because CI must cover pause/resume restoration logic even when the runner has no usable audio device.
