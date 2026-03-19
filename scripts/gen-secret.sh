#!/usr/bin/env bash
set -euo pipefail

# Outputs a strong base64 secret (>= 32 bytes of randomness).
# Example: export CONSOLE_JWT_SECRET="$(./scripts/gen-secret.sh)"

if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl not found (needed for ./scripts/gen-secret.sh)" >&2
  exit 1
fi

openssl rand -base64 32

