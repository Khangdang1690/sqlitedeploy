#!/usr/bin/env bash
# Verify `go install` from the local checkout produces a working sqlitedeploy
# binary. This is the local stand-in for what users get when they run
#     go install github.com/Khangdang1690/sqlitedeploy/cmd/sqlitedeploy@latest
# against a tagged release.
#
# Self-contained: stamps a SemVer-pre-release test version via -ldflags, points
# GOBIN at a scratch dir, runs the installed binary, and confirms
# `sqlitedeploy --version` echoes the same string.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TEST_VERSION="0.0.0-test"
SCRATCH="$REPO_ROOT/test/.scratch/go"

case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*) BIN_NAME="sqlitedeploy.exe" ;;
    *)                    BIN_NAME="sqlitedeploy" ;;
esac

cleanup() {
    rc=$?
    echo "--- go test cleanup"
    rm -rf "$SCRATCH"
    if [ "$rc" -eq 0 ]; then echo "PASS  test-go"; else echo "FAIL  test-go (rc=$rc)"; fi
    exit "$rc"
}
trap cleanup EXIT

echo "--- go install test (version=$TEST_VERSION)"

mkdir -p "$SCRATCH/bin"
export GOBIN="$SCRATCH/bin"

echo "[1/3] go install ./cmd/sqlitedeploy (GOBIN=$GOBIN)"
(cd "$REPO_ROOT" && go install -ldflags="-X main.version=$TEST_VERSION" ./cmd/sqlitedeploy)

if [ ! -x "$GOBIN/$BIN_NAME" ]; then
    echo "    binary not found at $GOBIN/$BIN_NAME" >&2
    ls -la "$GOBIN" >&2
    exit 1
fi

echo "[2/3] exec sqlitedeploy --version"
ACTUAL="$("$GOBIN/$BIN_NAME" --version)"
EXPECTED="sqlitedeploy version $TEST_VERSION"
if [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "    expected: $EXPECTED"
    echo "    got:      $ACTUAL"
    exit 1
fi
echo "    output matches: $ACTUAL"

echo "[3/3] sanity check --help (verifies cobra wired correctly)"
"$GOBIN/$BIN_NAME" --help >/dev/null
echo "    --help exits 0"
