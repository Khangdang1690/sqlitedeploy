"""Custom Hatch build hook for sqlitedeploy.

Copies the matching prebuilt Go binary into the wheel's package data and
forces the wheel's platform tag, so a single pyproject.toml can produce all
six platform wheels by varying environment variables.

Driven by:
    SQLITEDEPLOY_BINARY  absolute path to the Go binary to bundle
    SQLITEDEPLOY_PLAT    wheel platform tag (e.g. manylinux2014_x86_64,
                         macosx_11_0_arm64, win_amd64, win_arm64)
"""

from __future__ import annotations

import os
import shutil
from pathlib import Path

from hatchling.builders.hooks.plugin.interface import BuildHookInterface


class SqlitedeployBuildHook(BuildHookInterface):
    PLUGIN_NAME = "custom"

    def initialize(self, version: str, build_data: dict) -> None:
        binary = os.environ.get("SQLITEDEPLOY_BINARY")
        plat_tag = os.environ.get("SQLITEDEPLOY_PLAT")

        if not binary or not plat_tag:
            raise RuntimeError(
                "SQLITEDEPLOY_BINARY and SQLITEDEPLOY_PLAT must be set. "
                "This wheel cannot be built without selecting a target platform "
                "and supplying the matching Go binary."
            )

        src = Path(binary)
        if not src.is_file():
            raise FileNotFoundError(f"binary not found: {src}")

        dest_dir = Path(self.root) / "src" / "sqlitedeploy" / "_bin"
        dest_dir.mkdir(parents=True, exist_ok=True)

        # Clear any binary left over from a previous wheel build in the same
        # working tree — otherwise the wheel could end up with both the
        # POSIX and Windows binaries.
        for stale in dest_dir.iterdir():
            stale.unlink()

        is_windows = plat_tag.startswith("win")
        dest_name = "sqlitedeploy.exe" if is_windows else "sqlitedeploy"
        dest = dest_dir / dest_name
        shutil.copy2(src, dest)
        if not is_windows:
            dest.chmod(0o755)

        # Bundled native binary => wheel is platform-specific, not pure Python.
        # Python tag stays py3 (no extension modules), abi tag is none.
        build_data["pure_python"] = False
        build_data["infer_tag"] = False
        build_data["tag"] = f"py3-none-{plat_tag}"
