# Responsibility

This node owns reusable UI building blocks that can be shared by Musicon screens without depending on application-specific queue or playback behavior.

Examples include:

- generic panels
- lists
- progress bars
- empty-state renderers
- terminal image rendering widgets
- focusable layout helpers

# Boundaries

This node is not responsible for Musicon-specific screen orchestration, mode transitions, or backend-facing contracts.

Those concerns belong in `internal/ui`.
