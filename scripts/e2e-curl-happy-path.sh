#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required (used to parse JSON responses)" >&2
  exit 1
fi

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
CONSOLE_API_URL="${CONSOLE_API_URL:-http://localhost:8090}"
CONSOLE_UI_URL="${CONSOLE_UI_URL:-http://localhost:3000}"

# Pre-seeded by docs/seed_dev.sql (via ./scripts/seed-dev.sh).
TENANT_ID="${TENANT_ID:-tenant1}"
AGENT_ID="${AGENT_ID:-agent-1}"
GATEWAY_API_KEY="${GATEWAY_API_KEY:-sk-test-key-1}"

INTERNAL_TOKEN="${INTERNAL_TOKEN:-dev-internal-token-change-me}"

ADMIN_EMAIL="${ADMIN_EMAIL:-admin@openclause.dev}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin123}"
ORG_NAME="${ORG_NAME:-OpenClause Smoke Org}"
FIRST_TENANT_NAME="${FIRST_TENANT_NAME:-Smoke Tenant}"

APPROVER_EMAIL="${APPROVER_EMAIL:-approver@openclause.dev}"
APPROVER_PASSWORD="${APPROVER_PASSWORD:-approver123}"
APPROVER_NAME="${APPROVER_NAME:-Smoke Approver}"

SMOKE_RUN_ID="$(date +%s)"
APPROVER_EMAIL_UNIQUE="${APPROVER_EMAIL%@*}+${SMOKE_RUN_ID}@${APPROVER_EMAIL#*@}"

echo ">>> Starting stack (docker-compose via ./scripts/dev.sh)"
./scripts/dev.sh >/dev/null

echo ">>> Seeding dev data (tenant1, agent-1, sk-test-key-1, ...)"
./scripts/seed-dev.sh >/dev/null

echo ">>> Ensuring setup wizard initialized"
initialized="$(curl -sS "${CONSOLE_API_URL}/setup/status" | jq -r '.initialized')"
if [[ "${initialized}" != "true" ]]; then
  curl -sS "${CONSOLE_API_URL}/setup/initialize" \
    -H "Content-Type: application/json" \
    -d "$(jq -n \
      --arg org_name "${ORG_NAME}" \
      --arg email "${ADMIN_EMAIL}" \
      --arg password "${ADMIN_PASSWORD}" \
      --arg first_tenant_name "${FIRST_TENANT_NAME}" \
      '{org_name:$org_name,email:$email,password:$password,first_tenant_name:$first_tenant_name}')"
fi

echo ">>> Logging in as platform admin"
ADMIN_TOKEN="$(
  curl -sS "${CONSOLE_API_URL}/auth/login" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg email "${ADMIN_EMAIL}" --arg password "${ADMIN_PASSWORD}" '{email:$email,password:$password}')" \
  | jq -r '.token'
)"
if [[ -z "${ADMIN_TOKEN}" || "${ADMIN_TOKEN}" == "null" ]]; then
  echo "admin token missing; response was:" >&2
  curl -sS "${CONSOLE_API_URL}/auth/login" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg email "${ADMIN_EMAIL}" --arg password "${ADMIN_PASSWORD}" '{email:$email,password:$password}')" | jq . >&2
  exit 1
fi

echo ">>> Creating approver invite (role=approver, tenant=${TENANT_ID})"
INVITE_JSON="$(
  curl -sS "${CONSOLE_API_URL}/admin/invites" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$(jq -n \
      --arg email "${APPROVER_EMAIL_UNIQUE}" \
      --arg tenant_id "${TENANT_ID}" \
      --arg role "approver" \
      --arg name "${APPROVER_NAME}" \
      '{email:$email,tenant_id:$tenant_id,role:$role,name:$name}')"
)"
INVITE_TOKEN="$(echo "${INVITE_JSON}" | jq -r '.token')"
if [[ -z "${INVITE_TOKEN}" || "${INVITE_TOKEN}" == "null" ]]; then
  echo "invite token missing; invite response:" >&2
  echo "${INVITE_JSON}" >&2
  exit 1
fi

echo ">>> Accepting invite (setting known password)"
ACCEPT_RESP="$(
  curl -sS "${CONSOLE_API_URL}/auth/invite/accept" \
    -H "Content-Type: application/json" \
    -d "$(jq -n \
      --arg token "${INVITE_TOKEN}" \
      --arg password "${APPROVER_PASSWORD}" \
      --arg name "${APPROVER_NAME}" \
      '{token:$token,password:$password,name:$name}')"
)"
echo "${ACCEPT_RESP}" | jq .
ACCEPT_STATUS="$(echo "${ACCEPT_RESP}" | jq -r '.status // empty')"
if [[ "${ACCEPT_STATUS}" != "accepted" ]]; then
  echo "invite accept failed; response above" >&2
  exit 1
fi

echo ">>> Logging in as approver"
APPROVER_TOKEN="$(curl -sS "${CONSOLE_API_URL}/auth/login" \
  -H "Content-Type: application/json" \
  -d "$(jq -n --arg email "${APPROVER_EMAIL_UNIQUE}" --arg password "${APPROVER_PASSWORD}" '{email:$email,password:$password}')" \
  | jq -r '.token')"
if [[ -z "${APPROVER_TOKEN}" || "${APPROVER_TOKEN}" == "null" ]]; then
  echo "approver token missing; login response was:" >&2
  curl -sS "${CONSOLE_API_URL}/auth/login" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg email "${APPROVER_EMAIL_UNIQUE}" --arg password "${APPROVER_PASSWORD}" '{email:$email,password:$password}')" | jq . >&2
  exit 1
fi

echo ">>> Submitting high-risk tool call (expect decision=approve)"
IDEMPOTENCY_KEY="e2e-approve-${SMOKE_RUN_ID}"

TOOLCALL_JSON="$(
  curl -sS "${GATEWAY_URL}/v1/toolcalls" \
    -X POST \
    -H "X-API-Key: ${GATEWAY_API_KEY}" \
    -H "Content-Type: application/json" \
    -d "$(jq -n \
      --arg tenant_id "${TENANT_ID}" \
      --arg agent_id "${AGENT_ID}" \
      --arg tool "jira" \
      --arg action "issue.delete" \
      --arg resource "project/OPS/issue/123" \
      --argjson risk_score 8 \
      --arg idempotency_key "${IDEMPOTENCY_KEY}" \
      '{tenant_id:$tenant_id,agent_id:$agent_id,tool:$tool,action:$action,resource:$resource,risk_score:$risk_score,idempotency_key:$idempotency_key}')"
)"

DECISION="$(echo "${TOOLCALL_JSON}" | jq -r '.decision')"
EVENT_ID="$(echo "${TOOLCALL_JSON}" | jq -r '.event_id')"
echo "  toolcall decision=${DECISION} event_id=${EVENT_ID}"

if [[ "${DECISION}" != "approve" ]]; then
  echo "unexpected decision: ${DECISION}" >&2
  echo "${TOOLCALL_JSON}" >&2
  exit 1
fi
test -n "${EVENT_ID}" || { echo "event_id missing" >&2; echo "${TOOLCALL_JSON}"; exit 1; }

echo ">>> Waiting for approval in console-api queue"
APPROVAL_ID=""
for _ in $(seq 1 30); do
  APPROVALS="$(curl -sS "${CONSOLE_API_URL}/admin/approvals/pending" \
    -H "Authorization: Bearer ${APPROVER_TOKEN}")"

  # Find the approval request matching the event_id we just created.
  APPROVAL_ID="$(echo "${APPROVALS}" | jq -r --arg event_id "${EVENT_ID}" '.[] | select(.event_id==$event_id) | .id' | head -n1 || true)"

  if [[ -n "${APPROVAL_ID}" && "${APPROVAL_ID}" != "null" ]]; then
    break
  fi
  sleep 1
done

test -n "${APPROVAL_ID}" || { echo "approval not found for event_id=${EVENT_ID}" >&2; echo "${APPROVALS}" >&2; exit 1; }
echo "  approval_id=${APPROVAL_ID}"

echo ">>> Approving request"
curl -sS "${CONSOLE_API_URL}/admin/approvals/${APPROVAL_ID}/approve" \
  -X POST \
  -H "Authorization: Bearer ${APPROVER_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{}' >/dev/null

echo ">>> Executing approved request via gateway (retry on 409)"
EXEC_SUCCESS="false"
for _ in $(seq 1 30); do
  # We want both body and status code; use curl to separate them.
  resp_file="$(mktemp)"
  code="$(
    curl -sS -o "${resp_file}" -w "%{http_code}" \
      "${GATEWAY_URL}/v1/toolcalls/${EVENT_ID}/execute" \
      -X POST \
      -H "X-API-Key: ${GATEWAY_API_KEY}" \
      -H "Content-Type: application/json" \
      -d '{}' \
      || true
  )"

  body="$(cat "${resp_file}")"
  rm -f "${resp_file}"

  if [[ "${code}" == "200" ]]; then
    # Basic shape check
    if echo "${body}" | jq -e '.result and .result.status' >/dev/null 2>&1; then
      EXEC_SUCCESS="true"
      echo "  execution ok"
      break
    fi
  fi

  # If approval isn't ready yet, gateway returns 409 awaiting approval.
  if [[ "${code}" != "409" ]]; then
    echo "unexpected execute status=${code}" >&2
    echo "${body}" >&2
    exit 1
  fi

  sleep 1
done

if [[ "${EXEC_SUCCESS}" != "true" ]]; then
  echo "execution did not succeed within timeout" >&2
  exit 1
fi

echo ">>> Verifying audit trail entry is retrievable"
EVENT_DETAIL="$(curl -sS "${CONSOLE_API_URL}/admin/events/${EVENT_ID}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}")"
echo "${EVENT_DETAIL}" | jq -e --arg event_id "${EVENT_ID}" '.event_id==$event_id' >/dev/null

echo ">>> Verifying exports endpoints work (CSV + Bundle)"
SINCE="$(date -u -Iseconds | sed 's/+00:00/Z/')"
UNTIL="$(date -u -Iseconds | sed 's/+00:00/Z/')"

CSV_BODY="$(
  curl -sS "${CONSOLE_API_URL}/admin/events/export/csv?tenant_id=${TENANT_ID}&since=${SINCE}&until=${UNTIL}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}"
)"
echo "${CSV_BODY}" | head -n 1 | grep -q "event_id" || { echo "CSV export header missing" >&2; exit 1; }

bundle="$(curl -sS "${CONSOLE_API_URL}/admin/reports/export/bundle?tenant_id=${TENANT_ID}&since=${SINCE}&until=${UNTIL}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}")"
echo "${bundle}" | jq -e --arg tenant_id "${TENANT_ID}" '.tenant_id==$tenant_id and .event_count != null' >/dev/null

echo ">>> Happy-path smoke test completed successfully"
echo "    Admin UI: ${CONSOLE_UI_URL}"
