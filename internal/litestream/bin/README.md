# Bundled Litestream binaries

This directory holds platform-specific `litestream` binaries that get embedded
into the `sqlitedeploy` CLI at build time.

Files are named `litestream-<os>-<arch>` (plus `.exe` on Windows). Run
`make fetch-litestream` from the project root to populate this directory with
the latest stable release. Until then, the embedded files are PLACEHOLDER text
and `sqlitedeploy` falls back to looking for `litestream` on `PATH`.
