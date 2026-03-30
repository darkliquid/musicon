# Public API

This parent node delegates concrete APIs to focused child nodes:

- `pkg/coverart/core` for metadata, result, chain, and cache contracts
- `pkg/coverart/providers` for local and remote source implementations

# Contracts

- Child nodes together preserve the requested local-first and user-freedom-first lookup policy.
- The parent node exists to keep the reusable package boundary visible even though the implementation is split for graph sizing.
