#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo ">>> Seeding dev data (docs/seed_dev.sql)..."
docker compose --env-file .env -f deploy/docker-compose.yml exec -T postgres \
  psql -U openclause -d openclause < docs/seed_dev.sql

echo "✓ Seed complete"

