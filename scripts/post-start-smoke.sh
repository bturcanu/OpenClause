#!/usr/bin/env sh
set -eu

check() {
  name="$1"
  url="$2"
  marker="${3:-}"
  echo ">>> $name"
  body="$(curl -fsS "$url")"
  if [ -n "$marker" ]; then
    body_lower="$(printf '%s' "$body" | tr '[:upper:]' '[:lower:]')"
    marker_lower="$(printf '%s' "$marker" | tr '[:upper:]' '[:lower:]')"
    case "$body_lower" in
      *"$marker_lower"*)
        ;;
      *)
        echo "post-start smoke failed: $name response from $url did not include marker '$marker'" >&2
        exit 1
        ;;
    esac
  fi
}

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
APPROVALS_URL="${APPROVALS_URL:-http://localhost:8081}"
SLACK_URL="${CONNECTOR_SLACK_URL:-http://localhost:8082}"
JIRA_URL="${CONNECTOR_JIRA_URL:-http://localhost:8083}"
CONSOLE_API_URL="${CONSOLE_API_URL:-http://localhost:8090}"
OPA_URL="${OPA_URL:-http://localhost:8181}"

check "gateway health" "${GATEWAY_URL}/healthz" "ok"
check "approvals health" "${APPROVALS_URL}/healthz" "ok"
check "slack connector health" "${SLACK_URL}/healthz" "ok"
check "jira connector health" "${JIRA_URL}/healthz" "ok"
check "console-api health" "${CONSOLE_API_URL}/healthz" "ok"
check "OPA health" "${OPA_URL}/health" "{}"
check "gateway connector catalog" "${GATEWAY_URL}/v1/connectors" "name"
check "console-api setup status" "${CONSOLE_API_URL}/setup/status" "\"initialized\""

echo "post-start smoke passed"
