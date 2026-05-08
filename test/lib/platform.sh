#!/usr/bin/env bash
# Shared host-platform detection for the packaging tests.
#
# Sources this file expects:
#   $REPO_ROOT must be set by the caller (absolute path).
#
# Sets these variables:
#   HOST_OS         linux | darwin | windows
#   HOST_GOARCH     amd64 | arm64
#   HOST_NPM_KEY    linux-x64 | linux-arm64 | darwin-x64 | darwin-arm64 | win32-x64 | win32-arm64
#   HOST_PIP_TAG    manylinux2014_x86_64 | manylinux2014_aarch64 | macosx_11_0_x86_64 |
#                   macosx_11_0_arm64 | win_amd64 | win_arm64
#   HOST_BIN_NAME   sqlitedeploy or sqlitedeploy.exe
#   HOST_BIN_PATH   absolute path to the prebuilt binary in $REPO_ROOT/dist
#
# Exits non-zero if the host isn't one of the six supported platforms or
# if the binary hasn't been built yet.

set -euo pipefail

case "$(uname -s)" in
    Linux*)      HOST_OS=linux ;;
    Darwin*)     HOST_OS=darwin ;;
    MINGW*|MSYS*|CYGWIN*) HOST_OS=windows ;;
    *) echo "unsupported host OS: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
    x86_64|amd64) HOST_GOARCH=amd64 ;;
    arm64|aarch64) HOST_GOARCH=arm64 ;;
    *) echo "unsupported host arch: $(uname -m)" >&2; exit 1 ;;
esac

case "$HOST_OS-$HOST_GOARCH" in
    linux-amd64)   HOST_NPM_KEY=linux-x64;     HOST_PIP_TAG=manylinux2014_x86_64 ;;
    linux-arm64)   HOST_NPM_KEY=linux-arm64;   HOST_PIP_TAG=manylinux2014_aarch64 ;;
    darwin-amd64)  HOST_NPM_KEY=darwin-x64;    HOST_PIP_TAG=macosx_11_0_x86_64 ;;
    darwin-arm64)  HOST_NPM_KEY=darwin-arm64;  HOST_PIP_TAG=macosx_11_0_arm64 ;;
    windows-amd64) HOST_NPM_KEY=win32-x64;     HOST_PIP_TAG=win_amd64 ;;
    windows-arm64) HOST_NPM_KEY=win32-arm64;   HOST_PIP_TAG=win_arm64 ;;
esac

if [ "$HOST_OS" = "windows" ]; then
    HOST_BIN_NAME="sqlitedeploy.exe"
else
    HOST_BIN_NAME="sqlitedeploy"
fi

# Allow tests to point at a scratch binary location instead of the repo's
# main dist/. run-all.sh sets TEST_BIN_DIR to test/.scratch/dist so the
# user's dist/ never gets clobbered.
TEST_BIN_DIR="${TEST_BIN_DIR:-$REPO_ROOT/dist}"
HOST_BIN_PATH="$TEST_BIN_DIR/$HOST_BIN_NAME"

if [ ! -f "$HOST_BIN_PATH" ]; then
    cat >&2 <<EOF
binary not found: $HOST_BIN_PATH

Build it first:
    go build -ldflags="-X main.version=0.0.0-test" -o $HOST_BIN_PATH ./cmd/sqlitedeploy

Or just run \`bash test/run-all.sh\`, which builds it for you.
EOF
    exit 1
fi

export HOST_OS HOST_GOARCH HOST_NPM_KEY HOST_PIP_TAG HOST_BIN_NAME HOST_BIN_PATH
