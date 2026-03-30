# Business context

Playback mode is the comfortable listening surface of Musicon.

It centers the currently playing item visually, exposes transport and volume controls, and lets the user switch the main presentation between album art, lyrics, and visualization-oriented panes without leaving the terminal.

# Trigger

The user enters playback mode to monitor or control what is currently playing.

# Goal

Make core playback actions and status easy to understand at a glance while prioritizing an album-art-first experience.

# Participants

- `app/cli` starts the application loop.
- `internal/ui` renders playback state, accepts key input, and delegates transport requests through UI-facing contracts.
- `internal/audio` resolves queue entries into playable streams and manages actual audio output.
- `internal/mpris` mirrors playback state to desktop media controls and forwards remote transport requests back into Musicon.

# Paths

## Happy path

1. The user enters playback mode.
2. The UI shows the current track inside the square playback layout.
3. The user uses keyboard controls to play, pause, skip, seek, or adjust volume.
4. The audio runtime applies the transport request to the active output stream and returns updated playback state to the UI.
5. The MPRIS bridge reflects the updated playback state to the desktop session when available.
6. The user optionally toggles metadata, help, or an alternate center pane such as lyrics or visualization.

## Alternate path: information-first viewing

1. The user enters playback mode while music is already active.
2. The user switches the center pane to lyrics or another placeholder view.
3. The UI shows empty-state messaging when the requested data is unavailable from the backend contract.

# Invariants across all paths

- Playback remains a dedicated top-level mode.
- The primary layout stays inside the square application frame.
- Non-artwork panes exist as UI surfaces with backend hooks, even when no real data is supplied yet.
- Playback transport depends on a concrete runtime capable of turning queue items into live output.
- Desktop media controls must not become an alternate source of truth; they reflect and control the same playback runtime used by the terminal UI.
