# Responsibility

This parent node groups reusable cover-art resolution work that is intended to be useful beyond Musicon-specific UI wiring.

For the active work area, it provides structural context for:

- the reusable cover-art core contracts and cache wrappers
- the concrete local and remote provider implementations

# Boundaries

This parent node is not responsible for direct terminal rendering or Musicon-specific playback orchestration.

Terminal rendering belongs in `pkg/components`, while Musicon-specific playback wiring belongs in `internal/ui` and runtime metadata propagation belongs in `internal/audio` or source-layer code.
