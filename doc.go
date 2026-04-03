// Package main contains Musicon's executable entrypoint.
//
// This layer is intentionally thin. It owns process-start concerns such as:
//   - parsing flags
//   - loading and normalizing configuration
//   - constructing concrete runtime services
//   - composing those services into the terminal UI
//   - restoring and persisting app-owned session state
//
// The rest of the repository is split so that reusable packages stay free of
// process policy, while internal packages can focus on one subsystem at a time.
package main
