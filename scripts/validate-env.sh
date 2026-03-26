#!/usr/bin/env bash
set -euo pipefail

file=".env"
strict=0

usage() {
  cat <<'EOF'
Usage: ./scripts/validate-env.sh [--file PATH] [--strict]

Validates the OpenClause environment file before startup.
--strict turns placeholder/dev-secret warnings into hard failures.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --file)
      file="${2:-}"
      shift 2
      ;;
    --strict)
      strict=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ ! -f "$file" ]]; then
  echo "ERROR: env file not found: $file" >&2
  exit 1
fi

get_var() {
  local key="$1"
  awk -F= -v key="$key" '$1 == key { value = substr($0, index($0, "=") + 1) } END { print value }' "$file"
}

failures=0
warnings=0

error() {
  echo "ERROR: $*" >&2
  failures=$((failures + 1))
}

warn() {
  echo "WARNING: $*" >&2
  warnings=$((warnings + 1))
}

require_value() {
  local key="$1"
  local value
  value="$(get_var "$key")"
  if [[ -z "$value" ]]; then
    error "$key is missing or empty in $file"
  fi
}

warn_or_fail_placeholder() {
  local key="$1"
  local value
  value="$(get_var "$key")"
  shift
  if [[ -z "$value" ]]; then
    return 0
  fi
  for candidate in "$@"; do
    if [[ "$value" == "$candidate" ]]; then
      if [[ "$strict" -eq 1 ]]; then
        error "$key uses placeholder/dev value '$value' in strict mode"
      else
        warn "$key uses placeholder/dev value '$value'"
      fi
      return 0
    fi
  done
}

require_url() {
  local key="$1"
  local value
  value="$(get_var "$key")"
  if [[ -z "$value" ]]; then
    return 0
  fi
  case "$value" in
    http://*|https://*)
      ;;
    *)
      error "$key must be an absolute http(s) URL, got '$value'"
      ;;
  esac
}

require_value POSTGRES_PASSWORD
require_value INTERNAL_AUTH_TOKEN
require_value CONSOLE_JWT_SECRET

console_jwt_secret="$(get_var CONSOLE_JWT_SECRET)"
if [[ -n "$console_jwt_secret" && ${#console_jwt_secret} -lt 32 ]]; then
  error "CONSOLE_JWT_SECRET must be at least 32 characters"
fi

require_url OPA_URL
require_url APPROVALS_URL
require_url PUBLIC_APPROVALS_URL
require_url GATEWAY_URL
require_url PUBLIC_GATEWAY_URL
require_url PUBLIC_BASE_URL

warn_or_fail_placeholder POSTGRES_PASSWORD "changeme"
warn_or_fail_placeholder INTERNAL_AUTH_TOKEN "dev-internal-token-change-me"
warn_or_fail_placeholder CONSOLE_JWT_SECRET "change-me-in-production-openclause-jwt-secret"
warn_or_fail_placeholder EVIDENCE_S3_ACCESS_KEY "minioadmin"
warn_or_fail_placeholder EVIDENCE_S3_SECRET_KEY "minioadmin"
warn_or_fail_placeholder SLACK_BOT_TOKEN "xoxb-test-token"
warn_or_fail_placeholder JIRA_API_TOKEN "your-jira-token"

postgres_sslmode="$(get_var POSTGRES_SSLMODE)"
if [[ "$postgres_sslmode" == "disable" ]]; then
  if [[ "$strict" -eq 1 ]]; then
    error "POSTGRES_SSLMODE=disable is not allowed in strict mode"
  else
    warn "POSTGRES_SSLMODE=disable is acceptable for local dev only"
  fi
fi

if [[ "$failures" -gt 0 ]]; then
  echo "env validation failed with $failures error(s) and $warnings warning(s)" >&2
  exit 1
fi

echo "env validation passed for $file ($warnings warning(s))"
