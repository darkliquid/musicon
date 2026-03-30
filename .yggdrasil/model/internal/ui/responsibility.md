# Responsibility

This node owns the Musicon Bubble Tea user interface.

It is responsible for:

- the root application model
- queue and playback screen composition
- square viewport layout rules
- in-square help and playback overlay composition
- keymaps, focus, and mode transitions
- frontend-facing interfaces for queue data, source search, playback snapshots, normalized artwork lookup metadata, lyrics, and visualization placeholders

# Boundaries

This node is not responsible for:

- implementing music sources
- performing audio playback
- persisting queue state
- fetching remote content directly
- housing reusable generic widgets that are not tightly coupled to Musicon internals

Reusable UI primitives belong in `pkg/components`.
