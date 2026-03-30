# Logic

`internal/mpris` is a bridge between Musicon playback state and the desktop session bus.

It should:

- connect to the session bus
- export the standard MPRIS object path and interfaces
- translate Musicon playback snapshots into MPRIS properties such as playback status, metadata, and position
- translate incoming D-Bus control calls into playback-service method calls
- refresh property-backed state on a background ticker so D-Bus clients can observe live playback progress without driving UI updates
- use property callbacks for writable controls like loop mode and volume so remote desktop changes reuse the same playback mutations as local key presses
- route both relative `Seek` and absolute `SetPosition` requests through the same playback-service `SeekTo` API used by the TUI, so desktop media controls participate in the same debounced absolute-seek/runtime-swap model rather than inventing a second transport path

This node should own D-Bus-specific details so the audio runtime and UI remain focused on playback and presentation concerns.

## Decisions

- Chose a dedicated `internal/mpris` service over embedding D-Bus logic into `internal/audio` so desktop integration remains optional and isolated from playback output internals.
- Chose direct `godbus/dbus/v5` usage because no reliable higher-level wrapper was available in this environment.
- Chose to keep MPRIS seek semantics thin and delegate target validation/clamping to the playback runtime so desktop controls and TUI controls cannot drift into different seek behavior.
