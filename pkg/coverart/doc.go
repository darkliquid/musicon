// Package coverart provides reusable cover-art resolution primitives.
//
// It models the data and provider contracts needed to look up artwork from local
// files, embedded metadata, remote metadata URLs, and third-party services. The
// package is reusable on purpose so the UI can depend on a narrow adapter rather
// than embedding provider-chain logic directly into screen code.
package coverart
