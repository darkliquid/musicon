# Logic

`internal/ui` now contains:

- `rootModel`, the Bubble Tea application shell
- `queueScreen`, the Musicon-specific queue-management screen
- `playbackScreen`, the Musicon-specific playback screen
- shared UI-facing contracts and small rendering helpers

The root model computes a square viewport from terminal size with `components.ClampSquare`, renders a bordered application frame, keeps queue/playback as dedicated top-level modes, and emits periodic Bubble Tea ticks so playback state can refresh while audio is active.

It now also evaluates explicit minimum viewport requirements before rendering the main shell. If the terminal is smaller than the supported `20×20` minimum, the root model renders a dedicated resize warning and blocks normal application interactions until the terminal is large enough again.

Queue mode arranges source chips, filter chips, search input, result browsing, and queue inspection inside the square frame with discovery content above the queue.

Playback mode arranges an album-art-first center pane, a transport/progress strip, optional metadata, and switchable artwork/lyrics/eq/visualizer panes inside the same frame.

Nil or partially configured services are treated as valid UI states: the screens stay navigable and show empty-state messaging rather than inventing missing backend behavior. When playback services are present, repeat and stream toggles route through the injected runtime instead of living only in local screen state.

The node delegates reusable widgets such as lists, inputs, panels, progress bars, and empty-state renderers to `pkg/components`.

## Decisions

- Chose dedicated fullscreen queue and playback modes over a persistent split layout because the user explicitly selected that interaction model.
- Chose a square-centered viewport over using the entire terminal canvas because the user explicitly required the app to always fit inside a `1:1` frame.
- Chose an explicit `20×20` minimum terminal policy over allowing arbitrarily small degraded layouts so queue and playback screens fail clearly instead of becoming cramped and misleading.
- Chose interface-only backend integration over stub source or playback implementations because the user explicitly asked for UI-only work at this stage.
- Kept backend construction outside `internal/ui` even after adding a real audio runtime so the UI remains decoupled from concrete playback wiring.
- Chose a root-owned help toggle with screen-specific help views so shared chrome stays consistent while each mode can document its own controls.
