#!/usr/bin/env bash
set -euo pipefail

BASE="${GRUPO_URL:-http://127.0.0.1:8080}"
ADMIN_HOST="${GRUPO_ADMIN_DOMAIN:-admin.mygrupo.dev}"
SITE_HOST="${GRUPO_SITE_DOMAIN:-app1.mygrupo.dev}"

echo "== healthz"
curl -sf "$BASE/healthz" | grep -q ok

echo "== admin UI/API reachable"
curl -sf -H "Host: $ADMIN_HOST" "$BASE/api/settings" >/dev/null

echo "== site routing"
if curl -sf -H "Host: $SITE_HOST" "$BASE/" >/dev/null 2>&1; then
  echo "site responded"
else
  echo "site not deployed yet (run make seed && make deploy-fixture)"
fi

echo "== unknown host 404"
code=$(curl -sf -o /dev/null -w "%{http_code}" -H "Host: unknown.mygrupo.dev" "$BASE/" || true)
if [ "$code" != "404" ]; then
  echo "expected 404, got $code"
  exit 1
fi

echo "smoke checks passed"
