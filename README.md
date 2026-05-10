# sqlitedeploy

[![npm](https://img.shields.io/npm/v/sqlitedeploy.svg?label=npm)](https://www.npmjs.com/package/sqlitedeploy)
[![PyPI](https://img.shields.io/pypi/v/sqlitedeploy.svg?label=pypi)](https://pypi.org/project/sqlitedeploy/)
[![GitHub release](https://img.shields.io/github/v/release/Khangdang1690/sqlitedeploy?label=binary)](https://github.com/Khangdang1690/sqlitedeploy/releases)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

**Turso-style SQLite, but you bring your own cloud bucket.**

A **free**, distributed SQLite database in one terminal command. Every link in
the default path is $0:

- ✅ **Cloudflare R2** free tier (10 GB, $0 egress) — the durable backup
- ✅ **TryCloudflare quick tunnel** — public HTTPS URL, no domain, no account
- ✅ Bundled `sqld` (libsql-server, MIT) and `cloudflared` (free OSS)
- ✅ No SaaS subscription of ours

What you get:

- A **primary** node serves your app over **Hrana HTTP** (so Cloudflare Workers, Lambda, and Vercel can connect) and over a regular SQLite file (so any language with a SQLite driver works natively).
- **Read replicas** stream WAL frames live from the primary over gRPC — sub-second freshness, not the 5-second polling lag of WAL-shipping tools.
- **Continuous backup** to your own object-storage bucket (Cloudflare R2 / Backblaze B2 / any S3-compatible service). No vendor lock-in on storage.

Internally, `sqlitedeploy` bundles [`sqld`](https://github.com/tursodatabase/libsql/tree/main/libsql-server) (the open-source MIT-licensed core of Turso's runtime) and wires it up to your bucket via libSQL's `bottomless` replication. v1 of `sqlitedeploy` shipped Litestream; v2 is a hard cutover to sqld for edge/serverless support.

## Architecture

```
        PRIMARY HOST                                       READ REPLICA HOSTS
   ┌────────────────────────┐                          ┌──────────────────────┐
   │ your app (any lang)    │                          │ your app (read-only) │
   │  ⇄ Hrana :8080 (HTTP)  │                          │  ⇄ Hrana :8080       │
   │     via @libsql/client │                          │     via @libsql/client │
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
* Apps connect to sqld over **Hrana HTTP** with `@libsql/client` (or any libsql-compatible driver). sqld manages the on-disk SQLite + WAL inside `.sqlitedeploy/db/` — don't poke at those files directly while sqld is running.
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

`go install` produces a CLI that, on first run, downloads the matching real `sqld` (~30 MB, cached under the user cache dir at `sqlitedeploy/`) from this repo's GitHub Releases. The npm, pip, and Maven packages, by contrast, ship `sqld` already embedded — no first-run download.

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

If you skip `build-sqld`, the CLI falls back at runtime to `sqld` on `$PATH`. (The download-from-Releases fallback is enabled only in tagged release builds; a local `make build` still surfaces the loud "no sqld available" error so you notice the missing dependency.)

### 2. Try it locally (60 seconds, no signup, $0)

```bash
sqlitedeploy dev
```

Spins up sqld with a database directory at `.sqlitedeploy/db/` — no cloud, no auth, no account needed. Apps connect at `libsql://127.0.0.1:8080`. Persists between runs; pass `--reset` to wipe.

### 3. Sign in to Cloudflare (one-time, free)

You need a free Cloudflare account for the R2 bucket (10 GB free, $0 egress).

**One-time R2 activation.** Cloudflare requires a one-time ToS click-through on each account before any R2 API call works. Visit <https://dash.cloudflare.com/?to=/:account/r2/overview> and click `Purchase R2 Plan` — it's free.

Then, once per machine:

```bash
sqlitedeploy auth login
```

This walks you through creating an API token and stores it at `~/.config/sqlitedeploy/credentials.yml` (mode 0600).

### 4. Bring it live — `sqlitedeploy up`

```bash
sqlitedeploy up
```

One command:

1. Provisions an R2 bucket (10 GB free) and a scoped access key
2. Generates an Ed25519 JWT keypair for client + replica auth
3. Starts the bundled sqld with bottomless replication
4. Opens a Cloudflare Tunnel so apps reach sqld over HTTPS — **no domain, no port-forward, no TLS terminator, $0**

Sample output:

```
[1/5] ✓ Cloudflare auth      (cached at ~/.config/sqlitedeploy/credentials.yml)
[2/5] ✓ R2 bucket            sqlitedeploy-myapp (created, free 10 GB tier)
[3/5] ✓ R2 access key        scoped to bucket
[4/5] ✓ sqld primary         http://127.0.0.1:8080
[5/5] ✓ Cloudflare Tunnel    https://big-river-1234.trycloudflare.com  (free, ephemeral)

✓ Your SQLite is live!

  Public URL:  libsql://big-river-1234.trycloudflare.com
  Auth token:  eyJhbGciOi…
  Local DB:    /home/me/myapp/.sqlitedeploy/db/  (sqld-managed)
  Provider:    r2 (bucket=sqlitedeploy-myapp, prefix=db)

Ctrl-C to stop · re-run `sqlitedeploy up` to resume · `sqlitedeploy down` to tear down
```

The first run downloads `cloudflared` (~30 MB, cached); subsequent runs skip that.

> **Stable hostnames.** Quick tunnels are ephemeral (the `*.trycloudflare.com` URL changes between runs). For production with a custom domain, the simplest path is `--ingress=listen --public-url=https://db.example.com` and let your platform's HTTPS (Fly auto-TLS, Render, your own Caddy, etc.) handle it — see the "Pick your cloud" section above. For "bind localhost-only behind your own reverse proxy" behavior, pass `--ingress=listen --http-listen-addr=127.0.0.1:8080`.

### 5. Connect from your app

Connect to sqld over Hrana HTTP using any libsql-compatible client. **Don't open the on-disk SQLite file directly while sqld is running** — sqld owns the WAL and concurrent stock-driver access can corrupt it.

**Node.js / TypeScript / Workers / Lambda / Vercel** — same client everywhere:

```ts
import { createClient } from "@libsql/client";

const db = createClient({
  url: env.SQLITEDEPLOY_URL,                 // libsql://big-river-1234.trycloudflare.com
  authToken: env.SQLITEDEPLOY_REPLICA_JWT,   // the replica.jwt minted at up
});

const rows = await db.execute("SELECT * FROM users WHERE id = ?", [id]);
```

**Python**: use [`libsql-client`](https://pypi.org/project/libsql-client/) (the same Hrana protocol):

```python
import libsql_client
client = libsql_client.create_client_sync(
    url=os.environ["SQLITEDEPLOY_URL"],
    auth_token=os.environ["SQLITEDEPLOY_REPLICA_JWT"],
)
client.execute("SELECT 1")
```

See [`examples/cloudflare-workers-readonly/`](examples/cloudflare-workers-readonly/) for a working Worker.

> **Inspecting locally.** sqld's database file lives at `.sqlitedeploy/db/dbs/default/data` (with `-wal`/`-shm` siblings). To poke at it with the `sqlite3` CLI, **first stop sqld**, then `sqlite3 .sqlitedeploy/db/dbs/default/data`.

### 6. Tear it down

```bash
sqlitedeploy down          # remove local .sqlitedeploy/ (config + DB + JWT keys)
sqlitedeploy down --wipe   # also delete the R2 bucket
```

### 7. Attach a read replica (on another machine)

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

| Command                      | Purpose                                                                       |
| ---------------------------- | ----------------------------------------------------------------------------- |
| `sqlitedeploy dev`           | Run sqld locally with no cloud, no auth, no signup — instant SQLite-as-a-service |
| `sqlitedeploy up`            | Provision storage + start sqld + open a public Cloudflare Tunnel (the headline command) |
| `sqlitedeploy down`          | Remove local state; `--wipe` also deletes the R2 bucket                       |
| `sqlitedeploy auth login`    | Sign in with a Cloudflare API token (stored at user-config dir)               |
| `sqlitedeploy auth status`   | Show which Cloudflare account the saved token authenticates as               |
| `sqlitedeploy auth logout`   | Forget the saved Cloudflare token                                             |
| `sqlitedeploy attach`        | Set up a read replica, streaming from the primary's gRPC                      |
| `sqlitedeploy status`        | Show configured paths and endpoints                                           |

Run any command with `--help` for full flags.

## Pick your cloud

`sqlitedeploy up` separates two decisions: where your **bucket** lives (durable backup) and how clients reach the **primary** (public URL). Mix and match — they're independent.

### Storage: bring your own bucket on any cloud

The default zero-config path uses Cloudflare R2 (10 GB free, $0 egress). For everything else, pass `--byo-storage --provider=<kind>` plus an access key. The same `s3` driver works with **any S3-compatible service**: AWS S3, Backblaze B2, Tigris, Wasabi, MinIO, DigitalOcean Spaces, Linode Object Storage, Scaleway, your self-hosted MinIO, even R2 with a hand-rolled token.

| Provider                       | Flag                                               |
| ------------------------------ | -------------------------------------------------- |
| Cloudflare R2 (managed, default) | (no flags — uses `auth login` creds)             |
| Cloudflare R2 (BYO token)      | `--provider=r2 --byo-storage --account-id=…`       |
| Backblaze B2                   | `--provider=b2 --region=us-west-002`                |
| AWS S3 / DO Spaces / MinIO / … | `--provider=s3 --endpoint=… --region=…`             |

`--access-key` / `--secret-key` (or env `SQLITEDEPLOY_ACCESS_KEY` / `SQLITEDEPLOY_SECRET_KEY`) work for all four.

### Ingress: tunnel, or your platform's own HTTPS

| Mode                             | When to use                                                                          |
| -------------------------------- | ------------------------------------------------------------------------------------ |
| `--ingress=tunnel` (default)     | Anywhere with outbound HTTPS — laptop, home server, container with no public IP. Free `*.trycloudflare.com` URL. |
| `--ingress=listen`               | Production on Fly / Render / Railway / Cloud Run / ECS / Kubernetes / your own VPS — your platform already terminates TLS. |

In `listen` mode, sqld auto-binds `0.0.0.0:8080` (override with `--http-listen-addr`). Pass `--public-url=https://db.example.com` so the success banner echoes the URL clients will actually use.

### Deploy recipes

**Fly.io** (uses Fly's auto-TLS + persistent volume):

```bash
sqlitedeploy up \
  --ingress=listen \
  --public-url=https://$FLY_APP_NAME.fly.dev \
  --byo-storage --provider=s3 \
  --endpoint=https://s3.us-east-1.amazonaws.com --region=us-east-1 \
  --bucket=$BUCKET --access-key=$AWS_ACCESS_KEY_ID --secret-key=$AWS_SECRET_ACCESS_KEY
```

Make sure your `fly.toml` mounts a volume at `.sqlitedeploy/` and exposes port 8080.

**Render / Railway / Koyeb** (auto-TLS on the platform's `*.onrender.com` / `*.railway.app` domain):

```bash
sqlitedeploy up \
  --ingress=listen --public-url=https://$RENDER_EXTERNAL_HOSTNAME \
  --byo-storage --provider=s3 --endpoint=… --bucket=… --access-key=… --secret-key=…
```

Set the platform's persistent disk mount point to your project's `.sqlitedeploy/` directory.

**Cloud Run / ECS / GKE / your own VPS** (you bring TLS):

```bash
sqlitedeploy up --ingress=listen --byo-storage --provider=s3 --bucket=… …
```

Without `--public-url` the banner just says "Listening on 0.0.0.0:8080 — point your platform's ingress here." Wire your load balancer / Caddy / nginx / Cloudflare Tunnel front-door at port 8080 and you're done.

> **Replicas need TCP for gRPC.** If you're attaching read replicas, the primary's port `5001` (gRPC) needs to be reachable too. On Fly that's a second `[[services]]` block in `fly.toml`; on AWS that's an NLB; everywhere else it's whatever your platform calls "raw TCP ingress."

## Honest limitations

* **Single-writer only.** Sqld doesn't fix this — SQLite is single-writer at the file level. If you need multiple writers, use [LiteFS](https://fly.io/docs/litefs/), [rqlite](https://rqlite.io/), or [Turso's managed product](https://turso.tech/).
* **Primary must be network-reachable.** Edge clients (Workers, Lambda) connect over HTTP, so the primary needs a public endpoint. The default `up` flow gets you that for free via Cloudflare Tunnel; for production on Fly/Render/Cloud Run/your VPS use `--ingress=listen --public-url=…` so your platform's existing HTTPS handles it.
* **JWT keys to manage.** v2 uses Ed25519 JWTs for auth. Lose the private key on the primary and you can't mint new replica tokens. The keypair lives at `.sqlitedeploy/auth/` — back it up or commit to a sealed secrets store.
* **Async durability.** Writes are flushed to object storage by bottomless on a periodic schedule. The last few seconds of writes can be lost on a primary crash before the backup ships.
* **Free-tier ceilings.** R2/B2 free tiers cap at 10 GB. Watch your provider's dashboard.
* **No automatic failover in v2.** If the primary dies, a human spins up a fresh primary, points it at the bucket with `--sync-from-storage`, and re-points traffic.
* **No Windows.** Upstream `libsql-server` doesn't compile on Windows (POSIX-only syscalls in `libsql-wal`). Run sqlitedeploy under WSL2.
* **Upstream is in maintenance mode.** Turso has redirected new feature work to a separate "Turso Database" (concurrent-write MVCC rewrite). `libsql-server` is supported but not actively evolving — we'll evaluate a future migration in 6-12 months.

## How it actually works

`sqlitedeploy up` does these things on first run:

1. Provisions an R2 bucket + scoped access key (managed flow), or accepts your existing creds (manual flow).
2. Stores provider config + JWT keypair + replica token under `./.sqlitedeploy/`.
3. Creates `./.sqlitedeploy/db/` as the database directory sqld will own (sqld 0.24+ treats `--db-path` as a directory and stores the actual SQLite at `dbs/default/data` inside).
4. Resolves the bundled `sqld` binary (or falls back to `$PATH`).
5. Starts sqld with bottomless replication:

   ```
   sqld --db-path .sqlitedeploy/db \
        --http-listen-addr 127.0.0.1:8080 \
        --grpc-listen-addr 0.0.0.0:5001 \
        --auth-jwt-key-file .sqlitedeploy/auth/jwt_public.pem \
        --enable-bottomless-replication
   ```

   with `LIBSQL_BOTTOMLESS_*` env vars set from the provider config.

6. Resolves `cloudflared` (cached download or `$PATH`) and runs `cloudflared tunnel --url http://127.0.0.1:8080` to obtain a public `https://*.trycloudflare.com` URL — no domain or account needed for the tunnel itself.

Subsequent runs skip steps 1–3 and just resume the stack.

Sqld speaks Hrana to apps, gRPC to replicas, and replicates WAL frames to your bucket via bottomless. All the runtime heavy lifting is sqld's and cloudflared's; we're a packaging + bootstrap layer.

`sqlitedeploy attach` is the replica counterpart: writes a replica config, then runs sqld with `--primary-grpc-url` pointing at the primary. On first attach, `--sync-from-storage` seeds the local DB from the bucket; subsequent runs catch up over gRPC only.

## Migration from v1 (Litestream-based)

v2 is a hard cutover. The bucket layout is incompatible — v1 wrote LTX files; v2 writes bottomless format. There are no v1 production users to migrate, so the move is just:

1. Upgrade `sqlitedeploy` to v2.
2. `sqlitedeploy down` (drops local config; doesn't touch the bucket).
3. Use a fresh bucket prefix (or a fresh bucket).
4. Re-run `sqlitedeploy up`.

If you do have production v1 data and want a migration path, file an issue.

## Contributing

Bug reports, feature requests, and pull requests welcome at <https://github.com/Khangdang1690/sqlitedeploy/issues>. The packaging integration tests live in [`test/`](test/) — `bash test/run-all.sh` to run them locally.

## License

[Apache-2.0](LICENSE). Bundled [`sqld` from libsql](https://github.com/tursodatabase/libsql) is MIT-licensed; the bundled binary keeps that license.
