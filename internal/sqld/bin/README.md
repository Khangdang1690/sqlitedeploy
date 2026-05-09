# Bundled sqld (libsql-server) binaries

This directory holds the platform-specific `sqld` binaries that get embedded
into the `sqlitedeploy` CLI at build time, one per
`{linux,darwin} x {amd64,arm64}` (4 binaries — Windows is unsupported, see
below). `sqld` is the open-source server
binary from [tursodatabase/libsql](https://github.com/tursodatabase/libsql)
(formerly the standalone `sqld` repo). It speaks Hrana over HTTP/WebSocket so
edge runtimes (Cloudflare Workers, Lambda, Vercel) can connect, and ships with
**bottomless** replication that backs up to any S3-compatible bucket.

These binaries **are committed to the repository** so that builds via
`go install github.com/Khangdang1690/sqlitedeploy/cmd/sqlitedeploy@latest`
produce a self-contained CLI without requiring contributors to first run a
build step. Repo size grows by ~40 MB across the four binaries.

## Refreshing the binaries

Unlike Litestream (which we replaced), `sqld` doesn't ship Windows prebuilts
upstream and gates `bottomless` behind a Cargo feature, so we build all six
binaries from source ourselves.

When bumping `LIBSQL_VERSION` in the top-level `Makefile`:

1. Fetch the source: `make fetch-libsql-source` (clones the pinned tag into
   `build/libsql/`, gitignored).
2. Build for the host platform locally: `make build-sqld` (validates the
   recipe; needs Rust toolchain on the host).
3. Cross-platform binaries are produced by CI on native runners — see
   `.github/workflows/release.yml`. Don't try to cross-compile all 6 from one
   developer machine; download the artifacts from a release run instead.
4. Commit the updated files alongside the version bump:
   `git add internal/sqld/bin/ && git commit`.

If the binaries are missing or unreadable, the runtime falls back to looking
for a `sqld` executable on `$PATH` (see [../runner.go](../runner.go) — same
fallback pattern as the previous Litestream integration).

## Why we vendor source instead of upstream prebuilts

Upstream prebuilts may not have all the build options we want enabled, and
we want a deterministic, reproducible recipe pinned to a known tag. Building
from source ourselves makes the supply chain auditable.

Note on `bottomless`: it's pulled in as an unconditional path dependency by
libsql-server, so no feature flag is required at compile time. The
`--enable-bottomless-replication` runtime flag is what actually toggles
replication on or off.

## Why no Windows

libsql-server's `libsql-wal` dependency uses POSIX-only syscalls (`pwrite`,
`pwritev`, `std::os::unix::ffi::OsStrExt`) with no Windows fallbacks and no
Cargo feature flag to opt out. Upstream itself does not ship Windows release
binaries. Building on Windows fails with `error[E0433]: failed to resolve:
could not find 'unix' in 'os'`. Windows users should run `sqlitedeploy` and
their app inside WSL2.
