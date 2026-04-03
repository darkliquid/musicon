// Package radio implements Musicon's Radio Browser-backed internet radio source.
//
// The package has to bridge a search-oriented HTTP API with live playback
// formats that range from direct MP3/Ogg/WAV streams to HLS and AAC variants.
// Its design therefore separates source selection from stream-opening details so
// the rest of the app can treat radio like any other source.
package radio
