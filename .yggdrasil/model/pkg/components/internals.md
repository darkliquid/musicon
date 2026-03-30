# Logic

`pkg/components` currently provides:

- square viewport calculation helpers
- explicit terminal-size requirement helpers
- a single-line text input widget
- a selectable generic list widget
- a reusable terminal image widget backed by `github.com/blacktop/go-termimg`
- panel, progress-bar, and empty-state render helpers

These primitives are intentionally Bubble Tea-friendly but Musicon-agnostic, so `internal/ui` can compose them into queue and playback screens without embedding domain logic into the shared package.

The reusable `Input` widget now budgets its focused cursor inside the configured width instead of appending it past the edge of the line. This keeps parent layouts stable when a focused input sits inside a strictly sized square viewport.

The square viewport helpers now support visually square layouts under non-square terminal cells. Callers can supply a cell width-to-height ratio so a font whose cells are taller than they are wide produces a wider-in-columns, shorter-in-rows viewport that still looks like a square on screen.

The reusable `List` widget now supports an optional leading marker per item. This lets callers distinguish pinned or stateful rows such as "already queued" entries while keeping selection, scrolling, and row layout generic.

The same list now also exposes explicit selected-index restoration so callers that rebuild their item set from live state can preserve which logical row stays highlighted after reordering or snapshot refreshes.

The reusable `TerminalImage` widget now defaults `go-termimg` to the Unicode halfblock protocol so artwork remains visible even in terminals that do not actually display richer graphics protocols reliably. Callers and users can still opt into `auto`, `kitty`, `sixel`, or `iterm2` through `MUSICON_IMAGE_PROTOCOL` when they want higher-fidelity rendering in a compatible terminal.

The same widget now uses a fill-oriented scaling mode by default so album art expands to occupy more of the available viewport instead of sitting centered with large margins. Users who prefer preserved framing or exact stretching can override this with `MUSICON_IMAGE_SCALE`.

## Decisions

- Chose `pkg/components` for reusable widgets because the user explicitly requested that generic UI components live outside `internal/ui`.
- Chose stateless render helpers for panels, progress, and empty states while keeping stateful behavior in `Input` and `List` because those are the generic widgets that benefit most from reusable update logic.
- Chose a generic cached terminal-image component in `pkg/components` over embedding `go-termimg` calls directly in playback mode because the user wanted protocol-aware image rendering to stay reusable while `internal/ui` only supplies artwork-specific data and fallback messaging.
- Chose a guaranteed-visible halfblock default with an env override over always trusting `go-termimg` auto-detection because local artwork can already resolve correctly while terminal protocol auto-selection still fails to display images for some users.
- Chose a fill-oriented default scale mode with an env override over a conservative fit-only default because the user explicitly preferred artwork that uses the available square more aggressively.
- Chose to reserve cursor width inside the reusable input field instead of letting the focused cursor overflow because a one-column spill from a shared widget can visibly break square-constrained parent layouts.
- Chose an explicit cell width ratio input for square viewport math instead of assuming terminal cells are square because the user observed the visual frame distortion caused by tall terminal glyphs.
- Chose a generic leading marker field on list items instead of hard-coding queue icons into the widget because callers may need lightweight row state cues without turning the shared list into a Musicon-specific queue component.
- Chose an explicit `SetSelectedIndex` hook on the shared list instead of forcing every caller to infer selection through synthetic key events because identity-preserving rebuilds are generic widget behavior, not queue-specific logic.
