# Public API

This parent node delegates concrete APIs to focused child nodes:

- `pkg/coverart/providers/local` for local-file and embedded-art providers
- `pkg/coverart/providers/remote` for remote metadata-service providers

# Contracts

- Child nodes together preserve the requested local-first provider order and optional remote fallback behavior.

# Failure modes

- Failure modes are defined on the child nodes that own the concrete providers.
