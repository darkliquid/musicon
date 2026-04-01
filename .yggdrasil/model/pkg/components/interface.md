# Public API

The node currently exposes generic rendering primitives and widgets including:

- `ClampSquare`, `ClampSquareWithCellWidthRatio`, and `SquareViewport` for square-frame layout calculations
- `SizeRequirements` and `SizeCheck` for explicit terminal-size validation
- `Input` with `Update`, `View`, `SetSize`, `SetValue`, and `SetFocused`, backed by the Bubbles `textinput` component for cursor movement, word deletion, and standard editing shortcuts
- `List` with `SetItems`, `Update`, `View`, `SetSize`, `SetFocused`, and `SetSelectedIndex`, backed by the Bubbles `list` component for pagination, selection, and keyboard navigation, plus optional leading markers on items and marquee-style scrolling for the truncated selected title/subtitle while anchored prefixes remain fixed
- `Theme` for semantic UI color roles shared between config, reusable widgets, and Musicon-specific screens
- `ImageSource` plus `TerminalImage` with `SetSource`, `SetSize`, `View`, `Ready`, and `Error`, plus construction with explicit render settings when callers want config-driven behavior
- terminal-image renderer helpers that can canonicalize configured renderer names, compute the effective env-aware renderer selection, list the currently usable renderer backends for the active terminal, and expose the shared fallback cell-width ratio used when config does not pin one
- `RenderPanel`, `RenderProgress`, and `RenderEmptyState` helpers, each accepting an explicit semantic theme

# Contracts

- Components accept plain data and presentation state rather than Musicon-specific domain interfaces.
- Components clamp invalid sizes, preserve stable selection state where possible, and render meaningful empty states when no content is available.
- Square viewport helpers must support visually square layouts under non-square terminal cells by allowing callers to supply a cell width-to-height ratio.
- The generic list widget must allow callers to prepend a lightweight visual marker per row without taking on domain-specific queue/search semantics itself.
- The generic list widget should also let callers restore selection to a specific row index after rebuilding the item set so domain screens can preserve identity-based selection through reordering or live data refreshes.
- The generic list widget should also accept a configurable keymap for navigation actions so application-level keybinding policy does not have to be hardcoded inside the shared component.
- The generic list widget should reveal the full selected title/subtitle for truncated rows when the list itself is focused, while keeping any leading/source prefix anchored so row identity stays stable during the marquee.
- Shared component renderers should consume semantic theme roles rather than hardcoded terminal colors so config-driven palettes can restyle both reusable widgets and `internal/ui` chrome consistently.
- Components avoid direct knowledge of source search, queue mutation, or playback-engine behavior.
- Image rendering components should accept encoded image data and own terminal-protocol concerns internally rather than forcing screen code to call renderer libraries directly.
- The terminal-image component should default to a guaranteed-visible Unicode halfblock renderer and allow richer protocol selection through `MUSICON_IMAGE_PROTOCOL` (`auto`, `kitty`, `sixel`, `iterm2`, `halfblocks`).
- The terminal-image component should default to a fill-oriented scale mode so artwork occupies more of the available pane, while allowing `MUSICON_IMAGE_SCALE` (`fill`, `stretch`, `fit`, `auto`, `none`) to tune how aggressively it expands.
- The terminal-image component should also allow callers to provide explicit protocol and scale settings so application config can drive image rendering, while still letting `MUSICON_IMAGE_PROTOCOL` and `MUSICON_IMAGE_SCALE` act as highest-precedence runtime overrides.
- Image-renderer listing should use the same canonical labels and Chafa-backed capability detection as the renderer itself so CLI output matches what the widget can actually use.
- The shared terminal-image helpers should keep `ui.cell_width_ratio` on an explicit-config-or-fixed-fallback policy instead of trying to auto-detect font geometry at startup.

# Failure modes

- Invalid sizes clamp to safe minimums.
- Empty content renders a stable placeholder instead of panicking.
- Unsupported image data or terminal-rendering failures surface through component error state instead of crashing playback views.
