#!/usr/bin/env pwsh
$ErrorActionPreference = "Stop"

$file = ".env"
$strict = $false

for ($i = 0; $i -lt $args.Length; $i++) {
  switch ($args[$i]) {
    "--file" {
      $i++
      if ($i -ge $args.Length) { throw "--file requires a path" }
      $file = $args[$i]
    }
    "--strict" { $strict = $true }
    "-h" { Write-Host "Usage: ./scripts/validate-env.ps1 [--file PATH] [--strict]"; exit 0 }
    "--help" { Write-Host "Usage: ./scripts/validate-env.ps1 [--file PATH] [--strict]"; exit 0 }
    default { throw "unknown argument: $($args[$i])" }
  }
}

if (!(Test-Path $file)) {
  throw "env file not found: $file"
}

$values = @{}
Get-Content $file | ForEach-Object {
  if ($_ -match '^\s*#' -or $_ -notmatch '=') { return }
  $parts = $_.Split('=', 2)
  $values[$parts[0]] = if ($parts.Length -gt 1) { $parts[1] } else { "" }
}

$failures = 0
$warnings = 0

function Add-EnvError([string]$message) {
  Write-Host "ERROR: $message" -ForegroundColor Red
  $script:failures++
}

function Add-EnvWarning([string]$message) {
  Write-Host "WARNING: $message" -ForegroundColor Yellow
  $script:warnings++
}

function Require-Value([string]$key) {
  if (-not $values.ContainsKey($key) -or [string]::IsNullOrWhiteSpace($values[$key])) {
    Add-EnvError "$key is missing or empty in $file"
  }
}

function Validate-Url([string]$key) {
  if (-not $values.ContainsKey($key) -or [string]::IsNullOrWhiteSpace($values[$key])) { return }
  if ($values[$key] -notmatch '^https?://') {
    Add-EnvError "$key must be an absolute http(s) URL, got '$($values[$key])'"
  }
}

function Warn-Or-FailPlaceholder([string]$key, [string[]]$candidates) {
  if (-not $values.ContainsKey($key)) { return }
  foreach ($candidate in $candidates) {
    if ($values[$key] -eq $candidate) {
      if ($strict) {
        Add-EnvError "$key uses placeholder/dev value '$candidate' in strict mode"
      } else {
        Add-EnvWarning "$key uses placeholder/dev value '$candidate'"
      }
      return
    }
  }
}

Require-Value "POSTGRES_PASSWORD"
Require-Value "INTERNAL_AUTH_TOKEN"
Require-Value "CONSOLE_JWT_SECRET"

if ($values.ContainsKey("CONSOLE_JWT_SECRET") -and $values["CONSOLE_JWT_SECRET"].Length -lt 32) {
  Add-EnvError "CONSOLE_JWT_SECRET must be at least 32 characters"
}

@("OPA_URL", "APPROVALS_URL", "PUBLIC_APPROVALS_URL", "GATEWAY_URL", "PUBLIC_GATEWAY_URL", "PUBLIC_BASE_URL") | ForEach-Object {
  Validate-Url $_
}

Warn-Or-FailPlaceholder "POSTGRES_PASSWORD" @("changeme")
Warn-Or-FailPlaceholder "INTERNAL_AUTH_TOKEN" @("dev-internal-token-change-me")
Warn-Or-FailPlaceholder "CONSOLE_JWT_SECRET" @("change-me-in-production-openclause-jwt-secret")
Warn-Or-FailPlaceholder "EVIDENCE_S3_ACCESS_KEY" @("minioadmin")
Warn-Or-FailPlaceholder "EVIDENCE_S3_SECRET_KEY" @("minioadmin")
Warn-Or-FailPlaceholder "SLACK_BOT_TOKEN" @("xoxb-test-token")
Warn-Or-FailPlaceholder "JIRA_API_TOKEN" @("your-jira-token")

if ($values.ContainsKey("POSTGRES_SSLMODE") -and $values["POSTGRES_SSLMODE"] -eq "disable") {
  if ($strict) {
    Add-EnvError "POSTGRES_SSLMODE=disable is not allowed in strict mode"
  } else {
    Add-EnvWarning "POSTGRES_SSLMODE=disable is acceptable for local dev only"
  }
}

if ($failures -gt 0) {
  throw "env validation failed with $failures error(s) and $warnings warning(s)"
}

Write-Host "env validation passed for $file ($warnings warning(s))"
