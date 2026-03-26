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

if [[ -z "$TENANT_ID" ]]; then
  TENANT_ID="$(curl -fsS "${BASE_URL}/admin/tenants?limit=100" \
    -H "Authorization: Bearer ${TOKEN}" | jq -er 'map(select(.status == "active")) | first | .id')"
fi

if [[ -z "$AGENT_ID" ]]; then
  AGENT_ID="$(curl -fsS "${BASE_URL}/admin/tenants/${TENANT_ID}/agents?limit=100" \
    -H "Authorization: Bearer ${TOKEN}" | jq -er 'map(select(.status == "active")) | first | .id' 2>/dev/null || true)"
fi

if [[ -n "$AGENT_ID" ]]; then
  if ! curl -fsS "${BASE_URL}/admin/tenants/${TENANT_ID}/agents/${AGENT_ID}/integration" \
    -H "Authorization: Bearer ${TOKEN}" >/dev/null 2>/dev/null; then
    AGENT_ID=""
  fi
fi

if [[ -z "$AGENT_ID" ]]; then
  AGENT_ID="$(
    curl -fsS -X POST "${BASE_URL}/admin/onboarding/integrations" \
      -H "Authorization: Bearer ${TOKEN}" \
      -H 'Content-Type: application/json' \
      --data "$(jq -nc \
        --arg tenant_id "$TENANT_ID" \
        --arg agent_name "incontainer-smoke-$(date +%s)" \
        '{tenant_id:$tenant_id,agent_name:$agent_name,runtime:"python",approval_posture:"pilot_safe",tools:[{tool:"postgres",action:"query.readonly"}]}')" \
      | jq -er '.agent.id'
  )"
fi

read -r -d '' INNER_SCRIPT <<EOF || true
set -eu
BASE_URL=http://127.0.0.1:8090
TOKEN=\$(wget -qO- --post-data='{"email":"${ADMIN_EMAIL}","password":"${ADMIN_PASSWORD}"}' \
  --header='Content-Type: application/json' \
  "\$BASE_URL/auth/login" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
test -n "\$TOKEN"

echo "events:"
wget -qO- --header="Authorization: Bearer \$TOKEN" \
  "\${BASE_URL}/admin/events?tenant_id=${TENANT_ID}&limit=5" | sed -n '1,5p'

echo "analytics:"
wget -qO- --header="Authorization: Bearer \$TOKEN" \
  "\${BASE_URL}/admin/tenants/${TENANT_ID}/analytics/summary?range=24h&bucket_minutes=60&top_agents=5" | sed -n '1,12p'

echo "integration:"
wget -qO- --header="Authorization: Bearer \$TOKEN" \
  "\${BASE_URL}/admin/tenants/${TENANT_ID}/agents/${AGENT_ID}/integration" | sed -n '1,12p'

echo "revisions:"
wget -qO- --header="Authorization: Bearer \$TOKEN" \
  "\${BASE_URL}/admin/tenants/${TENANT_ID}/agents/${AGENT_ID}/integration/revisions?limit=5" | sed -n '1,12p'
EOF

docker compose --env-file .env -f deploy/docker-compose.yml exec -T console-api sh -lc "$INNER_SCRIPT"

echo "backend smoke complete"
