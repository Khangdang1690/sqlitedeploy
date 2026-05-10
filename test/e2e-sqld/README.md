# E2E test suite for sqlitedeploy v2 (sqld)

End-to-end smoke test that brings up a real MinIO bucket, runs the full
`init → run → write → read-via-Hrana` flow, and confirms data round-trips
through sqld + bottomless.

This is **separate** from the existing packaging tests in [`test/`](../) —
those validate npm/pip/maven shims; this validates the runtime.

## Prereqs

- Linux or macOS (sqld doesn't compile on Windows; use WSL2 there)
- `docker` + `docker compose`
- `curl`, `sqlite3`, `jq`
- A built `sqlitedeploy` binary (run `make build` from the repo root)

## Run

```bash
make build
bash test/e2e-sqld/run.sh
```

## What it tests

| Step                       | What's exercised                                           |
| -------------------------- | ---------------------------------------------------------- |
| `sqlitedeploy up --ingress=listen --byo-storage` | Provider config, JWT keypair gen, replica token mint, sqld primary + bottomless replication to MinIO |
| Stock sqlite3 write        | Local file write path (apps using existing SQLite drivers) |
| Hrana HTTP read with JWT   | Edge-client read path + JWT verification                   |

## Phase 6.5 (TODO)

A second host attaching as a replica:

1. Copy `.sqlitedeploy/auth/jwt_public.pem` + `replica.jwt` to a fresh tmpdir.
2. Run `sqlitedeploy attach --primary-grpc-url http://127.0.0.1:15001 ...`.
3. Read from the replica's local Hrana endpoint and verify the row written
   on the primary appears within ~1 second.

Not implemented yet because the simple smoke covers most of the recipe.
File an issue if you need it.
