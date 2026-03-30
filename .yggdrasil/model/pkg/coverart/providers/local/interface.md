# Public API

This node exposes:

- `LocalFilesProvider` / `NewLocalFilesProvider()`
- `EmbeddedProvider`

# Contracts

- Local providers must outrank every remote provider in the chain.
- Providers return `ErrNotFound` for misses and surface real file or tag parsing failures.
