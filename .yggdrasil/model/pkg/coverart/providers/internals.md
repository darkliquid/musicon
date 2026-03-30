# Logic

`pkg/coverart/providers` is now a parent node over two focused child nodes:

- `pkg/coverart/providers/local`
- `pkg/coverart/providers/remote`

## Decisions

- Split the concrete providers again into local and remote child nodes because adding Last.fm pushed the provider area past the preferred node size.
