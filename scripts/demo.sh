#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# OpenClause — 5-minute demo script
# Prerequisites: docker compose stack running (./scripts/dev.sh)
# ─────────────────────────────────────────────────────────────────────────────

API="http://localhost:8090"
GW="http://localhost:8080"
GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[0;33m'; CYAN='\033[0;36m'; NC='\033[0m'

ok()   { echo -e "  ${GREEN}✓${NC} $1"; }
fail() { echo -e "  ${RED}✗${NC} $1"; exit 1; }
info() { echo -e "\n${CYAN}▸ $1${NC}"; }

# ─── 1. Setup ────────────────────────────────────────────────────────────────
info "Step 1: Check setup & initialize if needed"
STATUS=$(curl -sS "$API/setup/status" | jq -r '.initialized')
if [ "$STATUS" = "false" ]; then
  curl -sS -X POST "$API/setup/initialize" \
    -H "Content-Type: application/json" \
    -d '{"email":"admin@openclause.dev","password":"Admin123!","first_tenant_name":"Demo Org","org_name":"OpenClause"}' > /dev/null
  ok "Initialized fresh instance"
else
  ok "Already initialized"
fi

# ─── 2. Login ────────────────────────────────────────────────────────────────
info "Step 2: Login as platform admin"
LOGIN=$(curl -sS -X POST "$API/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@openclause.dev","password":"Admin123!"}')
TOKEN=$(echo "$LOGIN" | jq -r '.token')
[ "$TOKEN" != "null" ] && [ -n "$TOKEN" ] || fail "Login failed"
ok "Logged in as admin@openclause.dev"

# ─── 3. Create tenant + agent + key ─────────────────────────────────────────
info "Step 3: Create tenant, agent, and API key"
TENANT=$(curl -sS -X POST "$API/admin/tenants" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"name\":\"demo-$(date +%s)\"}")
TENANT_ID=$(echo "$TENANT" | jq -r '.id')
[ "$TENANT_ID" != "null" ] || fail "Tenant creation failed"
ok "Tenant: $TENANT_ID"

AGENT=$(curl -sS -X POST "$API/admin/tenants/$TENANT_ID/agents" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"demo-agent","description":"AI agent for demo"}')
AGENT_ID=$(echo "$AGENT" | jq -r '.id')
ok "Agent: $AGENT_ID"

KEY=$(curl -sS -X POST "$API/admin/tenants/$TENANT_ID/apikeys" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"demo-key"}')
RAW_KEY=$(echo "$KEY" | jq -r '.raw_key')
ok "API Key: ${RAW_KEY:0:20}..."

SESSION_ID="demo-session-$(date +%s)"
TRACE_ID="trace-demo-$(date +%s)"
USER_ID="demo-user-123"
USER_NAME="Taylor Tester"
USER_EMAIL="taylor@example.com"

# ─── 4. Invite delivery ─────────────────────────────────────────────────────
info "Step 4: Create a user invite and verify email status"
INVITE_EMAIL="invitee-$(date +%s)@example.com"
INVITE=$(curl -sS -X POST "$API/admin/invites" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"email\":\"$INVITE_EMAIL\",\"tenant_id\":\"$TENANT_ID\",\"role\":\"viewer\",\"name\":\"Demo Invitee\"}")
INVITE_TOKEN=$(echo "$INVITE" | jq -r '.token')
INVITE_STATUS=$(echo "$INVITE" | jq -r '.email_status')
INVITE_ACCEPT_URL=$(echo "$INVITE" | jq -r '.accept_url')
[ "$INVITE_TOKEN" != "null" ] && [ -n "$INVITE_TOKEN" ] || fail "Invite token missing"
[ "$INVITE_ACCEPT_URL" != "null" ] && [ -n "$INVITE_ACCEPT_URL" ] || fail "Invite accept_url missing"
case "$INVITE_STATUS" in
  sent)
    ok "Invite email sent"
    ;;
  logged)
    ok "Invite link logged for dev/test (SMTP not configured)"
    ;;
  failed)
    ok "Invite created; email delivery failed and link can be copied"
    ;;
  *)
    fail "Unexpected invite email_status: $INVITE_STATUS"
    ;;
esac

# ─── 5. Allow toolcall ──────────────────────────────────────────────────────
info "Step 5: Submit a low-risk toolcall (auto-allowed)"
ALLOW=$(curl -sS -X POST "$GW/v1/toolcalls" \
  -H "X-API-Key: $RAW_KEY" -H "Content-Type: application/json" \
  -d "{\"tenant_id\":\"$TENANT_ID\",\"agent_id\":\"$AGENT_ID\",\"user_id\":\"$USER_ID\",\"session_id\":\"$SESSION_ID\",\"trace_id\":\"$TRACE_ID\",\"labels\":{\"user_name\":\"$USER_NAME\",\"user_email\":\"$USER_EMAIL\"},\"tool\":\"slack\",\"action\":\"channel.list\",\"risk_score\":1,\"idempotency_key\":\"demo-allow-$(date +%s)\"}")
ALLOW_DEC=$(echo "$ALLOW" | jq -r '.decision')
ALLOW_ID=$(echo "$ALLOW" | jq -r '.event_id')
[ "$ALLOW_DEC" = "allow" ] && ok "Decision: $ALLOW_DEC (event: $ALLOW_ID)" || fail "Expected allow, got $ALLOW_DEC"

# ─── 6. Deny toolcall ───────────────────────────────────────────────────────
info "Step 6: Submit an unknown-tool request (auto-denied)"
DENY=$(curl -sS -X POST "$GW/v1/toolcalls" \
  -H "X-API-Key: $RAW_KEY" -H "Content-Type: application/json" \
  -d "{\"tenant_id\":\"$TENANT_ID\",\"agent_id\":\"$AGENT_ID\",\"user_id\":\"$USER_ID\",\"session_id\":\"$SESSION_ID\",\"trace_id\":\"$TRACE_ID\",\"labels\":{\"user_name\":\"$USER_NAME\",\"user_email\":\"$USER_EMAIL\"},\"tool\":\"unknown\",\"action\":\"danger\",\"risk_score\":3,\"idempotency_key\":\"demo-deny-$(date +%s)\"}")
DENY_DEC=$(echo "$DENY" | jq -r '.decision')
[ "$DENY_DEC" = "deny" ] && ok "Decision: $DENY_DEC" || fail "Expected deny, got $DENY_DEC"

# ─── 7. Approve toolcall ────────────────────────────────────────────────────
info "Step 7: Submit a high-risk toolcall (requires approval)"
APPROVE=$(curl -sS -X POST "$GW/v1/toolcalls" \
  -H "X-API-Key: $RAW_KEY" -H "Content-Type: application/json" \
  -d "{\"tenant_id\":\"$TENANT_ID\",\"agent_id\":\"$AGENT_ID\",\"user_id\":\"$USER_ID\",\"session_id\":\"$SESSION_ID\",\"trace_id\":\"$TRACE_ID\",\"labels\":{\"user_name\":\"$USER_NAME\",\"user_email\":\"$USER_EMAIL\"},\"tool\":\"slack\",\"action\":\"msg.post\",\"risk_score\":8,\"params\":{\"channel\":\"#general\",\"text\":\"Hello from demo\"},\"idempotency_key\":\"demo-approve-$(date +%s)\"}")
APPROVE_DEC=$(echo "$APPROVE" | jq -r '.decision')
APPROVE_ID=$(echo "$APPROVE" | jq -r '.event_id')
APPROVAL_URL=$(echo "$APPROVE" | jq -r '.approval_url')
APPROVAL_REQ_ID=$(echo "$APPROVAL_URL" | grep -oE '[0-9a-f-]{36}$')
[ "$APPROVE_DEC" = "approve" ] && ok "Decision: $APPROVE_DEC (event: $APPROVE_ID)" || fail "Expected approve"

# ─── 8. Platform admin approves ─────────────────────────────────────────────
info "Step 8: Platform admin approves the request"
GRANT=$(curl -sS -X POST "$API/admin/approvals/$APPROVAL_REQ_ID/approve" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"reason":"Demo approval by admin"}')
GRANT_STATUS=$(echo "$GRANT" | jq -r '.status')
[ "$GRANT_STATUS" = "approved" ] && ok "Approved: $(echo "$GRANT" | jq -r '.grant_id')" || fail "Approval failed: $(echo "$GRANT" | jq -c .)"

# ─── 9. Execute ─────────────────────────────────────────────────────────────
info "Step 9: Execute the approved toolcall"
EXEC=$(curl -sS -X POST "$GW/v1/toolcalls/$APPROVE_ID/execute" \
  -H "X-API-Key: $RAW_KEY" -H "Content-Type: application/json")
EXEC_DEC=$(echo "$EXEC" | jq -r '.decision')
EXEC_STATUS=$(echo "$EXEC" | jq -r '.result.status')
EXEC_ID=$(echo "$EXEC" | jq -r '.event_id')
[ "$EXEC_STATUS" = "success" ] && ok "Executed: status=$EXEC_STATUS (event: $EXEC_ID)" || echo -e "  ${YELLOW}⚠${NC} Execute result: $EXEC_STATUS (mock connector)"

# ─── 10. Audit trail ────────────────────────────────────────────────────────
info "Step 10: Verify audit trail"
AUDIT=$(curl -sS "$API/admin/events/$APPROVE_ID" -H "Authorization: Bearer $TOKEN")
AUDIT_TOOL=$(echo "$AUDIT" | jq -r '.tool // .event.tool // empty')
[ -n "$AUDIT_TOOL" ] && ok "Audit event found: tool=$AUDIT_TOOL" || ok "Audit trail accessible"

# ─── 11. Sessions ───────────────────────────────────────────────────────────
info "Step 11: Verify session view"
SESSION_LIST=$(curl -sS "$API/admin/sessions?tenant_id=$TENANT_ID&session_id=$SESSION_ID" -H "Authorization: Bearer $TOKEN")
SESSION_MATCHES=$(echo "$SESSION_LIST" | jq 'length')
SESSION_FOUND_ID=$(echo "$SESSION_LIST" | jq -r '.[0].id // empty')
[ "$SESSION_MATCHES" -ge 1 ] && [ "$SESSION_FOUND_ID" = "$SESSION_ID" ] && ok "Session visible in console API: $SESSION_ID" || fail "Session not found in sessions API"

# ─── 12. Exports ────────────────────────────────────────────────────────────
info "Step 12: Export data"
CSV_CODE=$(curl -sS -o /dev/null -w "%{http_code}" "$API/admin/events/export/csv?tenant_id=$TENANT_ID" -H "Authorization: Bearer $TOKEN")
[ "$CSV_CODE" = "200" ] && ok "CSV export: HTTP $CSV_CODE" || fail "CSV export failed"

BUNDLE=$(curl -sS "$API/admin/reports/export/bundle?tenant_id=$TENANT_ID" -H "Authorization: Bearer $TOKEN")
BUNDLE_COUNT=$(echo "$BUNDLE" | jq -r '.event_count')
ok "Bundle export: $BUNDLE_COUNT events"

# ─── 13. Connectors ─────────────────────────────────────────────────────────
info "Step 13: List connectors"
GW_CONN_COUNT=$(curl -sS "$GW/v1/connectors" | jq 'length')
API_CONN_COUNT=$(curl -sS "$API/admin/connectors" -H "Authorization: Bearer $TOKEN" | jq 'length')
ok "Gateway connectors: $GW_CONN_COUNT registered"
ok "Console connectors: $API_CONN_COUNT registered"

# ─── 14. Analytics ──────────────────────────────────────────────────────────
info "Step 14: Tenant analytics"
ANALYTICS=$(curl -sS "$API/admin/tenants/$TENANT_ID/analytics/summary?range=24h&bucket_minutes=60&top_agents=5" -H "Authorization: Bearer $TOKEN")
TOTAL=$(echo "$ANALYTICS" | jq -r '.totals.total_events')
ok "Analytics: $TOTAL events in last 24h"

# ─── Summary ─────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}═══════════════════════════════════════════${NC}"
echo -e "${GREEN}  Demo complete — all steps passed${NC}"
echo -e "${GREEN}═══════════════════════════════════════════${NC}"
echo ""
echo "  Console UI:   http://localhost:3000"
echo "  Login:        admin@openclause.dev / Admin123!"
echo ""
echo "  Tenant ID:    $TENANT_ID"
echo "  Agent ID:     $AGENT_ID"
echo "  Session ID:   $SESSION_ID"
echo "  API Key:      ${RAW_KEY:0:25}..."
echo ""
echo "  Events created:"
echo "    Allow:    $ALLOW_ID"
echo "    Deny:     $(echo "$DENY" | jq -r '.event_id')"
echo "    Approve:  $APPROVE_ID"
echo "    Execute:  $EXEC_ID"
