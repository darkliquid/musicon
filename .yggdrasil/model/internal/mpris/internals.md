# Logic

`internal/mpris` is a bridge between Musicon playback state and the desktop session bus.

It should:

- connect to the session bus
- export the standard MPRIS object path and interfaces
- translate Musicon playback snapshots into MPRIS properties such as playback status, metadata, and position
- translate incoming D-Bus control calls into playback-service method calls
- refresh property-backed state on a background ticker so D-Bus clients can observe live playback progress without driving UI updates
- use property callbacks for writable controls like loop mode and volume so remote desktop changes reuse the same playback mutations as local key presses
- export player transport methods through an explicit D-Bus method table so MPRIS method names such as `Seek` and `SetPosition` do not need to mirror Go method names that trigger `go vet` interface checks
- fail both relative `Seek` and absolute `SetPosition` requests explicitly because Musicon no longer exposes seek control through its playback service and already advertises `CanSeek=false`

The package source now also carries package-level and exported-symbol documentation so the MPRIS lifecycle and method mappings remain readable from Go tooling without replaying the D-Bus export sequence mentally. The contributor-doc sweep also added a file-level overview around the concrete bridge implementation so newcomers can see that this node is pure infrastructure layered on top of the playback service.

This node should own D-Bus-specific details so the audio runtime and UI remain focused on playback and presentation concerns.

## Decisions

- Chose a dedicated `internal/mpris` service over embedding D-Bus logic into `internal/audio` so desktop integration remains optional and isolated from playback output internals.
- Chose direct `godbus/dbus/v5` usage because no reliable higher-level wrapper was available in this environment.
- Chose explicit D-Bus method-table export for player controls over exported Go methods because the MPRIS method names must stay standard while `go vet` rejects the `Seek` method name/signature pairing on the Go type.
- Chose to fail MPRIS seek requests explicitly over partially emulating them because the playback service no longer exposes seek support and the bridge already reports `CanSeek=false` to desktop clients.
