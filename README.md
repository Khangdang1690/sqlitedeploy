# sqlitedeploy

[![npm](https://img.shields.io/npm/v/sqlitedeploy.svg?label=npm)](https://www.npmjs.com/package/sqlitedeploy)
[![PyPI](https://img.shields.io/pypi/v/sqlitedeploy.svg?label=pypi)](https://pypi.org/project/sqlitedeploy/)
[![GitHub release](https://img.shields.io/github/v/release/Khangdang1690/sqlitedeploy?label=binary)](https://github.com/Khangdang1690/sqlitedeploy/releases)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

**Turso-style SQLite, but you bring your own cloud bucket.**

A free, distributed SQLite database in one terminal command:

- A **primary** node serves your app over **Hrana HTTP** (so Cloudflare Workers, Lambda, and Vercel can connect) and over a regular SQLite file (so any language with a SQLite driver works natively).
- **Read replicas** stream WAL frames live from the primary over gRPC — sub-second freshness, not the 5-second polling lag of WAL-shipping tools.
- **Continuous backup** to your own object-storage bucket (Cloudflare R2 / Backblaze B2 / any S3-compatible service). No vendor lock-in on storage.

Internally, `sqlitedeploy` bundles [`sqld`](https://github.com/tursodatabase/libsql/tree/main/libsql-server) (the open-source MIT-licensed core of Turso's runtime) and wires it up to your bucket via libSQL's `bottomless` replication. v1 of `sqlitedeploy` shipped Litestream; v2 is a hard cutover to sqld for edge/serverless support.

## Architecture

```
        PRIMARY HOST                                       READ REPLICA HOSTS
   ┌────────────────────────┐                          ┌──────────────────────┐
   │ your app (any lang)    │                          │ your app (read-only) │
   │  ⇄ data/app.db (file)  │                          │  ⇄ data/app.db       │
   │  ⇄ Hrana :8080 (HTTP)  │                          │  ⇄ Hrana :8080       │
   │            ▲           │                          │           ▲          │
   │            │           │                          │           │          │
   │           sqld ────gRPC :5001 (live WAL stream)──▶│         sqld         │
   │            │           │                          │     (--primary-      │
   │            ▼           │                          │      grpc-url)       │
   │       bottomless       │                          └──────────────────────┘
   │            │           │
   └────────────┼───────────┘                          ┌──────────────────────┐
                │                                      │ Cloudflare Worker /  │
                │                                      │ Lambda / Vercel      │
                │                                      │   ⇄ Hrana over HTTP  │
                │                                      │   to primary :8080   │
                ▼                                      └──────────────────────┘
   ┌──────────────────────┐
   │  Object storage      │     ◄── disaster recovery: any host with bucket
   │  (R2 / B2 / S3)      │         creds can `--sync-from-storage` to seed
   │  ── your bucket      │         a fresh DB
   └──────────────────────┘
```

* Exactly one node ever writes (the primary).
* Writes go to the local SQLite file (via stock SQLite drivers) **or** through Hrana to sqld — sqld keeps both views in sync.
* Read replicas stream WAL frames live from primary's gRPC; on first attach, they cold-start from the bucket (faster than replaying everything).
* Edge clients (Workers, Lambda, Vercel) connect directly to primary's Hrana endpoint over HTTP — no sidecar required on the edge.
* The bucket is your durable backup. Lose the primary, point a fresh sqld at the bucket with `--sync-from-storage` to recover.

## Quick start

### 1. Install

Pick whichever fits your stack — every option resolves to the same prebuilt binary (~50 MB, with the matching `sqld` embedded). The npm and pip packages are thin shims that locate the right binary for your OS/arch and exec it.

> **Windows:** sqld doesn't compile on Windows ([upstream `libsql-wal` uses POSIX-only syscalls](https://github.com/tursodatabase/libsql/tree/main/libsql-wal)). Run sqlitedeploy under **WSL2**.

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

The build is self-contained: the matching `sqld` is committed into the module under `internal/sqld/bin/` and embedded at compile time, so a `go install` produces the same binary the npm and pip packages ship.

**Java / Spring Boot** — via Maven Central:

```xml
<dependency>
  <groupId>io.github.khangdang1690</groupId>
  <artifactId>sqlitedeploy-cli</artifactId>
  <version>${sqlitedeploy.version}</version>
</dependency>
```

Maven resolves the right platform-classifier JAR (e.g. `linux-x86_64`) at install time.

**Standalone binary** (no package manager): download from <https://github.com/Khangdang1690/sqlitedeploy/releases>.

**From source** (requires Go 1.22+ and Rust 1.80+; macOS or Linux only):

```bash
make fetch-libsql-source   # shallow-clone tursodatabase/libsql @ pinned tag
make build-sqld            # cargo build sqld for the host platform
make build                 # outputs dist/sqlitedeploy with sqld embedded
```

If you skip `build-sqld`, the CLI falls back to looking for `sqld` on `$PATH`.

### 2. Sign in to Cloudflare (managed flow — recommended)

You need a free Cloudflare account. Recommended free tiers if you want to mix providers later:

| Provider          | Free tier             | Egress         |
| ----------------- | --------------------- | -------------- |
| **Cloudflare R2** | 10 GB                 | Free           |
| Backblaze B2      | 10 GB                 | 1 GB/day free  |
| AWS S3            | 5 GB (12 months only) | Charged        |

**One-time R2 activation.** Cloudflare requires a one-time ToS click-through on each account before any R2 API call works. Visit <https://dash.cloudflare.com/?to=/:account/r2/overview> and click `Purchase R2 Plan` — it's free.

Then, once per machine:

```bash
sqlitedeploy auth login
```

This walks you through creating an API token and stores it at `~/.config/sqlitedeploy/credentials.yml` (mode 0600).

### 3. Init the primary

```bash
sqlitedeploy init
```

The CLI:

1. Creates `./data/app.db` in WAL mode.
2. Picks/creates a bucket and a scoped R2 access key (managed flow) or uses your `--access-key` / `--secret-key` (manual flow).
3. Writes `./.sqlitedeploy/config.yml` with bucket + endpoint config.
4. Generates an Ed25519 JWT keypair under `./.sqlitedeploy/auth/` and mints a long-lived replica token.
5. Prints connection details.

Sample output:

```
sqlitedeploy primary initialized

  Database file:     /home/me/myapp/data/app.db
  Provider:          r2 (bucket=sqlitedeploy-myapp, prefix=db)

  Endpoints (after `sqlitedeploy run`):
    Hrana over HTTP:   http://127.0.0.1:8080   (apps + edge clients connect here)
    gRPC for replicas: 0.0.0.0:5001            (replica nodes attach here)
    Local file:        sqlite:///home/me/myapp/data/app.db

  JWT auth (Ed25519):
    Public key:        .../.sqlitedeploy/auth/jwt_public.pem
    Private key:       .../.sqlitedeploy/auth/jwt_private.pem
    Replica token:     .../.sqlitedeploy/auth/replica.jwt
```

For production exposure on the internet (so Workers can reach you), pass `--http-listen-addr 0.0.0.0:8080` to bind on all interfaces. Put a TLS terminator (Caddy / nginx / Cloudflare Tunnel) in front for HTTPS — sqld speaks plain HTTP itself.

### 4. Start sqld

```bash
sqlitedeploy run        # foreground; supervise with systemd / docker / etc.
```

This runs `sqld` with bottomless replication enabled. Apps and edge clients can now connect.

### 5. Connect from your app

**From a regular app** (local SQLite file works exactly like before):

```python
# Python (FastAPI, Django, anything)
import sqlite3
db = sqlite3.connect("data/app.db")
```

```js
// Node.js
const Database = require('better-sqlite3');
const db = new Database('data/app.db');
```

**From a Cloudflare Worker / Lambda / Vercel** (edge — connect to sqld over HTTP):

```ts
// Worker / Lambda
import { createClient } from "@libsql/client";

const db = createClient({
  url: "http://your-primary-host:8080",
  authToken: env.SQLITEDEPLOY_REPLICA_JWT,  // the replica.jwt minted at init
});

const rows = await db.execute("SELECT * FROM users WHERE id = ?", [id]);
```

See [`examples/cloudflare-workers-readonly/`](examples/cloudflare-workers-readonly/) for a working Worker.

### 6. Attach a read replica (on another machine)

Copy the JWT public key and replica token from the primary to the replica host (over scp — never paste secrets into chat):

```bash
scp primary:.sqlitedeploy/auth/jwt_public.pem .sqlitedeploy/auth/
scp primary:.sqlitedeploy/auth/replica.jwt   .sqlitedeploy/auth/
```

Then on the replica:

```bash
sqlitedeploy attach \
  --provider r2 --bucket my-app-db \
  --account-id <id> --access-key <key> --secret-key <secret> \
  --primary-grpc-url http://primary.example.com:5001
```

First attach cold-starts from the bucket via bottomless, then keeps streaming WAL frames live from the primary's gRPC. Sub-second freshness.

## CLI reference

| Command                      | Purpose                                                                              |
| ---------------------------- | ------------------------------------------------------------------------------------ |
| `sqlitedeploy auth login`    | Sign in with a Cloudflare API token (stored at user-config dir)                      |
| `sqlitedeploy auth status`   | Show which Cloudflare account the saved token authenticates as                       |
| `sqlitedeploy auth logout`   | Forget the saved Cloudflare token                                                    |
| `sqlitedeploy init`          | Set up a primary node (DB, JWT keypair, provider config)                             |
| `sqlitedeploy run`           | Run sqld in primary mode with bottomless replication                                 |
| `sqlitedeploy attach`        | Set up a read replica, streaming from the primary's gRPC                             |
| `sqlitedeploy status`        | Show configured paths and endpoints                                                  |
| `sqlitedeploy restore`       | (v2 stub) — for replica cold-start, use `attach`. See `--help` for the migration plan. |
| `sqlitedeploy destroy`       | Remove local sqlitedeploy state (does NOT touch bucket)                              |

Run any command with `--help` for full flags.

## Honest limitations

* **Single-writer only.** Sqld doesn't fix this — SQLite is single-writer at the file level. If you need multiple writers, use [LiteFS](https://fly.io/docs/litefs/), [rqlite](https://rqlite.io/), or [Turso's managed product](https://turso.tech/).
* **Primary must be network-reachable.** Edge clients (Workers, Lambda) connect over HTTP, so the primary needs a public endpoint. v1's "Litestream sidecar on a private VPS" model doesn't apply anymore.
* **JWT keys to manage.** v2 uses Ed25519 JWTs for auth. Lose the private key on the primary and you can't mint new replica tokens. The keypair lives at `.sqlitedeploy/auth/` — back it up or commit to a sealed secrets store.
* **Async durability.** Writes are flushed to object storage by bottomless on a periodic schedule. The last few seconds of writes can be lost on a primary crash before the backup ships.
* **Free-tier ceilings.** R2/B2 free tiers cap at 10 GB. Watch your provider's dashboard.
* **No automatic failover in v2.** If the primary dies, a human spins up a fresh primary, points it at the bucket with `--sync-from-storage`, and re-points traffic.
* **No Windows.** Upstream `libsql-server` doesn't compile on Windows (POSIX-only syscalls in `libsql-wal`). Run sqlitedeploy under WSL2.
* **Upstream is in maintenance mode.** Turso has redirected new feature work to a separate "Turso Database" (concurrent-write MVCC rewrite). `libsql-server` is supported but not actively evolving — we'll evaluate a future migration in 6-12 months.

## How it actually works

`sqlitedeploy init` does five things:

1. Creates `./data/app.db` and runs `PRAGMA journal_mode=WAL` (sqld requires WAL).
2. Stores your provider credentials in `./.sqlitedeploy/config.yml` (mode 0600, gitignored).
3. Generates an Ed25519 JWT keypair under `./.sqlitedeploy/auth/`.
4. Mints a long-lived replica JWT (10y) and writes it to `./.sqlitedeploy/auth/replica.jwt`.
5. Resolves a `sqld` binary — preferring the one embedded into `sqlitedeploy` at build time, falling back to `$PATH`.

`sqlitedeploy run` then runs:

```
sqld --db-path data/app.db \
     --http-listen-addr 127.0.0.1:8080 \
     --grpc-listen-addr 0.0.0.0:5001 \
     --auth-jwt-key-file .sqlitedeploy/auth/jwt_public.pem \
     --enable-bottomless-replication
```

with `LIBSQL_BOTTOMLESS_*` env vars set from your provider config. Sqld speaks Hrana to apps, gRPC to replicas, and replicates WAL frames to your bucket via bottomless. All the runtime heavy lifting is sqld's; we're a packaging + bootstrap layer.

`sqlitedeploy attach` does the inverse: writes a replica config, then runs sqld with `--primary-grpc-url` pointing at the primary. On first attach, `--sync-from-storage` seeds the local DB from the bucket; subsequent runs catch up over gRPC only.

## Migration from v1 (Litestream-based)

v2 is a hard cutover. The bucket layout is incompatible — v1 wrote LTX files; v2 writes bottomless format. There are no v1 production users to migrate, so the move is just:

1. Upgrade `sqlitedeploy` to v2.
2. `sqlitedeploy destroy` (drops local config; doesn't touch the bucket).
3. Use a fresh bucket prefix (or a fresh bucket).
4. Re-run `sqlitedeploy init`.

If you do have production v1 data and want a migration path, file an issue.

## Contributing

Bug reports, feature requests, and pull requests welcome at <https://github.com/Khangdang1690/sqlitedeploy/issues>. The packaging integration tests live in [`test/`](test/) — `bash test/run-all.sh` to run them locally.

## License

[Apache-2.0](LICENSE). Bundled [`sqld` from libsql](https://github.com/tursodatabase/libsql) is MIT-licensed; the bundled binary keeps that license.
