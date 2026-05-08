#!/usr/bin/env python3
"""Rewrite version fields in the pip packaging manifests.

    python3 scripts/stamp-versions.py 0.1.0

Affects:
    packaging/pip/pyproject.toml
    packaging/pip/src/sqlitedeploy/__init__.py
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

SEMVER_RE = re.compile(r"^\d+\.\d+\.\d+")


def main() -> None:
    if len(sys.argv) != 2 or not SEMVER_RE.match(sys.argv[1]):
        print("usage: stamp-versions.py <semver>", file=sys.stderr)
        sys.exit(1)
    version = sys.argv[1]
    repo_root = Path(__file__).resolve().parent.parent

    pyproject = repo_root / "packaging" / "pip" / "pyproject.toml"
    text = pyproject.read_text(encoding="utf-8")
    new_text, n = re.subn(
        r'(?m)^version = "[^"]+"', f'version = "{version}"', text, count=1
    )
    if n != 1:
        raise SystemExit(f"could not find version field in {pyproject}")
    pyproject.write_text(new_text, encoding="utf-8")
    print(f"stamped {pyproject.relative_to(repo_root)} -> {version}")

    init = repo_root / "packaging" / "pip" / "src" / "sqlitedeploy" / "__init__.py"
    text = init.read_text(encoding="utf-8")
    new_text, n = re.subn(
        r'(?m)^__version__ = "[^"]+"', f'__version__ = "{version}"', text, count=1
    )
    if n != 1:
        raise SystemExit(f"could not find __version__ in {init}")
    init.write_text(new_text, encoding="utf-8")
    print(f"stamped {init.relative_to(repo_root)} -> {version}")


if __name__ == "__main__":
    main()
