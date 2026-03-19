#!/usr/bin/env pwsh
$ErrorActionPreference = "Stop"

# Outputs a strong base64 secret (>= 32 bytes of randomness).
$bytes = New-Object byte[] 32
[System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
$b64 = [Convert]::ToBase64String($bytes)
Write-Output $b64

