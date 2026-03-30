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

## Decisions

- Chose `pkg/components` for reusable widgets because the user explicitly requested that generic UI components live outside `internal/ui`.
- Chose stateless render helpers for panels, progress, and empty states while keeping stateful behavior in `Input` and `List` because those are the generic widgets that benefit most from reusable update logic.
- Chose a generic cached terminal-image component in `pkg/components` over embedding `go-termimg` calls directly in playback mode because the user wanted protocol-aware image rendering to stay reusable while `internal/ui` only supplies artwork-specific data and fallback messaging.
- Chose to reserve cursor width inside the reusable input field instead of letting the focused cursor overflow because a one-column spill from a shared widget can visibly break square-constrained parent layouts.
- Chose an explicit cell width ratio input for square viewport math instead of assuming terminal cells are square because the user observed the visual frame distortion caused by tall terminal glyphs.
- Chose a generic leading marker field on list items instead of hard-coding queue icons into the widget because callers may need lightweight row state cues without turning the shared list into a Musicon-specific queue component.
