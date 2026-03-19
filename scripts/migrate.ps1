#!/usr/bin/env pwsh
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot

Write-Host ">>> Running migrations..."
docker compose --env-file "$root/.env" -f "$root/deploy/docker-compose.yml" exec -T postgres `
  psql -U openclause -d openclause < "$root/migrations/001_initial.sql"

Write-Host "✓ Migrations complete (001_initial)"

