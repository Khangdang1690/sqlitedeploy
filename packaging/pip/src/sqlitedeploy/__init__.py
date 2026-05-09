"""sqlitedeploy: distributed SQLite with sqld + bottomless replication.

This package is a thin wrapper that ships the prebuilt Go binary for the
host platform. All real logic lives in the Go CLI; this Python package
exists only so users can `pip install sqlitedeploy` and get the binary
on their PATH.
"""

__all__ = ["__version__"]
__version__ = "0.0.0"
