// Package audio implements Musicon's playback runtime.
//
// This package is the bridge between source-specific resolution and the rest of
// the app's playback model. It owns queue state, active playback state, speaker
// initialization, transport control, and live analysis data used by the UI's EQ
// and visualizer panes.
package audio
