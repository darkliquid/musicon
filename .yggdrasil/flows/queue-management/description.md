# Business context

Musicon is meant to be a simple, comfortable terminal interface for playing music from multiple sources.

Queue management is the discovery and curation workflow that lets the user find music, add it to the playback queue, inspect what will play next, remove items, and clear the queue when needed.

# Trigger

The user enters queue mode to search for music or adjust what should play next.

# Goal

Build a queue that reflects what the user wants to hear now while keeping the interaction fast and low-friction inside the terminal.

# Participants

- `app/cli` starts the terminal application and hands control to the UI.
- `internal/ui` renders the queue screen, collects user input, and routes actions to backend-facing contracts.
- `internal/sources/local` provides concrete local-file search results and queueable metadata.

# Paths

## Happy path

1. The user enters queue mode.
2. The user chooses or cycles a source and enters search text.
3. The UI asks the active source contract for matching items.
4. The user reviews search results and adds one or more items to the queue.
5. The user inspects the queue and optionally removes or clears entries.

## Alternate path: direct queue cleanup

1. The user enters queue mode with no search intent.
2. The user moves focus to the queue panel.
3. The user removes selected entries or clears the queue.

# Invariants across all paths

- Queue interactions stay within the square application frame.
- Queue management remains a dedicated mode, separate from playback mode.
- Source-specific search behavior is delegated through interfaces rather than implemented directly in the UI layer.
- Real source implementations should preserve enough metadata for later playback and artwork resolution, not just display labels.
