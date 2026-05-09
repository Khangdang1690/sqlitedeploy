"""Locate the bundled sqlitedeploy binary and exec it with the user's args.

The Go CLI reads <cwd>/.sqlitedeploy/config.yml and <cwd>/data/app.db, so we
must not chdir. On POSIX we use os.execvp to replace the Python process so
signals (Ctrl-C while `sqlitedeploy run` is serving) reach sqld directly.
Windows is unsupported (sqld doesn't compile there) — use WSL2.
"""

from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path


def _binary_path() -> Path:
    bin_dir = Path(__file__).resolve().parent / "_bin"
    name = "sqlitedeploy.exe" if os.name == "nt" else "sqlitedeploy"
    return bin_dir / name


def main() -> None:
    binary = _binary_path()
    if not binary.is_file():
        sys.stderr.write(
            f"sqlitedeploy: binary not found at {binary}.\n"
            "This wheel was built without a bundled binary, which is a "
            "packaging bug. Reinstall, or download a binary from "
            "https://github.com/Khangdang1690/sqlitedeploy/releases\n"
        )
        sys.exit(1)

    args = [str(binary), *sys.argv[1:]]

    if os.name == "nt":
        # No execvp on Windows — spawn and forward the exit code.
        completed = subprocess.run(args, check=False)
        sys.exit(completed.returncode)
    else:
        # Replace this process so signals propagate cleanly.
        os.execvp(str(binary), args)
