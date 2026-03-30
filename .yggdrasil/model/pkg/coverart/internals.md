# Logic

`pkg/coverart` is now a small parent node over two focused child nodes.

The split is:

- `pkg/coverart/core` for reusable metadata, chain, and cache contracts
- `pkg/coverart/providers` for concrete local and remote source implementations

This preserves the reusable package boundary while keeping each node within focused sizing limits.

## Decisions

- Chose `pkg/coverart` over `internal/coverart` because the user expects the cover-art service to be reusable outside Musicon-specific playback wiring.
- Split the node after implementation because the package grew beyond the preferred node size and naturally separated into reusable core abstractions vs concrete provider strategies.
