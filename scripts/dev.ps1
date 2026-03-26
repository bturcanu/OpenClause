#!/usr/bin/env pwsh
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot

if (!(Test-Path "$root/.env")) {
  Copy-Item "$root/.env.example" "$root/.env"
}

$envPath = Join-Path $root ".env"

function Ensure-Secret {
  param(
    [string]$Name,
    [string]$Placeholder = "",
    [int]$MinLength = 0
  )

  $pattern = "^$([regex]::Escape($Name))=(.*)$"
  $line = Select-String -Path $envPath -Pattern $pattern | Select-Object -First 1
  $value = ""
  if ($line -and $line.Line -match $pattern) {
    $value = $Matches[1]
  }

  if ([string]::IsNullOrWhiteSpace($value) -or ($Placeholder -ne "" -and $value -eq $Placeholder) -or $value.Length -lt $MinLength) {
    Write-Host "WARNING: $Name is missing/placeholder in .env; generating a strong secret for local dev." -ForegroundColor Yellow
    Write-Host "         This does NOT change production defaults; it just unblocks docker compose for this repo checkout." -ForegroundColor Yellow
    $secret = & "$root/scripts/gen-secret.ps1"
    $content = Get-Content -Path $envPath

    $updated = $false
    for ($i = 0; $i -lt $content.Count; $i++) {
      if ($content[$i] -match "^$([regex]::Escape($Name))=") {
        $content[$i] = "$Name=$secret"
        $updated = $true
      }
    }
    if (-not $updated) {
      $content += "$Name=$secret"
    }
    Set-Content -Path $envPath -Value $content
  }
}

Ensure-Secret -Name "CONSOLE_JWT_SECRET" -Placeholder "change-me-in-production-openclause-jwt-secret" -MinLength 32
Ensure-Secret -Name "INTERNAL_AUTH_TOKEN" -Placeholder "dev-internal-token-change-me" -MinLength 32

& "$root/scripts/validate-env.ps1" --file "$root/.env"

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

Write-Host ">>> Waiting for compose migration service..."
$migrateState = $null
$migrateExitCode = $null
for ($i = 1; $i -le 30; $i++) {
  try {
    $rows = docker compose --env-file "$root/.env" -f "$root/deploy/docker-compose.yml" ps --all --format json |
      ConvertFrom-Json |
      Where-Object { $_.Service -eq "migrate" }
    if ($rows) {
      $row = @($rows)[0]
      $migrateState = $row.State
      $migrateExitCode = "$($row.ExitCode)"
      if ($migrateState -eq "exited" -and $migrateExitCode -eq "0") {
        break
      }
    }
  } catch {
    # Fall through to retry / fallback below.
  }
  Write-Host ("  migrate not complete yet, retrying ({0}/30)..." -f $i)
  Start-Sleep -Seconds 2
}

if ($migrateState -ne "exited" -or $migrateExitCode -ne "0") {
  Write-Host "WARNING: compose migration service did not report a clean exit; running the explicit migration script as a fallback." -ForegroundColor Yellow
  & "$root/scripts/migrate.ps1"
}

Write-Host ">>> Running post-start smoke checks..."
docker compose --env-file "$root/.env" -f "$root/deploy/docker-compose.yml" --profile smoke run --rm poststart-smoke | Out-Null

Write-Host ""
Write-Host "✓ Gateway:    http://localhost:8080/healthz"
Write-Host "✓ Approvals:  http://localhost:8081/healthz"
Write-Host "✓ Slack:      http://localhost:8082/healthz"
Write-Host "✓ Jira:       http://localhost:8083/healthz"
Write-Host "✓ OPA:        http://localhost:8181/health"
Write-Host "✓ MinIO:      http://localhost:9001"
Write-Host "✓ Compose smoke: docker compose --env-file $root/.env -f $root/deploy/docker-compose.yml --profile smoke run --rm poststart-smoke"
