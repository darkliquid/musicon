# Public API

The node currently exposes generic rendering primitives and widgets including:

- `ClampSquare` and `SquareViewport` for square-frame layout calculations
- `SizeRequirements` and `SizeCheck` for explicit terminal-size validation
- `Input` with `Update`, `View`, `SetSize`, and `SetFocused`
- `List` with `SetItems`, `Update`, `View`, `SetSize`, and `SetFocused`
- `RenderPanel`, `RenderProgress`, and `RenderEmptyState` helpers

# Contracts

- Components accept plain data and presentation state rather than Musicon-specific domain interfaces.
- Components clamp invalid sizes, preserve stable selection state where possible, and render meaningful empty states when no content is available.
- Components avoid direct knowledge of source search, queue mutation, or playback-engine behavior.

# Failure modes

- Invalid sizes clamp to safe minimums.
- Empty content renders a stable placeholder instead of panicking.
