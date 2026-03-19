#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo ">>> Running migrations..."
docker compose --env-file .env -f deploy/docker-compose.yml exec -T postgres \
  psql -U openclause -d openclause < migrations/001_initial.sql

echo "✓ Migrations complete (001_initial)"

