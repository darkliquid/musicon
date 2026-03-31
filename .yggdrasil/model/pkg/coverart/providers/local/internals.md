# Logic

`pkg/coverart/providers/local` implements the two highest-priority artwork strategies:

1. sibling local cover files
2. embedded local artwork

Sibling-cover lookup now scans the audio file's directory and matches configured base names and extensions case-insensitively instead of guessing a few exact filename spellings. This keeps local-first artwork resolution reliable across common filesystem naming variations such as `cover.jpg`, `Cover.JPG`, or `FOLDER.PNG`.

The package source now also carries exported-symbol documentation so the local provider entry points remain clear from Go docs in addition to the package-level description.

## Decisions

- Split local providers from remote providers so the cover-art graph stays within node-size limits while preserving the requested local-first priority model.
