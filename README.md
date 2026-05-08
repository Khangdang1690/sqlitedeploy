# sqlitedeploy

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

Pick whichever fits your stack — all three install the same prebuilt binary
(~40 MB, with the matching Litestream embedded). Each language's package
just locates the right binary for your OS/arch and execs it.

**Node.js / TypeScript / Next.js**:

```bash
npm i -g sqlitedeploy
# or, project-local:
npm i sqlitedeploy && npx sqlitedeploy --help
```

`npm` resolves the matching `@weirdvl/<platform>` package via
`optionalDependencies`. No postinstall scripts, no network calls beyond the
registry.

**Python / FastAPI**:

```bash
pip install sqlitedeploy
sqlitedeploy --help
```

PyPI serves a platform-tagged wheel with the binary baked in — no
compilation, no postinstall.

**Standalone binary** (any language, including Go / Java / Spring Boot):
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
# Python
import sqlite3
db = sqlite3.connect("data/app.db")
```

```js
// Node
const Database = require('better-sqlite3');
const db = new Database('data/app.db');
```

```go
// Go
db, _ := sql.Open("sqlite3", "data/app.db")
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
| `sqlitedeploy status`      | Show config, local DB size, available snapshots                      |
| `sqlitedeploy restore`     | Pull the latest snapshot from object storage                         |
| `sqlitedeploy destroy`     | Remove local sqlitedeploy state (does NOT touch bucket)              |

Run any command with `--help` for full flags.

## Honest limitations

* **Single-writer only.** Multiple nodes cannot all write to the same database.
  Litestream is one-way replication; if two clones tried to push back to the
  same bucket they'd corrupt the master. If you need multiple writers, use
  [LiteFS](https://fly.io/docs/litefs/), [rqlite](https://rqlite.io/), or
  [Turso](https://turso.tech/) instead.
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
