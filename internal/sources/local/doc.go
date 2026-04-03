// Package local implements Musicon's filesystem-backed source.
//
// It handles two closely related responsibilities:
//   - indexing local files into UI-facing search results
//   - resolving those same results into playable decoded streams
//
// Keeping both halves together lets local metadata, cover-art hints, and file
// paths flow naturally from discovery into playback.
package local
