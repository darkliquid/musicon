# Logic

`pkg/coverart/providers/local` implements the two highest-priority artwork strategies:

1. sibling local cover files
2. embedded local artwork

## Decisions

- Split local providers from remote providers so the cover-art graph stays within node-size limits while preserving the requested local-first priority model.
