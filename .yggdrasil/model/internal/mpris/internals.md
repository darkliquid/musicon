# Logic

`internal/mpris` is a bridge between Musicon playback state and the desktop session bus.

It should:

- connect to the session bus
- export the standard MPRIS object path and interfaces
- translate Musicon playback snapshots into MPRIS properties such as playback status, metadata, and position
- translate incoming D-Bus control calls into playback-service method calls
- refresh property-backed state on a background ticker so D-Bus clients can observe live playback progress without driving UI updates
- use property callbacks for writable controls like loop mode and volume so remote desktop changes reuse the same playback mutations as local key presses
- reject MPRIS seek and set-position requests explicitly instead of trying to emulate a transport that the playback runtime no longer supports

This node should own D-Bus-specific details so the audio runtime and UI remain focused on playback and presentation concerns.

## Decisions

- Chose a dedicated `internal/mpris` service over embedding D-Bus logic into `internal/audio` so desktop integration remains optional and isolated from playback output internals.
- Chose direct `godbus/dbus/v5` usage because no reliable higher-level wrapper was available in this environment.
