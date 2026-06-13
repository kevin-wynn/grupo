#!/usr/bin/env bash
set -euo pipefail

BASE="${GRUPO_URL:-http://127.0.0.1:8080}"
SECRET="${GITHUB_WEBHOOK_SECRET:-dev-webhook-secret}"
PAYLOAD="$(cat fixtures/webhook-push.json)"

SIG="sha256=$(printf '%s' "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" | awk '{print $2}')"

curl -sf -X POST "$BASE/webhooks/github" \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: push" \
  -H "X-Hub-Signature-256: $SIG" \
  --data "$PAYLOAD"

echo
echo "webhook accepted"
