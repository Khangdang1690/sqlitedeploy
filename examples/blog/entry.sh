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
  --public-url="https://$FLY_APP_NAME.fly.dev" &

echo "[entry] waiting for sqld..."
for i in $(seq 1 60); do
  if curl -fsS -o /dev/null http://127.0.0.1:8080/health 2>/dev/null \
     && [ -s /data/.sqlitedeploy/auth/replica.jwt ]; then
    echo "[entry] sqld ready after ${i}s"
    break
  fi
  sleep 1
done

export LIBSQL_URL="http://127.0.0.1:8080"
export LIBSQL_AUTH_TOKEN="$(cat /data/.sqlitedeploy/auth/replica.jwt)"

echo "[entry] starting blog server..."
exec node /app/dist/index.js
