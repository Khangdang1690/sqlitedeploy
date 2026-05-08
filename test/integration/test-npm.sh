#!/usr/bin/env bash
# Verify the npm wrapper installs and execs the bundled binary correctly.
#
# Self-contained: builds its own binary with a SemVer test version, stamps
# the manifests, packs both packages, installs them in a scratch project,
# and confirms `sqlitedeploy --version` echoes the same string.
#
# Cleans up by restoring manifests on exit (even on failure).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
# SemVer pre-release format — npm requires this; pip would reject it
# (PEP 440 forbids the hyphen). The pip test uses its own PEP-compliant
# version.
TEST_VERSION="0.0.0-test"
TEST_BIN_DIR="$REPO_ROOT/test/.scratch/npm-dist"

# Build the binary fresh so its embedded version matches what we'll stamp
# into the manifests. Done before sourcing platform.sh because that script
# requires the binary to already exist.
case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*) BIN_NAME="sqlitedeploy.exe" ;;
    *)                    BIN_NAME="sqlitedeploy" ;;
esac
mkdir -p "$TEST_BIN_DIR"
echo "--- building test binary (version=$TEST_VERSION)"
go build -ldflags="-X main.version=$TEST_VERSION" \
    -o "$TEST_BIN_DIR/$BIN_NAME" \
    ./cmd/sqlitedeploy

export TEST_BIN_DIR
source "$REPO_ROOT/test/lib/platform.sh"

SCRATCH="$REPO_ROOT/test/.scratch/npm"
PLATFORM_DIR="$REPO_ROOT/packaging/npm/platforms/$HOST_NPM_KEY"
MAIN_DIR="$REPO_ROOT/packaging/npm/sqlitedeploy"

cleanup() {
    rc=$?
    echo "--- npm test cleanup"
    node "$REPO_ROOT/scripts/stamp-versions.js" 0.0.0 >/dev/null
    rm -rf "$SCRATCH" "$TEST_BIN_DIR" "$PLATFORM_DIR/bin"
    if [ "$rc" -eq 0 ]; then echo "PASS  test-npm"; else echo "FAIL  test-npm (rc=$rc)"; fi
    exit "$rc"
}
trap cleanup EXIT

echo "--- npm test (host=$HOST_NPM_KEY, version=$TEST_VERSION)"
echo "    binary: $HOST_BIN_PATH"

echo "[1/6] stamp test version into manifests"
node "$REPO_ROOT/scripts/stamp-versions.js" "$TEST_VERSION" >/dev/null

echo "[2/6] copy host binary into platform package"
mkdir -p "$PLATFORM_DIR/bin"
cp "$HOST_BIN_PATH" "$PLATFORM_DIR/bin/$HOST_BIN_NAME"
chmod +x "$PLATFORM_DIR/bin/$HOST_BIN_NAME" || true

echo "[3/6] npm pack platform + main"
mkdir -p "$SCRATCH"
PLATFORM_TGZ="$(cd "$PLATFORM_DIR" && npm pack --silent --pack-destination "$SCRATCH")"
MAIN_TGZ="$(cd "$MAIN_DIR" && npm pack --silent --pack-destination "$SCRATCH")"
PLATFORM_TGZ="$SCRATCH/$PLATFORM_TGZ"
MAIN_TGZ="$SCRATCH/$MAIN_TGZ"
echo "    platform tarball: $(basename "$PLATFORM_TGZ")"
echo "    main tarball:     $(basename "$MAIN_TGZ")"

echo "[4/6] install in scratch project"
APP="$SCRATCH/app"
mkdir -p "$APP"
(cd "$APP" && npm init -y >/dev/null 2>&1)
(cd "$APP" && npm install --silent --no-audit --no-fund \
    "$PLATFORM_TGZ" "$MAIN_TGZ" >/dev/null)

echo "[5/6] exec sqlitedeploy --version"
# node_modules/.bin/sqlitedeploy must exist — if both the main package and a
# platform package declare `bin: { sqlitedeploy: ... }`, npm refuses to
# create the symlink and `npx`/`PATH` falls through to whatever else is
# installed. This caught the rc.2 regression.
if [ ! -e "$APP/node_modules/.bin/sqlitedeploy" ] && \
   [ ! -e "$APP/node_modules/.bin/sqlitedeploy.cmd" ]; then
    echo "    node_modules/.bin/sqlitedeploy missing — bin conflict between" >&2
    echo "    main and platform packages? Check for duplicate \"bin\" fields." >&2
    ls -la "$APP/node_modules/.bin/" >&2
    exit 1
fi
ACTUAL="$(cd "$APP" && npx --no-install sqlitedeploy --version)"
EXPECTED="sqlitedeploy version $TEST_VERSION"
if [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "    expected: $EXPECTED"
    echo "    got:      $ACTUAL"
    exit 1
fi
echo "    output matches: $ACTUAL"

echo "[6/6] sanity check --help (verifies exit-code forwarding)"
(cd "$APP" && npx --no-install sqlitedeploy --help >/dev/null)
echo "    --help exits 0"
