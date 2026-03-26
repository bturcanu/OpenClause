#!/usr/bin/env pwsh
$ErrorActionPreference = "Stop"

function Check-Endpoint {
  param(
    [string]$Name,
    [string]$Url,
    [string]$Marker = ""
  )

  Write-Host ">>> $Name"
  $body = (Invoke-WebRequest -UseBasicParsing -Uri $Url).Content
  if ($Marker -and $body -notmatch [regex]::Escape($Marker)) {
    throw "post-start smoke failed: $Name response from $Url did not include marker '$Marker'"
  }
}

function Check-EndpointContainsIgnoreCase {
  param(
    [string]$Name,
    [string]$Url,
    [string]$Marker = ""
  )

  Write-Host ">>> $Name"
  $body = (Invoke-WebRequest -UseBasicParsing -Uri $Url).Content
  if ($Marker) {
    if (-not ($body.ToLowerInvariant().Contains($Marker.ToLowerInvariant()))) {
      throw "post-start smoke failed: $Name response from $Url did not include marker '$Marker'"
    }
  }
}

$gatewayUrl = if ($env:GATEWAY_URL) { $env:GATEWAY_URL } else { "http://localhost:8080" }
$approvalsUrl = if ($env:APPROVALS_URL) { $env:APPROVALS_URL } else { "http://localhost:8081" }
$slackUrl = if ($env:CONNECTOR_SLACK_URL) { $env:CONNECTOR_SLACK_URL } else { "http://localhost:8082" }
$jiraUrl = if ($env:CONNECTOR_JIRA_URL) { $env:CONNECTOR_JIRA_URL } else { "http://localhost:8083" }
$consoleApiUrl = if ($env:CONSOLE_API_URL) { $env:CONSOLE_API_URL } else { "http://localhost:8090" }
$opaUrl = if ($env:OPA_URL) { $env:OPA_URL } else { "http://localhost:8181" }

Check-EndpointContainsIgnoreCase "gateway health" "$gatewayUrl/healthz" "ok"
Check-EndpointContainsIgnoreCase "approvals health" "$approvalsUrl/healthz" "ok"
Check-EndpointContainsIgnoreCase "slack connector health" "$slackUrl/healthz" "ok"
Check-EndpointContainsIgnoreCase "jira connector health" "$jiraUrl/healthz" "ok"
Check-EndpointContainsIgnoreCase "console-api health" "$consoleApiUrl/healthz" "ok"
Check-Endpoint "OPA health" "$opaUrl/health" "{}"
Check-EndpointContainsIgnoreCase "gateway connector catalog" "$gatewayUrl/v1/connectors" "name"
Check-EndpointContainsIgnoreCase "console-api setup status" "$consoleApiUrl/setup/status" '"initialized"'

Write-Host "post-start smoke passed"
