#!/usr/bin/env bash
# Verify the pip wrapper installs and execs the bundled binary correctly.
#
# Self-contained: builds its own binary with a PEP 440-compliant test
# version, stamps the manifests, builds a single platform-tagged wheel via
# Hatch, installs it in a fresh venv, and confirms `sqlitedeploy --version`
# echoes the same string.
#
# Cleans up by restoring manifests on exit (even on failure).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
# PEP 440 dev-release format — Hatch validates with packaging.version which
# rejects the SemVer hyphen syntax we use for npm.
TEST_VERSION="0.0.0.dev0"
TEST_BIN_DIR="$REPO_ROOT/test/.scratch/pip-dist"

# Build a fresh binary stamped with this test's version (different from the
# npm test's because pip needs PEP 440 syntax).
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

SCRATCH="$REPO_ROOT/test/.scratch/pip"
TOOLS_VENV="$REPO_ROOT/.venv-tools"
PIP_DIR="$REPO_ROOT/packaging/pip"

PYTHON_VENV_BIN_DIR="bin"
HATCH_BIN="$TOOLS_VENV/bin/hatch"
if [ "$HOST_OS" = "windows" ]; then
    PYTHON_VENV_BIN_DIR="Scripts"
    HATCH_BIN="$TOOLS_VENV/Scripts/hatch"
fi

PY=python
command -v python3 >/dev/null 2>&1 && PY=python3

cleanup() {
    rc=$?
    echo "--- pip test cleanup"
    "$PY" "$REPO_ROOT/scripts/stamp-versions.py" 0.0.0 >/dev/null
    rm -rf "$SCRATCH" "$TEST_BIN_DIR" "$PIP_DIR/dist" "$PIP_DIR/src/sqlitedeploy/_bin"
    if [ "$rc" -eq 0 ]; then echo "PASS  test-pip"; else echo "FAIL  test-pip (rc=$rc)"; fi
    exit "$rc"
}
trap cleanup EXIT

echo "--- pip test (host=$HOST_PIP_TAG, version=$TEST_VERSION)"
echo "    binary: $HOST_BIN_PATH"

if [ ! -x "$HATCH_BIN" ]; then
    echo "hatch not found at $HATCH_BIN — run \`bash test/run-all.sh\` to bootstrap" >&2
    exit 1
fi

echo "[1/5] stamp test version into manifests"
"$PY" "$REPO_ROOT/scripts/stamp-versions.py" "$TEST_VERSION" >/dev/null

echo "[2/5] hatch build wheel for $HOST_PIP_TAG"
mkdir -p "$SCRATCH"
(cd "$PIP_DIR" && \
    SQLITEDEPLOY_BINARY="$HOST_BIN_PATH" \
    SQLITEDEPLOY_PLAT="$HOST_PIP_TAG" \
    "$HATCH_BIN" build --target wheel >/dev/null)
WHEEL="$(ls "$PIP_DIR/dist/"sqlitedeploy-*.whl | head -1)"
echo "    wheel: $(basename "$WHEEL")"

case "$(basename "$WHEEL")" in
    *"$HOST_PIP_TAG"*) ;;
    *)
        echo "    wheel name missing platform tag $HOST_PIP_TAG" >&2
        exit 1
        ;;
esac

echo "[3/5] install wheel in scratch venv"
"$PY" -m venv "$SCRATCH/venv"
"$SCRATCH/venv/$PYTHON_VENV_BIN_DIR/python" -m pip install --quiet --upgrade pip
"$SCRATCH/venv/$PYTHON_VENV_BIN_DIR/python" -m pip install --quiet "$WHEEL"

echo "[4/5] exec sqlitedeploy --version"
SD_BIN="$SCRATCH/venv/$PYTHON_VENV_BIN_DIR/sqlitedeploy"
[ "$HOST_OS" = "windows" ] && SD_BIN="$SD_BIN.exe"

ACTUAL="$("$SD_BIN" --version)"
EXPECTED="sqlitedeploy version $TEST_VERSION"
if [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "    expected: $EXPECTED"
    echo "    got:      $ACTUAL"
    exit 1
fi
echo "    output matches: $ACTUAL"

echo "[5/5] sanity check python -m sqlitedeploy --help"
"$SCRATCH/venv/$PYTHON_VENV_BIN_DIR/python" -m sqlitedeploy --help >/dev/null
echo "    python -m sqlitedeploy --help exits 0"
