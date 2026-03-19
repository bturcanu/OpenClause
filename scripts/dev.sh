#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

if [ ! -f .env ]; then
  cp .env.example .env
fi

CONSOLE_JWT_SECRET_VAL="$(grep -E '^CONSOLE_JWT_SECRET=' .env | head -n1 | cut -d= -f2- || true)"
if [ -z "${CONSOLE_JWT_SECRET_VAL}" ]; then
  echo "WARNING: CONSOLE_JWT_SECRET is missing/empty in .env; generating a strong secret for local dev."
  echo "         This does NOT change production defaults; it just unblocks docker compose for this repo checkout."
  secret="$("./scripts/gen-secret.sh")"

  tmp="$(mktemp)"
  awk -v secret="$secret" '
    BEGIN { done=0 }
    /^CONSOLE_JWT_SECRET=/ { print "CONSOLE_JWT_SECRET=" secret; done=1; next }
    { print }
    END { if (done==0) print "CONSOLE_JWT_SECRET=" secret }
  ' .env > "$tmp"
  mv "$tmp" .env
fi

echo ">>> Starting OpenClause stack..."
docker compose --env-file .env -f deploy/docker-compose.yml up --build -d

echo ">>> Waiting for postgres..."
i=1
while [ "$i" -le 30 ]; do
  if docker compose --env-file .env -f deploy/docker-compose.yml exec -T postgres pg_isready -U openclause -d openclause >/dev/null 2>&1; then
    break
  fi
  echo "  postgres not ready, retrying ($i/30)..."
  sleep 2
  i=$((i+1))
done

./scripts/migrate.sh

echo ""
echo "✓ Gateway:    http://localhost:8080/healthz"
echo "✓ Approvals:  http://localhost:8081/healthz"
echo "✓ Slack:      http://localhost:8082/healthz"
echo "✓ Jira:       http://localhost:8083/healthz"
echo "✓ OPA:        http://localhost:8181/health"
echo "✓ MinIO:      http://localhost:9001"

