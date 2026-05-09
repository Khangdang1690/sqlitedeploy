#!/usr/bin/env bash
# E2E smoke test for sqlitedeploy v2 (sqld + bottomless to MinIO).
#
# Validates:
#   1. `sqlitedeploy up --byo-storage --no-tunnel` bootstraps + runs against
#      an S3-compatible bucket without touching Cloudflare.
#   2. Writes via stock sqlite3 driver land in the local DB.
#   3. Hrana HTTP endpoint serves the same data to remote clients.
#   4. (TODO Phase 6.5) replica node streams from primary and sees the data.
#
# Prereqs: docker, docker compose, curl, sqlite3, jq, the sqlitedeploy binary
# at $SQLITEDEPLOY (defaults to ./dist/sqlitedeploy from `make build`).
#
# Usage:
#   bash test/e2e-sqld/run.sh
#
# Linux/macOS only — sqld doesn't compile on Windows. WSL works.

set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"

SQLITEDEPLOY="${SQLITEDEPLOY:-$REPO/dist/sqlitedeploy}"
WORK="$(mktemp -d -t sqlitedeploy-e2e-XXXXXX)"
trap 'cleanup' EXIT

cleanup() {
  set +e
  echo
  echo "[cleanup] tearing down..."
  if [[ -n "${SQLD_PID:-}" ]] && kill -0 "$SQLD_PID" 2>/dev/null; then
    kill "$SQLD_PID" || true
    wait "$SQLD_PID" 2>/dev/null || true
  fi
  if [[ -d "$HERE" ]]; then
    docker compose -f "$HERE/docker-compose.yml" down -v --remove-orphans >/dev/null 2>&1 || true
  fi
  rm -rf "$WORK" || true
}

check() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing prereq: $1"; exit 1; }
}
for tool in docker curl sqlite3 jq; do check "$tool"; done

if [[ ! -x "$SQLITEDEPLOY" ]]; then
  echo "no sqlitedeploy binary at $SQLITEDEPLOY"
  echo "run \`make build\` first, or set SQLITEDEPLOY=/path/to/sqlitedeploy"
  exit 1
fi

echo "== 1. starting MinIO + bucket-init =="
docker compose -f "$HERE/docker-compose.yml" up -d --wait
trap 'cleanup' EXIT

echo "== 2. sqlitedeploy up --byo-storage --no-tunnel (against MinIO, in background) =="
cd "$WORK"
"$SQLITEDEPLOY" up \
  --byo-storage \
  --no-tunnel \
  --provider s3 \
  --bucket sqlitedeploy-e2e \
  --region us-east-1 \
  --endpoint http://127.0.0.1:9000 \
  --access-key sqlitedeploytest \
  --secret-key sqlitedeploytestsecret \
  --http-listen-addr 127.0.0.1:18080 \
  --grpc-listen-addr 127.0.0.1:15001 \
  --bucket-prefix e2e \
  >"$WORK/sqld.log" 2>&1 &
SQLD_PID=$!

# Wait for Hrana HTTP to come up (up to 15s).
for i in {1..30}; do
  if curl -fsS http://127.0.0.1:18080/health 2>/dev/null; then break; fi
  sleep 0.5
done
if ! curl -fsS http://127.0.0.1:18080/health >/dev/null 2>&1; then
  echo "sqld didn't come up; log:"
  cat "$WORK/sqld.log"
  exit 1
fi

test -f "$WORK/.sqlitedeploy/db.sqlite"
test -f "$WORK/.sqlitedeploy/auth/jwt_public.pem"
test -f "$WORK/.sqlitedeploy/auth/jwt_private.pem"
test -f "$WORK/.sqlitedeploy/auth/replica.jwt"

echo "== 3. write via stock sqlite3 driver =="
sqlite3 "$WORK/.sqlitedeploy/db.sqlite" "CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY, label TEXT);"
sqlite3 "$WORK/.sqlitedeploy/db.sqlite" "INSERT INTO items (label) VALUES ('hello-from-stock-driver');"

echo "== 4. read via Hrana HTTP =="
TOKEN="$(cat "$WORK/.sqlitedeploy/auth/replica.jwt")"
RESP="$(curl -fsS \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"statements":["SELECT label FROM items ORDER BY id DESC LIMIT 1"]}' \
  http://127.0.0.1:18080/v1/execute)"
echo "  Hrana response: $RESP"
LABEL="$(echo "$RESP" | jq -r '.[0].results.rows[0][0] // .[0].rows[0][0] // .[0].results[0].rows[0][0] // empty')"
if [[ "$LABEL" != "hello-from-stock-driver" ]]; then
  echo "FAIL: expected label 'hello-from-stock-driver', got '$LABEL'"
  echo "Full response: $RESP"
  echo "sqld log:"
  cat "$WORK/sqld.log"
  exit 1
fi

echo
echo "✓ E2E smoke test passed"
echo "  - up bootstrapped the primary, started sqld, and Hrana HTTP came up"
echo "  - stock sqlite3 write was readable via Hrana with the replica JWT"
echo
echo "Phase 6.5 (TODO): spin up a replica with --primary-grpc-url, verify"
echo "the same row is visible from there. Requires copying auth files into a"
echo "second tmpdir and running attach against http://127.0.0.1:15001."
