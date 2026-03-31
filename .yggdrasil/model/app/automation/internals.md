# Logic

Repository automation is intentionally separate from the CLI entrypoint because CI/CD concerns act on the repository as a whole rather than on the executable startup path.

The current implementation:

- runs a CI workflow for pushes and pull requests
- verifies formatting with `gofmt -l .`
- runs `go vet ./...` as the repository lint gate alongside formatting checks
- runs `go test ./...` as the correctness gate
- installs `govulncheck` in the runner toolchain and invokes it from `$(go env GOPATH)/bin/govulncheck`
- runs Goreleaser on version-tag pushes matching `v*`
- lets Goreleaser fetch modules with `go mod download` before packaging instead of mutating `go.mod` or `go.sum` during release
- publishes archives for Linux, macOS, and Windows targets with per-release checksums

## Decisions

- Chose a dedicated `app/automation` infrastructure node over expanding `app/cli` because CI/CD behavior is repository automation, not executable wiring.
- Chose GitHub-hosted workflows over local-only release scripts because the request was specifically to set up GitHub workflows for repeatable automation.
- Chose tag-driven releases matching `v*` because the user was unavailable for follow-up, and tag-based publishing is the safest default for explicit release intent.
- Chose built-in Go tooling (`gofmt`, `go test`) plus `govulncheck` over a broader third-party lint stack to keep the pipeline aligned with repository-native checks and reduce configuration risk.
- Chose to gate CI on `go vet` again after fixing the `internal/mpris` export issue, so the repository now enforces both formatting and static interface checks in its lint workflow.
- Chose Goreleaser archives for Linux, macOS, and Windows over bespoke shell packaging because the request explicitly called for a Goreleaser-based cross-platform release pipeline.
