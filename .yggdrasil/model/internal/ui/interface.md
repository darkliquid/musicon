# Public API

The node currently exposes a small application-facing surface from `internal/ui`:

- `NewApp(Services, Options) *App`
- `Run(*App) error`

The `Services` struct carries the backend-facing contracts the UI compiles against:

- `SearchService` for source discovery and result retrieval, with caller-supplied context so UI search can cancel superseded provider work
- `QueueService` for queue snapshots and mutation
- `PlaybackService` for transport, volume, and playback snapshots
- `LyricsProvider`, `ArtworkProvider`, and `VisualizationProvider` for alternate playback panes, with lyrics providers receiving reusable metadata requests and returning reusable lyrics documents, artwork providers receiving reusable cover-art metadata, optionally reporting provider-attempt progress, and visualization providers returning live pane-sized EQ/visualizer content that is safe to request during ordinary redraws
- `Options` for startup mode, startup compact-mode preference, a resolved semantic theme palette, cell-width ratio, playback artwork rendering preferences, an optional restored session snapshot, and an app-owned session store used for persistence

# Contracts

- Contracts stay narrow and UI-oriented rather than mirroring backend internals.
- Queue and playback screens must run against nil or partially configured services by showing explicit placeholders and empty states.
- Search results and queue entries should be able to carry reusable artwork metadata forward so playback snapshots can reuse source-derived local paths, embedded-art hints, and external IDs.
- The root model owns mode switching, compact-mode toggling, help toggling, and visually square viewport resizing.
- `NewApp` seeds the Bubble Tea program with a best-effort initial terminal size so the first frame can render even when the terminal does not deliver an immediate startup resize event.
- `NewApp` should accept typed startup options from the application layer, including the initial mode and terminal cell width ratio, while still allowing an env override and a shared fixed fallback for legacy/default operation.
- `NewApp` should accept a fully resolved semantic theme palette from application wiring and apply it consistently across root chrome, warnings, queue chips, playback overlays, and reusable component render helpers without making `internal/ui` parse config files.
- The root model drives periodic tick-based redraws so playback status and progress can refresh without waiting for user input.
- The root model should capture restorable UI state through an app-owned session-store contract instead of writing files itself, so app wiring can choose where the session snapshot lives.
- The root model also publishes terminal window titles derived from mode, help state, and current playback snapshot through Bubble Tea's `View.WindowTitle` field.
- The root model should persist compact-mode state in the app-owned session snapshot so runtime toggles survive a restart alongside mode, help, queue, and playback-pane state.
- The root model also enforces the minimum supported terminal size and suppresses normal mode interaction until the viewport is large enough.
- The root model must render only the centered square itself during normal operation; persistent outer chrome such as tab bars, footer bars, or mode banners should not live outside the square.
- Help stays in the active mode instead of replacing it with a separate screen: the root model overlays the current mode's help card inside the square viewport.
- Queue mode owns source cycling, query input, filter toggles, and a single merged browser list where queued items remain pinned before the current search results.
- Queue mode should be able to restore its last source, query, current search results, expanded collections, focus zone, and selected browser row from a session snapshot when the app relaunches with compatible sources.
- Queue mode must let users move focus naturally with up/down across the visible control stack: source chips, search-kind chips, search input, and the merged browser list.
- When the source-chip or search-kind rows are focused, left/right should cycle the active source or active search kind without requiring separate focus-toggle shortcuts.
- When search is focused, printable input edits the active query; when any non-search zone is focused, queue-management shortcuts such as source switching, filter toggles, removal, and reorder actions become active again.
- Queue mode may keep a dedicated search shortcut as a convenience, but search entry must not depend on that shortcut being available.
- Queue mode must not block the Bubble Tea event loop on source-backed searches; slow or networked searches should resolve asynchronously so quit, mode switching, and navigation remain responsive.
- Queue mode should debounce live search input and cancel superseded in-flight searches so remote providers do not spin up one request per keystroke.
- When queue mode adds the first internet-radio stream to an otherwise empty queue and a playback service is present, it should start playback asynchronously instead of requiring an immediate mode switch plus extra play keypress.
- The UI must accept typed, config-driven keybindings for global shell actions and per-screen controls instead of hardcoding Bubble Tea key strings inside each update loop.
- Queue mode should clearly mark the currently playing queued item so users can tell which pinned row is active even while browsing or reordering the rest of the queue.
- Queue mode should pass source labels such as `radio:` or `youtube:` to the shared list as anchored leading prefixes instead of baking them into the scrolling title text, so focused marquee rendering can expose long names without moving the source identity marker.
- Queue mode should render directly into the square without wrapping itself in a second persistent chrome layer.
- Playback mode owns pane switching, transport key routing, repeat/stream toggles, and track-info visibility while delegating real playback state changes to injected services.
- Playback mode should honor a root-owned compact flag that strips playback-specific chrome; in compact mode the rendered playback view keeps the active artwork/lyrics/eq/visualizer pane visible while eliding pane labels, help overlays, and track-info cards, leaving only the pane content plus a minimal scrubber/progress strip.
- Playback mode should be able to restore its last pane, track-info overlay visibility, lyrics scroll position, and caller-supplied playback snapshot so reopening the app feels seamless even though audio does not auto-start.
- Playback mode should accept album-art rendering preferences from UI startup options so fill mode and protocol selection no longer depend on each screen reading env directly.
- Playback mode should treat the active artwork/lyrics/eq/visualizer pane as the base layer and place pane labels, transport controls, and optional track metadata as overlays within that same square instead of stacking separate panels below it.
- Compact playback should intentionally ignore track-info overlay visibility while enabled, but it must still allow pane switching so lyrics, EQ, and visualizer content remain accessible inside the minimal playback surface.
- When rendered artwork does not occupy the full playback pane, the remaining pane area should use a muted filler pattern so the image bounds remain legible without overwhelming the artwork itself.
- Playback artwork rendering should route provider-supplied image data through reusable `pkg/components` image rendering instead of embedding terminal-image protocol logic inside the screen model.
- Playback artwork requests should pass normalized cover-art metadata into the provider path instead of relying on a narrow track-ID-only contract.
- Playback lyrics requests should pass normalized reusable lyrics metadata into the provider path instead of relying on a narrow track-ID-only contract.
- Playback lyrics lookups must remain on-demand and non-blocking: entering or repainting the Lyrics pane may trigger a background lookup, but the Bubble Tea update loop must stay responsive while the resolver works.
- Playback lyrics panes should render plain lines from reusable lyrics documents by default, and when synced LRC timing is available they should auto-follow the active lyric line derived from the current playback position so karaoke-style sing-along stays aligned without blocking the UI.
- Playback lyrics panes must keep long lyric content inside the square pane by rendering it through a scrollable viewport rather than letting long documents spill past the visible area.
- Manual lyrics scrolling remains relevant for plain or unsynced documents; timed LRC documents should prefer playback-following over manual scroll offsets.
- Playback artwork providers should also be able to report recent provider/cache attempts so the playback pane can surface lookup progress without embedding provider-chain logic directly in the screen.
- Playback visualization providers should be callable on ordinary repaints rather than only once per pane/size, because live EQ surfaces change continuously even when the viewport dimensions stay fixed.
- Musicon-specific adaptation from reusable cover-art resolvers into the UI artwork contract belongs in `internal/ui`, not in `pkg/coverart`.
- Musicon-specific adaptation from reusable lyrics resolvers into the UI lyrics contract also belongs in `internal/ui`, not in `pkg/lyrics`.

# Failure modes

- Missing or empty backend data renders empty-state messaging instead of causing crashes.
- Unsupported capabilities leave the UI interactive, but route actions to no-op or explanatory status messaging.
- Layout changes from terminal resizing must preserve a valid centered square viewport.
- When the terminal is undersized, the UI must present clear resize requirements and keep only the quit path active.
- If the terminal does not respond to Bubble Tea's live size probe at startup, the UI must still render using the seeded initial dimensions until a real resize event arrives.
