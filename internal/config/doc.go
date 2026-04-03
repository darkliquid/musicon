// Package config owns Musicon's TOML-backed startup configuration surface.
//
// The package exists to keep configuration policy in one place instead of
// scattering ad-hoc environment and file parsing across the runtime. It loads
// defaults, overlays user TOML, resolves paths, and normalizes user-facing
// values into typed options that the executable can pass into other packages.
package config
