# Logic

`pkg/components` currently provides:

- square viewport calculation helpers
- explicit terminal-size requirement helpers
- a single-line text input widget
- a selectable generic list widget
- a reusable terminal image widget backed by `github.com/ploMP4/chafa-go`
- panel, progress-bar, and empty-state render helpers
- a shared `Theme` struct describing semantic color roles such as surface, primary, warning, text, and border

These primitives are intentionally Bubble Tea-friendly but Musicon-agnostic, so `internal/ui` can compose them into queue and playback screens without embedding domain logic into the shared package.

The reusable `Input` widget is backed by the Bubbles `textinput` component (`charm.land/bubbles/v2/textinput`), providing cursor movement, word-level deletion, paste support, and standard Emacs-style editing shortcuts. The widget disables suggestion-related keybindings (tab, up/down) at construction time to avoid conflicting with host screen navigation. The wrapper preserves the same `bool`-returning `Update` API so callers only need to know whether the value changed, while the textinput's cursor blink commands are discarded because the queue screen's View chain returns strings rather than `tea.View` structs.

The square viewport helpers now support visually square layouts under non-square terminal cells. Callers can supply a cell width-to-height ratio so a font whose cells are taller than they are wide produces a wider-in-columns, shorter-in-rows viewport that still looks like a square on screen.

The reusable `List` widget is backed by the Bubbles `list` component (`charm.land/bubbles/v2/list`), providing built-in pagination, cursor navigation, and page-based scrolling. The widget wraps `list.Model` with a custom `listDelegate` that preserves the original row rendering layout: prefix marker ("▸ " or "  "), optional leading indicator, title+subtitle, right-aligned meta, and focus/selection styling. All built-in chrome (title bar, filter, status bar, help, pagination dots) is disabled. The wrapper preserves the same `bool`-returning `Update` API by comparing the inner model's `Index()` before and after forwarding key press messages, discarding the `tea.Cmd` return. Items are wrapped in a `listEntry` type implementing `list.Item` (FilterValue returns Title). The delegate holds a `focused` flag via pointer sharing so `SetFocused` propagates to the rendering layer, and now also holds a semantic `Theme` so focused selection, muted metadata, and unfocused rows can all pick their colors from one shared palette. When the selected label is wider than the available left column and the list itself is focused, the delegate now renders only the trailing title/subtitle region through a marquee helper instead of a static ellipsis. Any leading/source prefix stays anchored at the left edge, and the right-side meta column stays fixed, which keeps row identity plus durations/state badges readable while still exposing the full track name. The user rationale for this behavior is that long queue entries are hard to inspect when truncated.

The reusable `TerminalImage` widget now defaults Chafa to the Unicode halfblock/symbol renderer so artwork remains visible even in terminals that do not actually display richer graphics protocols reliably. Callers and users can still opt into `auto`, `kitty`, `sixel`, or `iterm2` through `MUSICON_IMAGE_PROTOCOL` when they want higher-fidelity rendering in a compatible terminal.

The same widget now uses a fill-oriented scaling mode by default so album art expands to occupy more of the available viewport instead of sitting centered with large margins. Users who prefer preserved framing or exact stretching can override this with `MUSICON_IMAGE_SCALE`, and that env override takes precedence even when the application passes explicit render settings from config.

The widget now also supports explicit construction-time render settings for protocol and scale mode. This keeps terminal-image behavior reusable while letting the application move those knobs into TOML-backed startup config instead of requiring every caller to rely on env variables.

The same image code now exposes canonical renderer naming and terminal-aware renderer listing based on Chafa `TermInfo` pixel-mode detection so CLI inspection and widget behavior stay aligned.

Renderer scaling keeps the existing config surface (`fill`, `stretch`, `fit`, `auto`, `none`) but now maps it onto Chafa geometry rules plus a small pre-crop step for fill-style behavior. When users do not pin `ui.cell_width_ratio`, the shared helper now returns the fixed `0.5` fallback instead of trying to infer font metrics from the terminal.

The package source now also carries package-level and exported-symbol documentation so shared widget contracts remain visible through Go docs alongside the existing implementation-level notes.

## Decisions

- Chose `pkg/components` for reusable widgets because the user explicitly requested that generic UI components live outside `internal/ui`.
- Chose stateless render helpers for panels, progress, and empty states while keeping stateful behavior in `Input` and `List` because those are the generic widgets that benefit most from reusable update logic.
- Chose a generic cached terminal-image component in `pkg/components` over embedding Chafa calls directly in playback mode because the user wanted protocol-aware image rendering to stay reusable while `internal/ui` only supplies artwork-specific data and fallback messaging.
- Chose `github.com/ploMP4/chafa-go` over `github.com/blacktop/go-termimg` because the user reported that `go-termimg` does not actually work reliably for Kitty graphics support.
- Chose a guaranteed-visible halfblock default with an env override over always trusting richer-pixel auto-selection because local artwork can already resolve correctly while terminal protocol auto-selection still fails to display images for some users.
- Chose a fill-oriented default scale mode with an env override over a conservative fit-only default because the user explicitly preferred artwork that uses the available square more aggressively.
- Chose explicit image render settings on the reusable component over teaching `internal/ui` to translate config directly into renderer-library calls because reusable renderer policy still belongs in `pkg/components`.
- Chose to keep `MUSICON_IMAGE_PROTOCOL` and `MUSICON_IMAGE_SCALE` as highest-precedence runtime overrides even after adding config-backed settings because the app already treats `MUSICON_CELL_WIDTH_RATIO` the same way and ad hoc terminal debugging remains valuable.
- Chose to keep renderer canonicalization and capability listing alongside the reusable image component over reimplementing that logic in `main.go` because the widget and CLI must agree on the same renderer vocabulary and availability rules.
- Chose to fall back to a fixed `0.5` cell-width ratio when config does not set one because the user explicitly asked to keep configured values only when set and otherwise use a stable default fallback during the Chafa migration.
- Chose the Bubbles `list` component over the hand-rolled list because the user requested it. The wrapper disables all built-in chrome (filtering, title, status bar, help, pagination dots, quit keys) and uses a custom `listDelegate` for row rendering to preserve the existing visual layout. A custom `ListKeyMap` is mapped onto the inner model's `KeyMap`, with all filter/help/quit bindings disabled. Page-based scrolling replaces the previous smooth scrolling behavior.
- Chose focused-row marquee scrolling over permanently widening list rows or replacing the meta column because the user wanted long queue entries to become readable when hovered/focused without sacrificing the compact queue layout or hiding durations/status on the right; later constrained the marquee to scroll only the trailing title/subtitle because the user wanted the source prefix to remain visually anchored.
- Chose the Bubbles `textinput` component over the hand-rolled input because the user requested it and it provides cursor movement, word-level operations, and paste support out of the box. Suggestion-related keybindings (tab, up/down) are disabled at construction to avoid conflicts with host screen navigation. The wrapper discards cursor blink commands because the queue screen's View chain uses strings, not `tea.View` structs, so virtual cursor state is not propagated.
- Chose an explicit cell width ratio input for square viewport math instead of assuming terminal cells are square because the user observed the visual frame distortion caused by tall terminal glyphs.
- Chose a generic leading marker field on list items instead of hard-coding queue icons into the widget because callers may need lightweight row state cues without turning the shared list into a Musicon-specific queue component.
- Chose an explicit `SetSelectedIndex` hook on the shared list instead of forcing every caller to infer selection through synthetic key events because identity-preserving rebuilds are generic widget behavior, not queue-specific logic.
- Chose a configurable list keymap over hardcoding `up/down/j/k` forever because the user wanted all existing UI shortcuts to be configurable from TOML and list navigation is part of that surface.
- Chose a shared semantic `Theme` type plus themed panel/progress/empty-state helpers over scattering raw Lip Gloss colors through each renderer because the user explicitly wanted matugen-style coordinated colors across both reusable widgets and Musicon-specific screens.
