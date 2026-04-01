# Musicon

Musicon is a terminal music player for local libraries, internet radio, and YouTube Music.



https://github.com/user-attachments/assets/a77e6c1c-e165-435a-afcc-14bf803cef58



It combines:

- a Bubble Tea TUI with dedicated queue and playback modes
- local-library search and playback
- internet radio search and live playback through Radio Browser
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
- `--config-path=<path>` loads config from an explicit TOML path instead of the default XDG user config path
- `--audio-backend=<name>` overrides the configured audio backend
- `--image-backend=<name>` overrides the configured image renderer

Examples:

```bash
go run . --list-backends
go run . --config-path=/tmp/musicon.toml
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

### Internet radio

Musicon can also search Radio Browser for live stations and play them through a
mix of direct in-process decoding and native Go live-stream decoding when
needed.

The radio path is:

1. search Radio Browser by station name and tags
2. keep healthy stations in results, including HLS-backed entries
3. resolve playback through Radio Browser's click-count endpoint
4. decode direct MP3/Ogg/Vorbis/WAV streams in-process when possible
5. decode HLS AAC / Opus streams through `gohlslib` and decode ADTS AAC streams through `go-aac` while preserving live non-seekable playback behavior

## Configuration

Musicon loads TOML configuration from:

- `--config-path=<path>`, when passed on startup
- `$MUSICON_CONFIG`, when set to an explicit TOML file path
- otherwise `$XDG_CONFIG_HOME/musicon/config.toml` or `~/.config/musicon/config.toml`

The repository root also includes a complete example config in `musicon.toml`.

The config surface includes:

- audio backend selection
- UI start mode, semantic theme palette, and optional external theme file
- album-art renderer / fill mode
- local-library roots
- Radio Browser source options such as API base URL and search result limits
- YouTube source options such as cookies, cache directory, extra `yt-dlp` args,
  and search result limits
- configurable global, queue, and playback keybindings

A complete example with every supported option set to its normal default value:

```toml
[audio]
backend = "auto"

[ui]
start_mode = "queue"
cell_width_ratio = 0.5

[ui.theme]
file = ""
background = "235"
surface = "236"
surface_variant = "238"
primary = "63"
on_primary = "230"
text = "252"
text_muted = "246"
text_subtle = "244"
border = "240"
warning = "52"
on_warning = "230"

[ui.album_art]
fill_mode = "fill"
backend = "halfblocks"
protocol = "halfblocks"

[keybinds.global]
quit = ["ctrl+c"]
toggle_mode = ["tab"]
toggle_help = ["?"]

[keybinds.queue]
toggle_search_focus = ["ctrl+f"]
source_prev = ["["]
source_next = ["]"]
cycle_search_mode = ["m"]
mode_songs = ["1"]
mode_artists = ["2"]
mode_albums = ["3"]
mode_playlists = ["4"]
expand_selected = ["e"]
activate_selected = ["enter"]
move_selected_up = ["ctrl+k"]
move_selected_down = ["ctrl+j"]
clear_queue = ["ctrl+x"]
remove_selected = ["x"]
browser_up = ["up", "k"]
browser_down = ["down", "j"]
browser_home = ["home"]
browser_end = ["end"]
browser_page_up = ["pgup"]
browser_page_down = ["pgdown"]

[keybinds.playback]
cycle_pane = ["v"]
toggle_info = ["i"]
toggle_repeat = ["r"]
toggle_stream = ["s"]
toggle_pause = ["space"]
previous_track = ["["]
next_track = ["]"]
seek_backward = ["left"]
seek_forward = ["right"]
volume_down = ["-"]
volume_up = ["=", "+"]

[sources.local]
dirs = ["~/Music"]

[sources.youtube]
enabled = true
max_results = 20
cookies_file = ""
cookies_from_browser = ""
extra_args = []
cache_dir = "~/.cache/musicon/youtube"

[sources.radio]
enabled = true
max_results = 20
base_url = "https://all.api.radio-browser.info"
```

`[ui.theme]` is the new theming surface. You can either set all semantic roles
inline or point `file` at a TOML palette file and then override only the roles
you want in the main config. Relative `file` paths are resolved relative to the
config file that declares them. Legacy `theme = "default"` configs are still
accepted as a compatibility alias for the built-in default palette.

On Linux, `~/Music` and `~/.cache/musicon/youtube` correspond to the normal
per-user default library and cache locations.

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
