# Guidance

- Compute the active viewport from the minimum terminal dimension.
- Center the square frame horizontally and vertically.
- Size queue and playback sublayouts from the square frame, not from raw terminal size.
- Keep decorative borders and overlays within the square frame so mode layouts remain predictable.
- When the terminal is below the declared minimums, render a dedicated resize message instead of a degraded partial UI.
