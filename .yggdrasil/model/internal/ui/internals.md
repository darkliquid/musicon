# Logic

`internal/ui` now contains:

- `rootModel`, the Bubble Tea application shell
- `queueScreen`, the Musicon-specific queue-management screen
- `playbackScreen`, the Musicon-specific playback screen
- shared UI-facing contracts and small rendering helpers

The root model computes a visually square viewport from terminal size with `components.ClampSquareWithCellWidthRatio`, renders a bordered application frame, keeps queue/playback as dedicated top-level modes, emits periodic Bubble Tea ticks so playback state can refresh while audio is active, and updates the terminal title through Bubble Tea's `View.WindowTitle` field instead of embedding title control sequences in the visible frame content.

Startup now seeds Bubble Tea with an initial terminal size derived from the live stdout TTY when available, then falls back to `COLUMNS`/`LINES`, then `80x24`. `Init` still explicitly requests the live window size so interactive terminals can correct the seeded dimensions immediately. This avoids a blank startup state in PTYs or terminal emulators that never answer Bubble Tea's size probe.

The root model assumes a default terminal cell width ratio of `0.5`, meaning a typical terminal glyph is roughly twice as tall as it is wide. Users can override that assumption with `MUSICON_CELL_WIDTH_RATIO` when their font differs. This keeps the rendered frame visually square instead of merely using the same number of rows and columns.

It now also evaluates explicit minimum viewport requirements before rendering the main shell. If the terminal is smaller than the supported `20×20` minimum, the root model renders a dedicated resize warning and blocks normal application interactions until the terminal is large enough again.

Queue mode arranges source chips, filter chips, search input, result browsing, and queue inspection inside the square frame with discovery content above the queue.

Playback mode arranges an album-art-first center pane, a transport/progress strip, optional metadata, and switchable artwork/lyrics/eq/visualizer panes inside the same frame.

Nil or partially configured services are treated as valid UI states: the screens stay navigable and show empty-state messaging rather than inventing missing backend behavior. When playback services are present, repeat and stream toggles route through the injected runtime instead of living only in local screen state, and the root shell uses the latest playback snapshot to keep the terminal title in sync with visible state. Search results and queue entries can now carry reusable artwork metadata so local file paths, embedded-art hints, and external IDs survive queueing before a concrete source resolver exists. Artwork providers then receive normalized reusable cover-art metadata and playback mode delegates protocol detection and rendering to a reusable `pkg/components.TerminalImage` widget. A small adapter in `internal/ui` bridges reusable `pkg/coverart` resolvers to the UI-facing `ArtworkProvider` contract without pushing Bubble Tea or component types into the reusable package.

The node delegates reusable widgets such as lists, inputs, panels, progress bars, and empty-state renderers to `pkg/components`.

## Decisions

- Chose dedicated fullscreen queue and playback modes over a persistent split layout because the user explicitly selected that interaction model.
- Chose a square-centered viewport over using the entire terminal canvas because the user explicitly required the app to always fit inside a `1:1` frame.
- Chose an explicit `20×20` minimum terminal policy over allowing arbitrarily small degraded layouts so queue and playback screens fail clearly instead of becoming cramped and misleading.
- Chose interface-only backend integration over stub source or playback implementations because the user explicitly asked for UI-only work at this stage.
- Kept backend construction outside `internal/ui` even after adding a real audio runtime so the UI remains decoupled from concrete playback wiring.
- Chose terminal-title control in the root UI shell over a separate runtime service because the title is presentation state derived from what the UI is already rendering.
- Chose a root-owned help toggle with screen-specific help views so shared chrome stays consistent while each mode can document its own controls.
- Chose to keep `go-termimg` usage out of `internal/ui` by pushing terminal-image rendering into `pkg/components`, so playback mode remains focused on choosing panes and handling fallback messaging instead of protocol details.
- Chose to pass full cover-art metadata into artwork providers instead of a bare track ID so the future reusable provider chain can use local paths, embedded art, and multiple external IDs without making the playback screen aware of lookup policy.
- Chose to seed Bubble Tea with a best-effort initial terminal size instead of waiting only on `RequestWindowSize`, because some PTYs never answer the size query and would otherwise leave the UI stuck in the zero-dimension loading state.
- Chose a configurable default cell width ratio of `0.5` instead of treating terminal cells as square because visual squareness in real terminals depends on glyph geometry, not only on row and column counts.
