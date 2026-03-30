# Responsibility

This node owns the reusable core abstractions for cover-art resolution.

It is responsible for:

- normalized lookup metadata
- result and image payload shapes
- provider and chain interfaces
- cache contracts and disk-cache primitives

# Boundaries

This node is not responsible for:

- concrete local or remote provider behavior
- terminal rendering
- Musicon-specific UI adaptation

Those concerns belong to sibling provider nodes, `pkg/components`, and `internal/ui`.
