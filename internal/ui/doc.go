// Package ui contains Musicon's Bubble Tea application shell.
//
// The package is structured around two top-level user modes:
//   - queue management for discovery and curation
//   - playback for artwork, lyrics, visualization, and transport
//
// It deliberately depends on small UI-facing service interfaces instead of
// concrete runtime implementations so the presentation layer can stay focused on
// layout, input, and state transitions rather than backend construction.
package ui
