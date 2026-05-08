# Packaging integration tests

This directory verifies that `npm i sqlitedeploy` and `pip install sqlitedeploy`
both produce a working CLI on the host platform — without publishing to any
real registry.

## What's tested

The packaging mechanics: stamping versions, packing tarballs/wheels, npm's
`optionalDependencies` resolution, and the resolver shims that locate and
exec the bundled Go binary.

We do **not** test:
- Replication against real R2/B2/S3 (needs live credentials).
- Cross-platform builds (CI's matrix covers that — we test the host's
  platform only).
- The Go CLI's own commands (those have their own `go test ./...`).

## Layout

```
test/
├── run-all.sh             # entry point — runs both integration tests
├── lib/platform.sh        # shared host-platform detection
├── integration/
│   ├── test-npm.sh        # pack & install the npm wrapper, run the CLI
│   └── test-pip.sh        # build & install the pip wheel, run the CLI
└── fixtures/              # illustrative consumer apps (not run automatically)
    ├── nextjs/            # how a Next.js / TS app would consume it
    └── fastapi/           # how a FastAPI / Python app would consume it
```

## Running the tests

```bash
# 0. one-time: build the Go binary for your host platform
go build -ldflags="-X main.version=0.0.0-test" -o dist/sqlitedeploy.exe ./cmd/sqlitedeploy
#   (drop the .exe on macOS / Linux)

# 1. run everything
bash test/run-all.sh

# or run one at a time
bash test/integration/test-npm.sh
bash test/integration/test-pip.sh
```

A successful run prints `PASS` for each test and exits 0.

## Design choices

**Self-contained.** Each test creates its scratch directory under
`test/.scratch/` (gitignored) and tears it down on exit, so re-running is
idempotent. The repo's source manifests are stamped to `0.0.0-test`,
verified, and reset to `0.0.0` on exit — even on failure (via `trap`).

**Host-platform only.** Cross-compiling six binaries on every developer
laptop is wasteful; CI handles all six on tagged releases. The tests detect
the host's `os/arch`, expect `dist/sqlitedeploy[.exe]` to exist, and
exercise the matching package only.

**No global installs.** Hatch lives in `.venv-tools/` (created by
`run-all.sh`), the wheel is installed into a per-test venv under
`test/.scratch/`, and npm packages install into a scratch `node_modules/`.
Nothing reaches the user's global Python or global npm.

**Fixtures are illustrative, not automated.** Real Next.js / FastAPI apps
pull in their own native deps (`better-sqlite3` builds against Node ABI;
`uvicorn` is a heavy install) and don't make the wrapper any more tested.
The fixtures show a developer what minimal usage looks like; the
integration scripts above prove the wrapper itself works.
