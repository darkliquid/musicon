# Responsibility

This node owns concrete audio playback runtime behavior for Musicon.

It is responsible for:

- initializing and closing the output pipeline
- using mago-backed playback output as the physical playback surface
- using `github.com/gopxl/beep` streamers and controls for decode/playback composition
- exposing queue-aware playback state and transport controls through interfaces consumed by `internal/ui`
- honoring configured audio backend selection when initializing the playback device

# Boundaries

This node is not responsible for:

- rendering terminal UI
- owning generic UI widgets
- implementing source discovery or search
- deciding screen layout or keyboard help
