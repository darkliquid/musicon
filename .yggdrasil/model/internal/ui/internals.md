# Logic

`internal/ui` now contains:

- `rootModel`, the Bubble Tea application shell
- `queueScreen`, the Musicon-specific queue-management screen
- `playbackScreen`, the Musicon-specific playback screen
- shared UI-facing contracts and small rendering helpers

The root model computes a visually square viewport from terminal size with `components.ClampSquareWithCellWidthRatio`, renders only that centered square during normal operation, keeps queue/playback as dedicated top-level modes, emits periodic Bubble Tea ticks so playback state can refresh while audio is active, and updates the terminal title through Bubble Tea's `View.WindowTitle` field instead of embedding title control sequences in the visible frame content.

Startup now seeds Bubble Tea with an initial terminal size derived from the live stdout TTY when available, then falls back to `COLUMNS`/`LINES`, then `80x24`. `Init` still explicitly requests the live window size so interactive terminals can correct the seeded dimensions immediately. This avoids a blank startup state in PTYs or terminal emulators that never answer Bubble Tea's size probe.

The root model now prefers an explicit configured cell width ratio, still allows `MUSICON_CELL_WIDTH_RATIO` to override that value explicitly, and otherwise falls back to the shared default ratio from `pkg/components`. This keeps square-layout policy stable after the renderer migration while preserving explicit user control.

It now also evaluates explicit minimum viewport requirements before rendering the main shell. If the terminal is smaller than the supported `20×20` minimum, the root model renders a dedicated resize warning and blocks normal application interactions until the terminal is large enough again.

Queue mode arranges source chips, filter chips, search input, and a single merged browser list directly inside the square viewport without adding another persistent panel wrapper. Queued items stay pinned at the top of that browser with a marker, while current search results are appended below them without the marker until the user adds them to the queue. Typing and deletion keys keep the query live while arrow keys continue to move the browser selection, so `enter` can immediately toggle the selected row between enqueued and not enqueued without any explicit focus switch. It now also polls the playback snapshot while rendering so the active queued track can be marked inline, and it uses dedicated reorder keys to move queued items while keeping the selected row attached to the moved entry.

Playback mode now renders the selected artwork/lyrics/eq/visualizer pane as the full square background, then composes transport/progress details as a bottom overlay, pane identity as a top overlay, and optional track metadata as an additional top overlay. When album art does not fill the pane, the remaining area now uses a muted filler pattern so the eye can still read the intended artwork viewport. Help no longer swaps to a separate chrome-heavy screen; the root model centers the active screen's help card over the current square body.

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
- Chose to keep terminal-image library usage out of `internal/ui` by pushing rendering into `pkg/components`, so playback mode remains focused on choosing panes and handling fallback messaging instead of protocol details.
- Chose to pass full cover-art metadata into artwork providers instead of a bare track ID so the future reusable provider chain can use local paths, embedded art, and multiple external IDs without making the playback screen aware of lookup policy.
- Chose to seed Bubble Tea with a best-effort initial terminal size instead of waiting only on `RequestWindowSize`, because some PTYs never answer the size query and would otherwise leave the UI stuck in the zero-dimension loading state.
- Chose a shared fixed default cell width ratio over renderer-library auto-detection during the Chafa migration because the user explicitly asked for config-or-fallback behavior once `go-termimg` was removed.
- Chose a single merged queue browser over separate results and queue panes because the user wanted queued items to remain persistent and visually prioritized while search hits continue to appear after them in the same navigation flow.
- Chose explicit queue-reorder shortcuts plus an inline now-playing marker over a separate queue-edit mode because the user wanted the merged browser to stay direct and comfortable without more focus choreography.
- Chose a square-only root presentation over persistent outer tabs and footer chrome because the user wanted all relevant UI to stay inside one clean centered square at all times.
- Chose full-width row replacement overlays over a more complex transparent layer compositor because Bubble Tea's rendered content path is string-based and the simpler overlay model preserves the art-first layout without introducing fragile terminal-protocol composition logic.
- Chose a muted filler pattern around centered artwork over leaving unused pane space blank because the user wanted the album-art viewport to feel less empty while still making the rendered image bounds visible.
