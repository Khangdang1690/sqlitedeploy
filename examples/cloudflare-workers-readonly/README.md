# Cloudflare Workers — read-only client example

This Worker connects to a `sqlitedeploy` primary over **Hrana HTTP** using
[`@libsql/client`](https://www.npmjs.com/package/@libsql/client). It's the
proof point for `sqlitedeploy` v2's edge support — under v1 (Litestream),
Workers couldn't talk to the DB at all.

The Worker has two routes:

- `GET /health` → `ok`
- `GET /*` → JSON listing tables in the DB and the SQLite version

## Setup

You need a running `sqlitedeploy` primary on a host the Worker can reach over
the public internet (or via a tunnel like `cloudflared`).

### 1. Configure `wrangler.toml`

Edit `wrangler.toml` and set `PRIMARY_URL` to your primary's Hrana HTTP
endpoint. For production, terminate TLS in front of sqld with Caddy / nginx /
Cloudflare Tunnel and use `https://...:443`. For local development, expose
your primary over a tunnel:

```bash
# On the primary host:
sqlitedeploy run                        # binds 127.0.0.1:8080 by default
cloudflared tunnel --url http://localhost:8080
# → https://random-name.trycloudflare.com
```

Then set `PRIMARY_URL = "https://random-name.trycloudflare.com"` in
`wrangler.toml`.

### 2. Provide the replica JWT as a Wrangler secret

The replica token was minted by `sqlitedeploy init` and saved at
`.sqlitedeploy/auth/replica.jwt` on the primary host. Copy it into the
Worker's secret store — **don't** put it in `wrangler.toml`:

```bash
wrangler secret put SQLITEDEPLOY_REPLICA_JWT < /path/to/.sqlitedeploy/auth/replica.jwt
```

### 3. Install dependencies and deploy

```bash
npm install
npm run deploy
```

Wrangler prints the deployed Worker URL. Hit `/` to see tables; hit `/health`
for a liveness check.

## Development loop

```bash
npm run dev          # local dev with miniflare; hot-reload on src/ changes
```

Local dev still talks to a real `PRIMARY_URL` over HTTP — there's no
mock/in-memory DB. Point at a dev primary, or use a Cloudflare Tunnel to
expose `localhost:8080` from your dev machine.

## What this proves

| Property                                   | v1 (Litestream)                | v2 (sqld)                           |
| ------------------------------------------ | ------------------------------ | ----------------------------------- |
| Can a Cloudflare Worker query the DB?      | No — no edge-compatible client | **Yes** — Hrana over HTTP           |
| Replica freshness                          | 5 s polling                    | Sub-second gRPC stream              |
| Apps still work with stock SQLite drivers? | Yes                            | Yes (sqld manages the file in-place) |

## Troubleshooting

- **401 / token rejected**: the JWT's signing key must match the primary's
  `--auth-jwt-key-file`. If you re-ran `sqlitedeploy init`, keys rotated; mint
  a new replica token and re-set the secret.
- **Connection timeout / DNS failure**: `PRIMARY_URL` must be reachable from
  Cloudflare's edge. Localhost-only primaries need a tunnel.
- **Empty `tables` array**: the DB has no user tables yet. Either create one
  (`echo "CREATE TABLE users (id TEXT PRIMARY KEY);" | sqlite3 data/app.db`)
  or use this Worker against a primary that already has data.
