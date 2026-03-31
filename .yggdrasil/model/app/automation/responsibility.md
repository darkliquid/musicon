# Responsibility

This node owns repository automation for Musicon.

It is responsible for GitHub Actions workflows that verify the Go codebase on push and pull request, run repository-level lint and vulnerability checks, and publish tagged cross-platform binaries through Goreleaser.

# Boundaries

This node is not responsible for application runtime behavior, CLI startup wiring, UI logic, or the internals of audio, source, or metadata services.

It defines how the repository is validated and packaged, not how the player itself behaves at runtime.
