#!/usr/bin/env bash

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8090}"
GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@openclause.dev}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-Admin123!}"
ORG_NAME="${ORG_NAME:-OpenClause Smoke Org}"
FIRST_TENANT_NAME="${FIRST_TENANT_NAME:-Smoke Tenant}"
PREVIEW_TENANT_ID="${PREVIEW_TENANT_ID:-}"

source "$(dirname "${BASH_SOURCE[0]}")/smoke-lib.sh"

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require curl
require jq

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT

request() {
  local method="$1"
  local url="$2"
  local body="${3:-}"
  shift 3 || true

  if [[ -n "$body" ]]; then
    curl -fsS -X "$method" "$url" "$@" \
      -H 'Content-Type: application/json' \
      --data "$body"
  else
    curl -fsS -X "$method" "$url" "$@"
  fi
}

resolve_preview_tenant_id() {
  local requested="$1"
  local tenants
  tenants="$(request GET "$BASE_URL/admin/tenants?limit=100" "" -H "Authorization: Bearer $TOKEN")"
  if [[ -z "$requested" ]]; then
    echo "$tenants" | jq -er 'map(select(.status == "active")) | first | .id'
    return
  fi
  echo "$tenants" | jq -er --arg requested "$requested" '
    map(select(.id == $requested or .name == $requested))
    | first
    | .id
  '
}

echo "== login =="
ensure_setup_initialized "$BASE_URL" "$ADMIN_EMAIL" "$ADMIN_PASSWORD" "$ORG_NAME" "$FIRST_TENANT_NAME"
TOKEN="$(
  request POST "$BASE_URL/auth/login" \
    "$(jq -nc --arg email "$ADMIN_EMAIL" --arg password "$ADMIN_PASSWORD" '{email:$email,password:$password}')" \
  | jq -er '.token'
)"
echo "token acquired"

PREVIEW_TENANT_ID="$(resolve_preview_tenant_id "$PREVIEW_TENANT_ID")"
echo "preview tenant: $PREVIEW_TENANT_ID"

ts="$(date +%s)"

echo "== preview =="
preview_body="$(
  jq -nc \
    --arg tenant_id "$PREVIEW_TENANT_ID" \
    --arg agent_name "curl-preview-$ts" \
    --arg runtime "typescript" \
    --arg approval_posture "pilot_safe" \
    --arg environment_label "dev" \
    --arg owner_name "curl-smoke" \
    --arg description "live preview smoke" \
    '{
      tenant_id:$tenant_id,
      agent_name:$agent_name,
      runtime:$runtime,
      approval_posture:$approval_posture,
      environment_label:$environment_label,
      owner_name:$owner_name,
      description:$description,
      tools:[{tool:"postgres",action:"query.readonly"}]
    }'
)"
preview_resp="$(
  request POST "$BASE_URL/admin/onboarding/bundles/preview" "$preview_body" \
    -H "Authorization: Bearer $TOKEN"
)"
echo "$preview_resp" | jq -e '.mode == "preview" and .agent.preview == true and .bundle.runtime == "typescript" and (.bundle.artifacts | length) > 0' >/dev/null
echo "$preview_resp" | jq '{mode,preview:.agent.preview,runtime:.bundle.runtime,artifact_count:(.bundle.artifacts|length)}'

echo "== create =="
create_body="$(
  jq -nc \
    --arg new_tenant_name "curl-smoke-$ts" \
    --arg agent_name "curl-agent-$ts" \
    --arg runtime "python" \
    --arg approval_posture "pilot_safe" \
    --arg environment_label "dev" \
    --arg owner_name "curl-smoke" \
    --arg description "live create smoke" \
    '{
      new_tenant_name:$new_tenant_name,
      agent_name:$agent_name,
      runtime:$runtime,
      approval_posture:$approval_posture,
      environment_label:$environment_label,
      owner_name:$owner_name,
      description:$description,
      tools:[{tool:"postgres",action:"query.readonly"}]
    }'
)"
create_resp="$(
  request POST "$BASE_URL/admin/onboarding/integrations" "$create_body" \
    -H "Authorization: Bearer $TOKEN"
)"
echo "$create_resp" | jq -e '.mode == "created" and (.api_key.raw_key | type == "string") and (.api_key.raw_key | length > 0) and (.bundle.runtime == "python")' >/dev/null
echo "$create_resp" | jq '{mode,tenant_id:.tenant.id,agent_id:.agent.id,has_raw_key:(.api_key.raw_key|type=="string" and length>0),runtime:.bundle.runtime,artifact_count:(.bundle.artifacts|length)}'

tenant_id="$(echo "$create_resp" | jq -r '.tenant.id')"
agent_id="$(echo "$create_resp" | jq -r '.agent.id')"
api_key="$(echo "$create_resp" | jq -r '.api_key.raw_key')"

echo "== direct archive =="
archive_headers="$tmpdir/archive.headers"
archive_path="$tmpdir/onboarding.zip"
curl -fsS \
  -D "$archive_headers" \
  -o "$archive_path" \
  -X POST "$BASE_URL/admin/onboarding/bundles/archive" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  --data "$create_resp"
grep -qi '^content-type: application/zip' "$archive_headers"
[[ -s "$archive_path" ]]
echo "archive_bytes=$(wc -c < "$archive_path" | tr -d ' ')"

echo "== governed toolcall =="
toolcall_body="$(
  jq -nc \
    --arg tenant_id "$tenant_id" \
    --arg agent_id "$agent_id" \
    --arg session_id "curl-session-$ts" \
    --arg trace_id "curl-trace-$ts" \
    --arg idem "curl-idem-$ts" \
    '{
      tenant_id:$tenant_id,
      agent_id:$agent_id,
      tool:"postgres",
      action:"query.readonly",
      resource:"demo-users",
      session_id:$session_id,
      trace_id:$trace_id,
      idempotency_key:$idem,
      params:{
        sql:"select id, name, email, created_at from demo_users order by created_at desc limit 3",
        params:[]
      }
    }'
)"
toolcall_resp="$(
  request POST "$GATEWAY_URL/v1/toolcalls" "$toolcall_body" \
    -H "X-API-Key: $api_key"
)"
echo "$toolcall_resp" | jq -e '(.event_id | type == "string") and .decision == "allow"' >/dev/null
echo "$toolcall_resp" | jq '{event_id,decision,result}'

echo "== saved integration =="
integration_resp="$(
  request GET "$BASE_URL/admin/tenants/$tenant_id/agents/$agent_id/integration" "" \
    -H "Authorization: Bearer $TOKEN"
)"
echo "$integration_resp" | jq -e '.runtime == "python" and (.tools | length) == 1' >/dev/null
echo "$integration_resp" | jq '{runtime,approval_posture,tool_count:(.tools|length)}'

echo "== revisions =="
revisions_resp="$(
  request GET "$BASE_URL/admin/tenants/$tenant_id/agents/$agent_id/integration/revisions?limit=5" "" \
    -H "Authorization: Bearer $TOKEN"
)"
echo "$revisions_resp" | jq -e '(.revisions | length) >= 1' >/dev/null
echo "$revisions_resp" | jq '{count:(.revisions|length),modes:(.revisions|map(.mode))}'

echo "== saved bundle =="
saved_bundle_resp="$(
  request GET "$BASE_URL/admin/tenants/$tenant_id/agents/$agent_id/integration/bundle" "" \
    -H "Authorization: Bearer $TOKEN"
)"
echo "$saved_bundle_resp" | jq -e '.mode == "fetched" and (.integration != null) and (.bundle.runtime == "python") and (.bundle.environment.OPENCLAUSE_BASE_URL == "http://localhost:8080")' >/dev/null
echo "$saved_bundle_resp" | jq '{mode,has_integration:(.integration!=null),has_api_key:(.api_key!=null),runtime:.bundle.runtime,base_url:.bundle.environment.OPENCLAUSE_BASE_URL}'

echo "== saved bundle archive =="
saved_archive_headers="$tmpdir/saved-archive.headers"
saved_archive_path="$tmpdir/saved-onboarding.zip"
curl -fsS \
  -D "$saved_archive_headers" \
  -o "$saved_archive_path" \
  "$BASE_URL/admin/tenants/$tenant_id/agents/$agent_id/integration/bundle?defaults=true&archive=true" \
  -H "Authorization: Bearer $TOKEN"
grep -qi '^content-type: application/zip' "$saved_archive_headers"
[[ -s "$saved_archive_path" ]]
echo "saved_archive_bytes=$(wc -c < "$saved_archive_path" | tr -d ' ')"

echo "== regenerate =="
regenerate_body="$(
  jq -nc \
    --arg tenant_id "$tenant_id" \
    --arg agent_id "$agent_id" \
    '{
      tenant_id:$tenant_id,
      agent_id:$agent_id,
      runtime:"python",
      approval_posture:"tenant_default",
      tools:[{tool:"postgres",action:"query.readonly"}]
    }'
)"
regenerate_resp="$(
  request POST "$BASE_URL/admin/onboarding/bundles/regenerate" "$regenerate_body" \
    -H "Authorization: Bearer $TOKEN"
)"
echo "$regenerate_resp" | jq -e '.mode == "regenerated" and (.api_key.raw_key? // "" | length == 0) and (.api_key.key_prefix | type == "string")' >/dev/null
echo "$regenerate_resp" | jq '{mode,has_raw_key:(.api_key.raw_key? // "" | length > 0),key_prefix:.api_key.key_prefix}'

echo "== regenerate defaults =="
defaults_resp="$(
  request POST "$BASE_URL/admin/onboarding/bundles/regenerate-defaults" \
    "$(jq -nc --arg tenant_id "$tenant_id" --arg agent_id "$agent_id" '{tenant_id:$tenant_id,agent_id:$agent_id}')" \
    -H "Authorization: Bearer $TOKEN"
)"
echo "$defaults_resp" | jq -e '.mode == "regenerated_defaults" and (.bundle.applied_defaults | length) >= 3' >/dev/null
echo "$defaults_resp" | jq '{mode,applied_defaults:(.bundle.applied_defaults|length),runtime:.bundle.runtime}'

echo "== events =="
events_resp="$(
  request GET "$BASE_URL/admin/events?tenant_id=$tenant_id&limit=5" "" \
    -H "Authorization: Bearer $TOKEN"
)"
echo "$events_resp" | jq -e '(type == "array") and length >= 1 and .[0].reason != null' >/dev/null
echo "$events_resp" | jq 'map({event_id,tool,action,decision,reason})[0:3]'

echo "== sessions =="
sessions_resp="$(
  request GET "$BASE_URL/admin/sessions?tenant_id=$tenant_id&limit=5" "" \
    -H "Authorization: Bearer $TOKEN"
)"
echo "$sessions_resp" | jq -e '(type == "array") and length >= 1 and .[0].id != null' >/dev/null
echo "$sessions_resp" | jq 'map({session_id:.id,event_count,last_event_at,agent_id})[0:3]'

echo "== analytics =="
analytics_resp="$(
  request GET "$BASE_URL/admin/tenants/$tenant_id/analytics/summary?range=24h&bucket_minutes=60&top_agents=5" "" \
    -H "Authorization: Bearer $TOKEN"
)"
echo "$analytics_resp" | jq -e '.onboarding_checklist.has_toolcall == true and .onboarding_checklist.has_execution == true and (.pilot_health.last_event.event_id | type == "string") and (.pilot_health.last_session.session_id | type == "string") and (.pilot_health.next_actions | type == "array")' >/dev/null
echo "$analytics_resp" | jq '{has_toolcall:.onboarding_checklist.has_toolcall,has_execution:.onboarding_checklist.has_execution,last_event:(.pilot_health.last_event.event_id // ""),last_session:(.pilot_health.last_session.session_id // ""),next_best_actions:(.pilot_health.next_actions|length)}'

echo "onboarding curl smoke passed for tenant=$tenant_id agent=$agent_id"
