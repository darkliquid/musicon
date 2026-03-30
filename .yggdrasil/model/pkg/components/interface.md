# Public API

The node currently exposes generic rendering primitives and widgets including:

- `ClampSquare`, `ClampSquareWithCellWidthRatio`, and `SquareViewport` for square-frame layout calculations
- `SizeRequirements` and `SizeCheck` for explicit terminal-size validation
- `Input` with `Update`, `View`, `SetSize`, and `SetFocused`
- `List` with `SetItems`, `Update`, `View`, `SetSize`, and `SetFocused`, plus optional leading markers on items
- `ImageSource` plus `TerminalImage` with `SetSource`, `SetSize`, `View`, `Ready`, and `Error`
- `RenderPanel`, `RenderProgress`, and `RenderEmptyState` helpers

# Contracts

- Components accept plain data and presentation state rather than Musicon-specific domain interfaces.
- Components clamp invalid sizes, preserve stable selection state where possible, and render meaningful empty states when no content is available.
- Square viewport helpers must support visually square layouts under non-square terminal cells by allowing callers to supply a cell width-to-height ratio.
- The generic list widget must allow callers to prepend a lightweight visual marker per row without taking on domain-specific queue/search semantics itself.
- Components avoid direct knowledge of source search, queue mutation, or playback-engine behavior.
- Image rendering components should accept encoded image data and own terminal-protocol concerns internally rather than forcing screen code to call renderer libraries directly.

# Failure modes

- Invalid sizes clamp to safe minimums.
- Empty content renders a stable placeholder instead of panicking.
- Unsupported image data or terminal-rendering failures surface through component error state instead of crashing playback views.
