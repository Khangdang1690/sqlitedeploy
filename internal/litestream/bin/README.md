# Bundled Litestream binaries

This directory holds the platform-specific `litestream` binaries that get
embedded into the `sqlitedeploy` CLI at build time, one per
`{linux,darwin,windows} x {amd64,arm64}`.

These binaries **are committed to the repository** so that builds via
`go install github.com/Khangdang1690/sqlitedeploy/cmd/sqlitedeploy@latest`
produce a self-contained CLI without requiring contributors to first run a
fetch step. Repo size grows by ~120 MB across the six binaries.

## Refreshing the binaries

When bumping `LITESTREAM_VERSION` in the top-level `Makefile`:

1. Run `make fetch-litestream` (or `pwsh scripts\fetch-litestream.ps1` on
   Windows). This downloads each platform's binary from the upstream
   Litestream release and writes them here.
2. Commit the updated files alongside the version bump:
   `git add internal/litestream/bin/ && git commit`.

If the binaries are missing or unreadable, the runtime falls back to looking
for a `litestream` executable on `$PATH` (see
[../runner.go](../runner.go) — `placeholderPrefix` and
`Resolve`).
