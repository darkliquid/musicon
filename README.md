# Musicon

Musicon is a terminal music player for local libraries and YouTube Music.

It combines:

- a Bubble Tea TUI with dedicated queue and playback modes
- local-library search and playback
- YouTube Music search with `yt-dlp`-backed streaming playback
- desktop media-key integration through MPRIS
- configurable audio backends, image rendering, and keybindings

## Requirements

- Go, to build the application
- a working audio backend supported by the host system
- `yt-dlp`, for YouTube / YouTube Music playback

Optional integrations:

- Spotify, Apple Music, and Last.fm credentials for richer cover-art lookup
- an MPRIS-capable desktop session for media-key / desktop-player integration

## Running

Build and run with Go:

```bash
go run .
```

Useful startup flags:

- `--list-backends` lists usable audio backends in config-compatible form
- `--list-image-renderers` lists usable album-art renderers
- `--audio-backend=<name>` overrides the configured audio backend
- `--image-backend=<name>` overrides the configured image renderer

Examples:

```bash
go run . --list-backends
go run . --audio-backend=pulse
go run . --image-backend=kitty
```

## Features

### Queue mode

- searches across configured sources
- keeps queued items pinned while search results continue below them
- supports source switching, result filtering, queue reordering, and live search

### Playback mode

- artwork-first square layout with alternate lyrics / EQ / visualizer panes
- asynchronous transport handling so input remains responsive during backend work
- debounced absolute seeking that collapses repeated seek taps into one target
- repeat, stream-continuation, volume, previous / next, and pause controls

### Local library

Local playback is backed by configurable directory roots and preserves local file
metadata for downstream cover-art lookup.

### YouTube Music

Musicon searches YouTube Music directly, but YouTube streaming playback depends
on `yt-dlp`.

The playback path is:

1. search YouTube Music over HTTP for fast results
2. use `yt-dlp -j` to extract the final media URL and request headers
3. perform direct HTTP range reads inside Musicon
4. decode WebM/Opus in-process with the pure-Go playback pipeline

YouTube playback keeps a bounded PCM window in memory for cheap nearby seeks and
uses shared file-backed range caching to support farther replacement-stream
seeks without redownloading every previously visited block.

## Configuration

Musicon loads TOML configuration from its default XDG search path, or from an
explicit file path when configured externally.

The config surface includes:

- audio backend selection
- UI start mode and theme
- album-art renderer / fill mode
- local-library roots
- YouTube source options such as cookies, cache directory, extra `yt-dlp` args,
  and search result limits
- configurable global, queue, and playback keybindings

The default config filename is:

```text
musicon.toml
```

## Cover art

Musicon uses a local-first cover-art chain. Depending on available metadata and
credentials, it can look up artwork from:

- local cover files
- embedded artwork
- MusicBrainz
- Spotify
- Apple Music
- Last.fm

## Documentation

The codebase now includes package-level docs and concise exported-symbol
comments across the main implementation packages. The YouTube streaming path and
other non-obvious internals also have inline comments where the control flow is
harder to infer from code alone.
