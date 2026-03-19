#!/usr/bin/env pwsh
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot

if (!(Test-Path "$root/.env")) {
  Copy-Item "$root/.env.example" "$root/.env"
}

#
# CONSOLE_JWT_SECRET is required by docker-compose (compose interpolation uses
# `${CONSOLE_JWT_SECRET:? ... }`). If missing/empty, generate a strong secret
# and write it to `.env` explicitly with a prominent warning.
#
$envPath = Join-Path $root ".env"
$line = Select-String -Path $envPath -Pattern '^CONSOLE_JWT_SECRET=' | Select-Object -First 1
$val = $null
if ($line -and $line.Line -match '^CONSOLE_JWT_SECRET=(.*)$') {
  $val = $Matches[1]
}

if ([string]::IsNullOrWhiteSpace($val)) {
  Write-Host "WARNING: CONSOLE_JWT_SECRET is missing/empty in .env; generating a strong secret for local dev." -ForegroundColor Yellow
  Write-Host "         This does NOT change production defaults; it just unblocks docker compose for this repo checkout." -ForegroundColor Yellow
  $secret = & "$root/scripts/gen-secret.ps1"
  $content = Get-Content -Path $envPath

  $updated = $false
  for ($i=0; $i -lt $content.Count; $i++) {
    if ($content[$i] -match '^CONSOLE_JWT_SECRET=') {
      $content[$i] = "CONSOLE_JWT_SECRET=$secret"
      $updated = $true
    }
  }
  if (-not $updated) {
    $content += "CONSOLE_JWT_SECRET=$secret"
  }
  Set-Content -Path $envPath -Value $content
}

Write-Host ">>> Starting OpenClause stack..."
docker compose --env-file "$root/.env" -f "$root/deploy/docker-compose.yml" up --build -d | Out-Null

Write-Host ">>> Waiting for postgres..."
for ($i = 1; $i -le 30; $i++) {
  $ok = docker compose --env-file "$root/.env" -f "$root/deploy/docker-compose.yml" exec -T postgres `
    pg_isready -U openclause -d openclause 2>$null
  if ($LASTEXITCODE -eq 0) {
    break
  }
  Write-Host ("  postgres not ready, retrying ({0}/30)..." -f $i)
  Start-Sleep -Seconds 2
}

& "$root/scripts/migrate.ps1"

Write-Host ""
Write-Host "✓ Gateway:    http://localhost:8080/healthz"
Write-Host "✓ Approvals:  http://localhost:8081/healthz"
Write-Host "✓ Slack:      http://localhost:8082/healthz"
Write-Host "✓ Jira:       http://localhost:8083/healthz"
Write-Host "✓ OPA:        http://localhost:8181/health"
Write-Host "✓ MinIO:      http://localhost:9001"

