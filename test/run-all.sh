#!/usr/bin/env bash
# Single entry point for the packaging integration tests.
#
# Run from anywhere:
#     bash test/run-all.sh
#
# Each integration test is fully self-contained: it builds its own binary
# (with a test-version stamped via -ldflags), packs/builds the wrapper,
# installs it in scratch space, and confirms the CLI works end-to-end.
# This file's only jobs are to bootstrap the Hatch tools venv (one-time)
# and call them in sequence.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# Bootstrap the tools venv if missing. We use a project-local .venv-tools/
# rather than installing hatch globally — keeps the user's system Python
# untouched. uv is fast on Windows; fall back to plain venv + pip otherwise.
if [ ! -x ".venv-tools/bin/hatch" ] && [ ! -x ".venv-tools/Scripts/hatch.exe" ]; then
    echo "--- bootstrapping .venv-tools/ (one-time, isolated)"
    if command -v uv >/dev/null 2>&1; then
        uv venv .venv-tools --python 3.11
        UV_LINK_MODE=copy uv pip install --python .venv-tools hatch
    else
        PY=python; command -v python3 >/dev/null && PY=python3
        "$PY" -m venv .venv-tools
        if [ -x ".venv-tools/Scripts/python.exe" ]; then
            .venv-tools/Scripts/python -m pip install --quiet --upgrade pip
            .venv-tools/Scripts/python -m pip install --quiet hatch
        else
            .venv-tools/bin/python -m pip install --quiet --upgrade pip
            .venv-tools/bin/python -m pip install --quiet hatch
        fi
    fi
fi

bash test/integration/test-npm.sh
bash test/integration/test-pip.sh
bash test/integration/test-go.sh
bash test/integration/test-maven.sh

echo "--- all packaging tests passed"
