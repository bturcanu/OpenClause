#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT_DIR"

BASE_URL="${BASE_URL:-http://localhost:8090}"
TENANT_ID="${TENANT_ID:-}"
AGENT_ID="${AGENT_ID:-}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@openclause.dev}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-Admin123!}"
ORG_NAME="${ORG_NAME:-OpenClause Smoke Org}"
FIRST_TENANT_NAME="${FIRST_TENANT_NAME:-Smoke Tenant}"

source "$(dirname "${BASH_SOURCE[0]}")/smoke-lib.sh"

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require curl
require jq

ensure_setup_initialized "$BASE_URL" "$ADMIN_EMAIL" "$ADMIN_PASSWORD" "$ORG_NAME" "$FIRST_TENANT_NAME"

TOKEN="$(login_console_admin "$BASE_URL" "$ADMIN_EMAIL" "$ADMIN_PASSWORD")"

discover_first_tenant() {
  curl -fsS "${BASE_URL}/admin/tenants?limit=100" \
    -H "Authorization: Bearer ${TOKEN}" | jq -er 'map(select(.status == "active")) | first | .id'
}

discover_first_agent() {
  local tenant_id="$1"
  curl -fsS "${BASE_URL}/admin/tenants/${tenant_id}/agents?limit=100" \
    -H "Authorization: Bearer ${TOKEN}" | jq -er 'map(select(.status == "active")) | first | .id'
}

has_saved_integration() {
  local tenant_id="$1"
  local agent_id="$2"
  curl -fsS "${BASE_URL}/admin/tenants/${tenant_id}/agents/${agent_id}/integration" \
    -H "Authorization: Bearer ${TOKEN}" >/dev/null
}

bootstrap_agent() {
  local tenant_id="$1"
  local ts
  ts="$(date +%s)"
  curl -fsS -X POST "${BASE_URL}/admin/onboarding/integrations" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H 'Content-Type: application/json' \
    --data "$(jq -nc \
      --arg tenant_id "$tenant_id" \
      --arg agent_name "console-backend-smoke-${ts}" \
      --arg runtime "python" \
      --arg approval_posture "pilot_safe" \
      '{
        tenant_id:$tenant_id,
        agent_name:$agent_name,
        runtime:$runtime,
        approval_posture:$approval_posture,
        tools:[{tool:"postgres",action:"query.readonly"}]
      }')" | jq -er '.agent.id'
}

if [[ -z "$TENANT_ID" ]]; then
  TENANT_ID="$(discover_first_tenant)"
fi

if [[ -z "$AGENT_ID" ]]; then
  AGENT_ID="$(discover_first_agent "$TENANT_ID" 2>/dev/null || true)"
fi

if [[ -z "$AGENT_ID" ]] || ! has_saved_integration "$TENANT_ID" "$AGENT_ID" 2>/dev/null; then
  AGENT_ID="$(bootstrap_agent "$TENANT_ID")"
fi

echo "using tenant=${TENANT_ID} agent=${AGENT_ID}"

curl -fsS -i "${BASE_URL}/admin/events?tenant_id=${TENANT_ID}&limit=5" \
  -H "Authorization: Bearer ${TOKEN}" | tee /tmp/openclause-events.http >/dev/null

events_resp="$(curl -fsS "${BASE_URL}/admin/events?tenant_id=${TENANT_ID}&limit=5" \
  -H "Authorization: Bearer ${TOKEN}")"
echo "$events_resp" | jq -e 'if type=="array" then length >= 0 else false end' >/dev/null
echo "$events_resp" | jq '{event_count:length, latest_event:(.[0].event_id // ""), latest_reason:(.[0].reason // "")}'

curl -fsS -i "${BASE_URL}/admin/tenants/${TENANT_ID}/analytics/summary?range=24h&bucket_minutes=60&top_agents=5" \
  -H "Authorization: Bearer ${TOKEN}" | tee /tmp/openclause-analytics.http >/dev/null

analytics_resp="$(curl -fsS "${BASE_URL}/admin/tenants/${TENANT_ID}/analytics/summary?range=24h&bucket_minutes=60&top_agents=5" \
  -H "Authorization: Bearer ${TOKEN}")"
echo "$analytics_resp" | jq -e '.pilot_health.status != null and (.pilot_health.next_actions | type == "array")' >/dev/null
echo "$analytics_resp" | jq '{pilot_status:.pilot_health.status,next_actions:(.pilot_health.next_actions|length),last_event:(.pilot_health.last_event.event_id // "")}'

curl -fsS -i "${BASE_URL}/admin/tenants/${TENANT_ID}/agents/${AGENT_ID}/integration" \
  -H "Authorization: Bearer ${TOKEN}" | tee /tmp/openclause-integration.http >/dev/null

revisions_resp="$(curl -fsS "${BASE_URL}/admin/tenants/${TENANT_ID}/agents/${AGENT_ID}/integration/revisions?limit=5" \
  -H "Authorization: Bearer ${TOKEN}")"
echo "$revisions_resp" | jq -e '(.revisions | length) >= 1' >/dev/null
echo "$revisions_resp" | jq '{revision_count:(.revisions|length), latest_mode:(.revisions[0].mode // "")}'

echo "console backend smoke complete"
