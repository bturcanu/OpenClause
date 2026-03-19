#!/usr/bin/env pwsh
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot

Write-Host ">>> Seeding dev data (docs/seed_dev.sql)..."
docker compose --env-file "$root/.env" -f "$root/deploy/docker-compose.yml" exec -T postgres `
  psql -U openclause -d openclause < "$root/docs/seed_dev.sql"

Write-Host "✓ Seed complete"

