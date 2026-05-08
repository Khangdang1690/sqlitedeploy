# sqlitedeploy

Distributed SQLite in one terminal command. Your master lives in your own
object-storage bucket (Cloudflare R2 / Backblaze B2 / S3); your working copy
lives next to your application. Any language with a SQLite driver connects
to it natively.

This package installs the prebuilt platform-native binary and exposes it as
the `sqlitedeploy` command.

## Install

```bash
npm i sqlitedeploy
# or
npm i -g sqlitedeploy
```

`npm` picks the matching binary from one of the platform packages
(`@weirdvl/linux-x64`, `@weirdvl/darwin-arm64`, …) automatically
via `optionalDependencies`. No `postinstall`, no network calls beyond the
registry.

If your install is configured with `--no-optional` or `--ignore-scripts`
the resolver will tell you what to do.

## Quick start

```bash
npx sqlitedeploy auth login    # one-time Cloudflare R2 setup
npx sqlitedeploy init          # creates ./data/app.db + ./.sqlitedeploy/
npx sqlitedeploy run           # foreground replication loop
```

Then connect from your app:

```js
const Database = require('better-sqlite3');
const db = new Database('./data/app.db');
```

## Supported platforms

linux-x64, linux-arm64, darwin-x64, darwin-arm64, win32-x64, win32-arm64.

For unsupported platforms (e.g. linux-riscv64, freebsd) download a binary
from <https://github.com/Khangdang1690/elite/releases>.

## Documentation

Full CLI reference, architecture, and limitations:
<https://github.com/Khangdang1690/elite>.

## License

Apache-2.0.
