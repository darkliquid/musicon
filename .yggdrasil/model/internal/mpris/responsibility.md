# Responsibility

This node owns Musicon's MPRIS bridge.

It is responsible for:

- exporting an MPRIS-compatible D-Bus object on the session bus
- projecting current playback metadata and status from the runtime into MPRIS properties
- routing desktop media-control actions back into Musicon playback controls
- polling runtime snapshots often enough to keep desktop-visible state current without involving the Bubble Tea event loop

# Boundaries

This node is not responsible for:

- terminal UI rendering
- direct audio output
- source discovery or queue search
- deciding application layout or key bindings
