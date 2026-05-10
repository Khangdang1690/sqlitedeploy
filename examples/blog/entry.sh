#!/bin/sh
set -eu

: "${R2_ACCESS_KEY:?Set with: fly secrets set R2_ACCESS_KEY=...}"
: "${R2_SECRET_KEY:?Set with: fly secrets set R2_SECRET_KEY=...}"
: "${CF_ACCOUNT_ID:?Set with: fly secrets set CF_ACCOUNT_ID=...}"
: "${CF_R2_BUCKET:?Set with: fly secrets set CF_R2_BUCKET=...}"

mkdir -p /data && cd /data

sqlitedeploy up \
  --byo-storage \
  --provider=r2 \
  --bucket="$CF_R2_BUCKET" \
  --account-id="$CF_ACCOUNT_ID" \
  --access-key="$R2_ACCESS_KEY" \
  --secret-key="$R2_SECRET_KEY" \
  --ingress=listen \
  --http-listen-addr 127.0.0.1:8080 &
SQLD_PID=$!

# Sidecar shape: sqld is loopback-only; the Node app on :3000 is the public face.
# No --public-url because nothing outside the container talks to sqld directly.

echo "[entry] waiting for sqld..."
ready=false
for i in $(seq 1 60); do
  if curl -fsS -o /dev/null http://127.0.0.1:8080/health 2>/dev/null \
     && [ -s /data/.sqlitedeploy/auth/replica.jwt ]; then
    ready=true
    echo "[entry] sqld ready after ${i}s"
    break
  fi
  if ! kill -0 "$SQLD_PID" 2>/dev/null; then
    echo "[entry] FATAL: sqld exited during startup. Check fly logs above for sqld output." >&2
    exit 1
  fi
  sleep 1
done
if [ "$ready" != "true" ]; then
  echo "[entry] FATAL: sqld didn't become ready in 60s. Check R2 creds + bucket access." >&2
  exit 1
fi

export LIBSQL_URL="http://127.0.0.1:8080"
export LIBSQL_AUTH_TOKEN="$(cat /data/.sqlitedeploy/auth/replica.jwt)"

echo "[entry] starting blog server..."
exec node /app/dist/index.js
