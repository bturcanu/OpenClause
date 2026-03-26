#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

if [ ! -f .env ]; then
  cp .env.example .env
fi

ensure_secret() {
  local key="$1"
  local placeholder="${2:-}"
  local min_len="${3:-0}"
  local value
  local needs_secret=0

  value="$(grep -E "^${key}=" .env | head -n1 | cut -d= -f2- || true)"
  if [ -z "$value" ] || { [ -n "$placeholder" ] && [ "$value" = "$placeholder" ]; } || [ "${#value}" -lt "$min_len" ]; then
    needs_secret=1
  fi

  if [ "$needs_secret" -eq 1 ]; then
    echo "WARNING: ${key} is missing/placeholder in .env; generating a strong secret for local dev."
    echo "         This does NOT change production defaults; it just unblocks docker compose for this repo checkout."
    secret="$(bash ./scripts/gen-secret.sh)"
    tmp="$(mktemp)"
    awk -v key="$key" -v secret="$secret" '
      BEGIN { done=0; prefix=key "=" }
      index($0, prefix) == 1 { print prefix secret; done=1; next }
      { print }
      END { if (done==0) print prefix secret }
    ' .env > "$tmp"
    mv "$tmp" .env
  fi
}

ensure_secret CONSOLE_JWT_SECRET "change-me-in-production-openclause-jwt-secret" 32
ensure_secret INTERNAL_AUTH_TOKEN "dev-internal-token-change-me" 32

bash ./scripts/validate-env.sh --file .env

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

echo ">>> Waiting for compose migration service..."
i=1
while [ "$i" -le 30 ]; do
  status="$(docker compose --env-file .env -f deploy/docker-compose.yml ps --all --format json 2>/dev/null | jq -sr 'map(select(.Service=="migrate")) | .[0].State // empty' 2>/dev/null || true)"
  if [ "$status" = "exited" ]; then
    exit_code="$(docker compose --env-file .env -f deploy/docker-compose.yml ps --all --format json 2>/dev/null | jq -sr 'map(select(.Service=="migrate")) | .[0].ExitCode // empty' 2>/dev/null || true)"
    if [ "$exit_code" = "0" ]; then
      break
    fi
  fi
  echo "  migrate not complete yet, retrying ($i/30)..."
  sleep 2
  i=$((i+1))
done

if [ "${status:-}" != "exited" ] || [ "${exit_code:-}" != "0" ]; then
  echo "WARNING: compose migration service did not report a clean exit; running the explicit migration script as a fallback."
  bash ./scripts/migrate.sh
fi

echo ">>> Running post-start smoke checks..."
docker compose --env-file .env -f deploy/docker-compose.yml --profile smoke run --rm poststart-smoke

echo ""
echo "✓ Gateway:    http://localhost:8080/healthz"
echo "✓ Approvals:  http://localhost:8081/healthz"
echo "✓ Slack:      http://localhost:8082/healthz"
echo "✓ Jira:       http://localhost:8083/healthz"
echo "✓ OPA:        http://localhost:8181/health"
echo "✓ MinIO:      http://localhost:9001"
echo "✓ Compose smoke: docker compose --env-file .env -f deploy/docker-compose.yml --profile smoke run --rm poststart-smoke"
