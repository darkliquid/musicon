# Logic

`pkg/components` currently provides:

- square viewport calculation helpers
- explicit terminal-size requirement helpers
- a single-line text input widget
- a selectable generic list widget
- a reusable terminal image widget backed by `github.com/blacktop/go-termimg`
- panel, progress-bar, and empty-state render helpers

These primitives are intentionally Bubble Tea-friendly but Musicon-agnostic, so `internal/ui` can compose them into queue and playback screens without embedding domain logic into the shared package.

## Decisions

- Chose `pkg/components` for reusable widgets because the user explicitly requested that generic UI components live outside `internal/ui`.
- Chose stateless render helpers for panels, progress, and empty states while keeping stateful behavior in `Input` and `List` because those are the generic widgets that benefit most from reusable update logic.
- Chose a generic cached terminal-image component in `pkg/components` over embedding `go-termimg` calls directly in playback mode because the user wanted protocol-aware image rendering to stay reusable while `internal/ui` only supplies artwork-specific data and fallback messaging.
