# sqlitedeploy

[![npm](https://img.shields.io/npm/v/sqlitedeploy.svg?label=npm)](https://www.npmjs.com/package/sqlitedeploy)
[![PyPI](https://img.shields.io/pypi/v/sqlitedeploy.svg?label=pypi)](https://pypi.org/project/sqlitedeploy/)
[![GitHub release](https://img.shields.io/github/v/release/Khangdang1690/sqlitedeploy?label=binary)](https://github.com/Khangdang1690/sqlitedeploy/releases)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

A free, distributed SQLite database in one terminal command. Your durable
master lives in your own object-storage bucket (Cloudflare R2 / Backblaze B2 /
any S3-compatible service); your working copy lives next to your application.
Any language with a SQLite driver connects to it natively — no SDK required.

## Architecture

```
        PRIMARY NODE                                READ REPLICA NODES
   ┌────────────────────┐                        ┌──────────────────────┐
   │ your app (any lang)│                        │ your app (read-only) │
   │       │            │                        │           │          │
   │       ▼            │                        │           ▼          │
   │   data/app.db ─────┼── litestream ──┐       │      data/app.db     │
   │   (WAL mode)       │   replicate    │       │     (refreshed       │
   └────────────────────┘                │       │      every N s)      │
                                         │       └────────▲─────────────┘
                                         ▼                │
                          ┌──────────────────────┐        │
                          │  Object storage      │────────┘
                          │  (R2 / B2 / S3)      │   litestream restore
                          │  ── your bucket      │
                          └──────────────────────┘
```

* Exactly one node ever writes (the primary).
* Writes are continuously WAL-replicated to your bucket by Litestream.
* Replica nodes periodically pull from the bucket → near-real-time read replicas.
* Disaster recovery: lose the primary, run `sqlitedeploy restore` on a fresh
  box, point traffic at it.

## Quick start

### 1. Install

Pick whichever fits your stack — every option resolves to the same prebuilt
binary (~40 MB, with the matching Litestream embedded). The npm and pip
packages are thin shims that locate the right binary for your OS/arch and
exec it; no postinstall scripts, no network calls beyond the registry.

**Node.js / TypeScript / Next.js**:

```bash
npm i -g sqlitedeploy
# or, project-local:
npm i sqlitedeploy && npx sqlitedeploy --help
```

**Python / FastAPI**:

```bash
pip install sqlitedeploy
sqlitedeploy --help
```

**Go** (1.22+):

```bash
go install github.com/Khangdang1690/sqlitedeploy/cmd/sqlitedeploy@latest
sqlitedeploy --help
```

The build is self-contained: the matching `litestream` is committed into the
module under `internal/litestream/bin/` and embedded at compile time, so a
`go install` produces the same ~40 MB binary the npm and pip packages ship.

**Java / Spring Boot** — via Maven Central:

```xml
<dependency>
  <groupId>io.github.khangdang1690</groupId>
  <artifactId>sqlitedeploy-cli</artifactId>
  <version>${sqlitedeploy.version}</version>
</dependency>
```

Maven resolves the right platform-classifier JAR (e.g. `linux-x86_64`) at
install time, mirroring npm's per-platform packages. Run via
`mvn exec:java -Dexec.mainClass=io.github.khangdang1690.sqlitedeploy.Main`,
or use the launcher class directly from your build. Some pipelines prefer
[`os-maven-plugin`](https://github.com/trustin/os-maven-plugin) to detect
the classifier automatically.

**Standalone binary** (other platforms, no package manager):
download from <https://github.com/Khangdang1690/sqlitedeploy/releases> and
put it on your `PATH`.

**From source** (requires Go 1.22+).

macOS / Linux (with `make`, `curl`, `tar`, `unzip`):

```bash
make fetch-litestream    # downloads litestream into internal/litestream/bin/
make build               # outputs dist/sqlitedeploy (~40 MB, self-contained)
```

Windows (no `make` required, uses built-in PowerShell + `tar.exe`):

```powershell
pwsh scripts\fetch-litestream.ps1
go build -o dist\sqlitedeploy.exe .\cmd\sqlitedeploy
```

If you skip the fetch step, the CLI falls back to looking for `litestream`
on your `PATH`. Install Litestream separately from
<https://litestream.io/install/> if you'd rather not bundle.

### 2. Sign in to Cloudflare (managed flow — recommended)

You need a free Cloudflare account. Recommended free tiers if you want to
mix providers later:

| Provider          | Free tier             | Egress         |
| ----------------- | --------------------- | -------------- |
| **Cloudflare R2** | 10 GB                 | Free           |
| Backblaze B2      | 10 GB                 | 1 GB/day free  |
| AWS S3            | 5 GB (12 months only) | Charged        |

**One-time R2 activation.** Cloudflare requires a one-time ToS click-through
on each account before any R2 API call works. Visit
<https://dash.cloudflare.com/?to=/:account/r2/overview> and click
`Purchase R2 Plan` — it's free; the 10 GB tier means no charges unless you
exceed it. If you skip this step, `sqlitedeploy init` prints an instructive
error pointing back here.

For Cloudflare R2 the CLI handles bucket creation and access-key generation
for you. Run once per machine:

```bash
sqlitedeploy auth login
```

This opens your browser to dash.cloudflare.com, walks you through creating
an API token with the right permissions (Workers R2 Storage Edit + API
Tokens Edit), and stores the validated token at `~/.config/sqlitedeploy/credentials.yml`
(mode 0600). From then on, every `sqlitedeploy init` reuses it — no more
account IDs or access keys to copy around.

`sqlitedeploy auth status` shows which account you're signed in as.
`sqlitedeploy auth logout` forgets the saved token.

### 3. Init the primary

**Managed flow** (after `auth login`):

```bash
sqlitedeploy init
```

The CLI lists your existing R2 buckets, lets you pick one or create a new one
(default name: `sqlitedeploy-<projectdir>`), then creates a bucket-scoped R2
access key automatically. You'll see something like:

```
Using Cloudflare account: My Account
Existing R2 buckets:
  1. another-app
  2. (create new)
Choice [2]: 2
New bucket name [sqlitedeploy-myapp]:
Creating bucket: sqlitedeploy-myapp
Creating scoped R2 access key (sqlitedeploy-sqlitedeploy-myapp-laptop)...
  access key id: 4f9c...

sqlitedeploy primary initialized
  Database file:     /home/me/myapp/data/app.db
  Connection (URI):  sqlite:////home/me/myapp/data/app.db
  Provider:          r2 (bucket=sqlitedeploy-myapp, path=db)
```

**Manual flow** (any provider, or for CI / non-interactive use):

```bash
sqlitedeploy init \
  --provider r2 \
  --bucket my-app-db \
  --account-id <your-cloudflare-account-id> \
  --access-key <key> --secret-key <secret>
```

Supplying `--access-key` / `--secret-key` / `--account-id` automatically
disables the managed flow, so you bypass Cloudflare's API entirely. This is
also the path for B2 and generic S3.

### 4. Start replication

```bash
sqlitedeploy run        # foreground; supervise with systemd / docker / etc.
```

### 5. Connect from your app

The connection string is just the local SQLite file path — every language
works natively:

```python
# Python (FastAPI, Django, anything)
import sqlite3
db = sqlite3.connect("data/app.db")
```

```js
// Node.js (Next.js, Express)
const Database = require('better-sqlite3');
const db = new Database('data/app.db');
```

```go
// Go
db, _ := sql.Open("sqlite3", "data/app.db")
```

```java
// Java / Spring Boot — needs `org.xerial:sqlite-jdbc` on the classpath.
// In application.yml:
//   spring.datasource.url: jdbc:sqlite:./data/app.db
//   spring.datasource.driver-class-name: org.sqlite.JDBC
// Or in plain JDBC:
Connection db = DriverManager.getConnection("jdbc:sqlite:./data/app.db");
```

### 6. Attach a read replica (on another machine)

```bash
sqlitedeploy attach \
  --provider r2 --bucket my-app-db \
  --account-id <id> --access-key <key> --secret-key <secret>
```

This pulls the latest snapshot, then stays near-real-time by re-restoring
every 5 seconds (configurable via `--pull-interval`). Your app reads from the
local file as usual.

## CLI reference

| Command                  | Purpose                                                                |
| ------------------------ | ---------------------------------------------------------------------- |
| `sqlitedeploy auth login`  | Sign in with a Cloudflare API token (stored at user-config dir)      |
| `sqlitedeploy auth status` | Show which Cloudflare account the saved token authenticates as       |
| `sqlitedeploy auth logout` | Forget the saved Cloudflare token                                    |
| `sqlitedeploy init`        | Set up a primary node (managed for R2; manual for B2/S3 or with flags) |
| `sqlitedeploy run`         | Run continuous WAL replication on the primary                        |
| `sqlitedeploy attach`      | Set up a read replica node                                           |
| `sqlitedeploy status`      | Show config, local DB size, replicated LTX files in your bucket      |
| `sqlitedeploy restore`     | Pull the latest replicated state from object storage                 |
| `sqlitedeploy destroy`     | Remove local sqlitedeploy state (does NOT touch bucket)              |

Run any command with `--help` for full flags.

## Honest limitations

* **Single-writer only.** Multiple nodes cannot all write to the same database.
  Litestream is one-way replication; if two clones tried to push back to the
  same bucket they'd corrupt the master. If you need multiple writers, use
  [LiteFS](https://fly.io/docs/litefs/), [rqlite](https://rqlite.io/), or
  [Turso](https://turso.tech/) instead.
* **Two processes — not serverless-friendly.** `sqlitedeploy run` is a
  long-lived sidecar that has to run alongside your application. This rules
  out fully-serverless platforms (Vercel, Cloudflare Workers, AWS Lambda,
  Edge runtimes) where you can't keep a daemon alive. You need a real
  long-running compute host: a VPS, container, EC2, Fly.io machine, Render
  worker, etc.
* **Async durability.** Writes are flushed to object storage on Litestream's
  schedule (default ~1 second). The last few seconds of writes can be lost on
  a primary crash before the WAL ships.
* **Replica lag.** Read replicas pull from object storage every 5 s by default.
  This is not "read-your-writes" consistent across nodes.
* **Free-tier ceilings.** R2/B2 free tiers cap at 10 GB. Watch `sqlitedeploy
  status` and your provider's dashboard.
* **No automatic failover in v1.** If the primary node dies, a human runs
  `sqlitedeploy restore` on a new box and re-points traffic.

## How it actually works

`sqlitedeploy init` does five things:

1. Creates `./data/app.db` and runs `PRAGMA journal_mode=WAL` (Litestream's
   one hard requirement).
2. Stores your provider credentials in `./.sqlitedeploy/config.yml` (mode 0600,
   added to `.gitignore` automatically).
3. Renders a `./.sqlitedeploy/litestream.yml` config pointing the local DB at
   your bucket.
4. Resolves a `litestream` binary — preferring the one bundled into
   `sqlitedeploy` at build time, falling back to `$PATH`.
5. Prints a connection string your app can use straight away.

`sqlitedeploy run` then just runs `litestream replicate -config ...` against
that config. All the heavy lifting is Litestream's.

`sqlitedeploy attach` does the inverse: writes a config with no `replicas:`
block (so the node can't accidentally write to the master), runs
`litestream restore` once for the initial snapshot, then re-runs `restore`
on a timer to stay current.

## Contributing

Bug reports, feature requests, and pull requests welcome at
<https://github.com/Khangdang1690/sqlitedeploy/issues>. The packaging integration
tests live in [`test/`](test/) — `bash test/run-all.sh` to run them locally.

## License

[Apache-2.0](LICENSE). Bundled
[Litestream](https://github.com/benbjohnson/litestream) is also Apache-2.0.
