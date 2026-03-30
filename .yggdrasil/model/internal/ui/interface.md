# Public API

The node currently exposes a small application-facing surface from `internal/ui`:

- `NewApp(Services) *App`
- `Run(*App) error`

The `Services` struct carries the backend-facing contracts the UI compiles against:

- `SearchService` for source discovery and result retrieval
- `QueueService` for queue snapshots and mutation
- `PlaybackService` for transport, seek, volume, and playback snapshots
- `LyricsProvider`, `ArtworkProvider`, and `VisualizationProvider` for alternate playback panes, with artwork providers receiving reusable cover-art metadata and supplying image data to reusable rendering components

# Contracts

- Contracts stay narrow and UI-oriented rather than mirroring backend internals.
- Queue and playback screens must run against nil or partially configured services by showing explicit placeholders and empty states.
- Search results and queue entries should be able to carry reusable artwork metadata forward so playback snapshots can reuse source-derived local paths, embedded-art hints, and external IDs.
- The root model owns mode switching, help toggling, and visually square viewport resizing.
- `NewApp` seeds the Bubble Tea program with a best-effort initial terminal size so the first frame can render even when the terminal does not deliver an immediate startup resize event.
- `NewApp` also chooses a terminal cell width ratio (default `0.5`, override via `MUSICON_CELL_WIDTH_RATIO`) so the square viewport remains visually square under non-square terminal glyphs.
- The root model drives periodic tick-based redraws so playback status and progress can refresh without waiting for user input.
- The root model also publishes terminal window titles derived from mode, help state, and current playback snapshot through Bubble Tea's `View.WindowTitle` field.
- The root model also enforces the minimum supported terminal size and suppresses normal mode interaction until the viewport is large enough.
- Queue mode owns source cycling, query input, filter toggles, and a single merged browser list where queued items remain pinned before the current search results.
- Queue mode must keep query editing and browser navigation live at the same time: typing and deletion refresh the active search while movement keys continue to change the selected row, and `enter` toggles the selected item between enqueued and not enqueued.
- Playback mode owns pane switching, transport key routing, scrubber controls, repeat/stream toggles, and track-info visibility while delegating real playback state changes to injected services.
- Playback artwork rendering should route provider-supplied image data through reusable `pkg/components` image rendering instead of embedding terminal-image protocol logic inside the screen model.
- Playback artwork requests should pass normalized cover-art metadata into the provider path instead of relying on a narrow track-ID-only contract.
- Musicon-specific adaptation from reusable cover-art resolvers into the UI artwork contract belongs in `internal/ui`, not in `pkg/coverart`.

# Failure modes

- Missing or empty backend data renders empty-state messaging instead of causing crashes.
- Unsupported capabilities leave the UI interactive, but route actions to no-op or explanatory status messaging.
- Layout changes from terminal resizing must preserve a valid centered square viewport.
- When the terminal is undersized, the UI must present clear resize requirements and keep only the quit path active.
- If the terminal does not respond to Bubble Tea's live size probe at startup, the UI must still render using the seeded initial dimensions until a real resize event arrives.
