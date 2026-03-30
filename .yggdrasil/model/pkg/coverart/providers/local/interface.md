# Public API

This node exposes:

- `LocalFilesProvider` / `NewLocalFilesProvider()`
- `EmbeddedProvider`

# Contracts

- Local providers must outrank every remote provider in the chain.
- Sibling-cover discovery should treat configured cover base names and extensions case-insensitively so files like `Cover.JPG` resolve the same way as `cover.jpg`.
- Providers return `ErrNotFound` for misses and surface real file or tag parsing failures.
