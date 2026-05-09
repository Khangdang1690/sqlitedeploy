# sqlitedeploy

Distributed SQLite in one terminal command. Your master lives in your own
object-storage bucket (Cloudflare R2 / Backblaze B2 / S3); your working copy
lives next to your application. Any language with a SQLite driver connects
to it natively.

This package installs the prebuilt platform-native binary and exposes it as
the `sqlitedeploy` command.

## Install

```bash
pip install sqlitedeploy
```

PyPI serves a platform-tagged wheel with the matching binary baked in —
no compilation, no postinstall scripts.

## Quick start

```bash
sqlitedeploy auth login    # one-time Cloudflare R2 setup
sqlitedeploy up            # provisions storage + tunnel + starts sqld
```

Then connect from your app:

```python
import sqlite3
db = sqlite3.connect(".sqlitedeploy/db.sqlite")
```

You can also invoke as a module:

```bash
python -m sqlitedeploy --help
```

## Supported platforms

linux x86_64, linux aarch64, macOS x86_64, macOS arm64, Windows x86_64,
Windows arm64.

For unsupported platforms (e.g. linux-riscv64, FreeBSD) download a binary
from <https://github.com/Khangdang1690/sqlitedeploy/releases>.

## Documentation

Full CLI reference, architecture, and limitations:
<https://github.com/Khangdang1690/sqlitedeploy>.

## License

Apache-2.0.
