# Local Testing Guide

End-to-end walkthrough: boot the stack, seed data, and exercise every major flow.

Use [readme.md](/Users/bogdan/dev/personal/OpenClause/readme.md) as the canonical quick-start and product overview. This guide is the deeper local/operator reference for curl recipes, smoke checks, and environment-specific notes.

## Prerequisites

- Docker Desktop running
- `curl` (or any HTTP client)
- Go 1.25+ (for `go test`)
- Java 11+ (for `sdk/java/gradlew test`)
- Python 3.10+ with recent `pip`/`setuptools`/`wheel` if you want to validate `sdk/python` editable installs

## 1. Start the Stack

```bash
# From repo root (macOS/Linux)
./scripts/dev.sh

# On Windows (PowerShell)
./scripts/dev.ps1
```

Verify health:

```bash
curl http://localhost:8080/healthz   # Gateway
curl http://localhost:8081/healthz   # Approvals
curl http://localhost:8090/healthz   # Console API
curl http://localhost:8181/health    # OPA
```

## 2. Seed Test Data

First-run setup (recommended) creates the initial platform admin + first tenant.

After that, you can seed optional dev data (agents, API keys, sessions) as needed.

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres psql -U openclause -d openclause << 'SQL'

-- Tenant
INSERT INTO tenants (id, name, status, config) VALUES
  ('tenant1', 'Acme Corp', 'active', '{"max_risk_auto_approve": 5}')
ON CONFLICT (id) DO NOTHING;

-- Agent
INSERT INTO agents (id, tenant_id, name, status) VALUES
  ('agent-1', 'tenant1', 'Test Agent', 'active')
ON CONFLICT (id) DO NOTHING;

-- API key: sk-test-key-1 (SHA-256 of the raw key)
INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, status) VALUES
  ('key-1', 'tenant1', 'dev-key',
   'sk-test-',
   'c1fa602237f88a7c84dc1cff004a4f10f0e85127b2a3461aa33aea6694808262',
   'active')
ON CONFLICT (id) DO NOTHING;

SQL
```

> **Hashes above**: API key hash is `SHA-256("sk-test-key-1")`.
>
> IMPORTANT: `API_KEYS` must map to the *actual* `tenant_id` values present in your database.
> If you used the setup wizard, your tenant id is likely a UUID (not `tenant1`), so update
> `API_KEYS` accordingly.

## Token handling note (dev)

- Invite + password reset tokens are stored in the database as keyed HMAC hashes (not plaintext).
- `POST /admin/invites` still returns the raw `token` once, plus an absolute `accept_url` and `email_status`.
- `GET /admin/invites` does not return the raw token after creation; it only shows pending invite metadata + email delivery status.
- Console-api may log raw invite/reset token URLs in development. Control this with `CONSOLE_DEV_LOG_RAW_TOKENS`:
  - `true` (default): log raw token URLs for easier local testing
  - `false`: suppress raw token URLs in logs
- If `SMTP_HOST` / `SMTP_FROM` are unset, invite delivery uses the dev/test logging sender and `email_status` is `logged`.

On Windows PowerShell, pipe the SQL through docker directly:

```powershell
Get-Content docs\seed_dev.sql | docker compose -f deploy/docker-compose.yml exec -T postgres psql -U openclause -d openclause
```

## 3. Console Login

```bash
curl -s http://localhost:8090/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"<platform-admin-email>","password":"<platform-admin-password>"}'
```

```powershell
$body = @{ email = "<platform-admin-email>"; password = "<platform-admin-password>" } | ConvertTo-Json
$token = (Invoke-RestMethod -Method Post -Uri "http://localhost:8090/auth/login" -ContentType "application/json" -Body $body).token
```

The login response includes both `token` and `session_id`. Save the `token` from the response:

```bash
export TOKEN="<paste token here>"
```

Determine your `TENANT_ID`:

- This guide's later curl examples assume the dev seed data above is present (tenant `tenant1`, agent `agent-1`, API key raw value `sk-test-key-1`).
- If you used the SQL snippet above, set `TENANT_ID=tenant1`.
- If you only used the Setup Wizard and did not run the seed SQL, you must create an agent + API key for your actual tenant and update `agent_id` / `X-API-Key` values in the commands below (they currently assume `agent-1` + `sk-test-key-1`).
```bash
curl -s "http://localhost:8090/admin/tenants" \
  -H "Authorization: Bearer $TOKEN"
```

Set:
```bash
export TENANT_ID="tenant1" # if you ran the seed SQL above; otherwise replace with your tenant id
```

## 4. Create a Tenant via Console (optional)

```bash
curl -s http://localhost:8090/admin/tenants \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"New Tenant"}'
```

## 5. Tool-Call: ALLOW Path (low risk read)

```bash
curl -s http://localhost:8080/v1/toolcalls \
  -H "X-API-Key: sk-test-key-1" \
  -H "Content-Type: application/json" \
  -d "{
    \"tenant_id\": \"$TENANT_ID\",
    \"agent_id\": \"agent-1\",
    \"user_id\": \"user-123\",
    \"session_id\": \"run-local-001\",
    \"trace_id\": \"trace-local-001\",
    \"labels\": {\"user_name\": \"Avery Analyst\", \"user_email\": \"avery@example.com\"},
    \"tool\": \"slack\",
    \"action\": \"channel.list\",
    \"risk_score\": 1,
    \"idempotency_key\": \"test-allow-1\"
  }"
```

Expected: `"decision": "allow"` with a `result` object.

## 6. Tool-Call: DENY Path (unlisted action)

```bash
curl -s http://localhost:8080/v1/toolcalls \
  -H "X-API-Key: sk-test-key-1" \
  -H "Content-Type: application/json" \
  -d "{
    \"tenant_id\": \"$TENANT_ID\",
    \"agent_id\": \"agent-1\",
    \"user_id\": \"user-123\",
    \"session_id\": \"run-local-001\",
    \"trace_id\": \"trace-local-001\",
    \"labels\": {\"user_name\": \"Avery Analyst\", \"user_email\": \"avery@example.com\"},
    \"tool\": \"postgres\",
    \"action\": \"query.readonly\",
    \"params\": { \"sql\": \"SELECT 1\", \"params\": [] },
    \"risk_score\": 3,
    \"idempotency_key\": \"test-deny-1\"
  }"
```

Expected: `"decision": "deny"`, reason `"action not in allowlist"`.

## 7. Tool-Call: APPROVE Path (high risk)

```bash
curl -s http://localhost:8080/v1/toolcalls \
  -H "X-API-Key: sk-test-key-1" \
  -H "Content-Type: application/json" \
  -d "{
    \"tenant_id\": \"$TENANT_ID\",
    \"agent_id\": \"agent-1\",
    \"user_id\": \"user-123\",
    \"session_id\": \"run-local-001\",
    \"trace_id\": \"trace-local-001\",
    \"labels\": {\"user_name\": \"Avery Analyst\", \"user_email\": \"avery@example.com\"},
    \"tool\": \"jira\",
    \"action\": \"issue.create\",
    \"params\": { \"project\": \"OPS\", \"summary\": \"Test issue from local testing\" },
    \"resource\": \"project/OPS\",
    \"risk_score\": 8,
    \"idempotency_key\": \"test-approve-1\"
  }"
```

Expected: `"decision": "approve"` with an `approval_url`.

## 7a. (Optional) Update Notification Routing (Tier 2 item 7)

To change where approval notifications are delivered for your tenant (Slack + HTTPS webhooks), update the per-tenant routing config as `tenant_admin`:

1. Log in as `tenant_admin` and set:
```bash
export TENANT_ADMIN_TOKEN="<paste tenant_admin JWT>"
```

2. Update routing (example: Slack channel):
```bash
curl -s -X PUT "http://localhost:8090/admin/tenants/$TENANT_ID/notification-config" \
  -H "Authorization: Bearer $TENANT_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "approver_group": "tenant_admin",
    "notify": [
      { "kind": "slack", "channel": "#team-alerts" }
    ]
  }'
```

3. (Optional) Verify it:
```bash
curl -s "http://localhost:8090/admin/tenants/$TENANT_ID/notification-config" \
  -H "Authorization: Bearer $TENANT_ADMIN_TOKEN"
```

Webhook URLs are validated server-side (`https` only; private/loopback IPs rejected) to prevent SSRF.

Save the `event_id`:

```bash
export EVENT_ID="<paste event_id>"
```

## 7b. Create deny_spike Alert Rule (Tier 2 item 8)

OpenClause evaluates alert rules in the background via `cmd/alert-worker` (poll interval controlled by `ALERT_WORKER_INTERVAL_SEC`, default `30s`).

### 1) Ensure notification routing exists for alert delivery

Alert notifications reuse the same per-tenant notification config as approval notifications (`tenants.config.notification_config`).

Update routing for the target tenant as `tenant_admin` (or `platform_admin`):
```bash
curl -s -X PUT "http://localhost:8090/admin/tenants/$TENANT_ID/notification-config" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "approver_group": "",
    "notify": [
      { "kind": "slack", "channel": "#team-alerts" }
    ]
  }'
```

### 2) Create a deny_spike rule

```bash
curl -s -X POST "http://localhost:8090/admin/tenants/$TENANT_ID/alerts/rules" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "deny-spike-smoke",
    "kind": "deny_spike",
    "enabled": true,
    "config_json": { "n": 3, "m_minutes": 5 }
  }' | jq .
```

### 3) Generate 3 deny tool-calls quickly

Important: for alert firing, the tenant id on the *alert rule* must match the tenant id written to `tool_events`.
- In the default dev setup, `TENANT_ID` is `tenant1`.
- If you used the Setup Wizard only, your tenant id may be a UUID; ensure `$TENANT_ID` matches the tenant id for the API key you use (e.g. `sk-test-key-1`).

These tool-call parameters are the default “deny” example used by the local policy tests (slack `msg.update`):
```bash
for i in 1 2 3; do
  curl -s -X POST "http://localhost:8080/v1/toolcalls" \
    -H "X-API-Key: sk-test-key-1" \
    -H "Content-Type: application/json" \
    -d "{
      \"tenant_id\": \"$TENANT_ID\",
      \"agent_id\": \"agent-1\",
      \"tool\": \"slack\",
      \"action\": \"msg.update\",
      \"risk_score\": 1,
      \"idempotency_key\": \"deny-spike-smoke-$i\"
    }" | jq -r '.decision + " " + .event_id'
done
```

Expected: each call returns `"decision": "deny"` with an `event_id`.

### 4) Verify the worker emitted an alert event

Wait for the worker (default 30s), then list alert events for the tenant:
```bash
sleep 45
curl -s "http://localhost:8090/admin/tenants/$TENANT_ID/alerts/events?limit=10" \
  -H "Authorization: Bearer $TOKEN" | jq .
```

Expected: at least one alert event with:
- `status` = `sent` (or `pending` if the sink temporarily fails)
- message containing `"deny spike: 3 denies"`
- if `status` is `pending`, the payload includes `attempt_count`, `last_error`, and `next_attempt_at` so the UI can show retry timing

## 8. Approve the Request

First, list pending approvals to get the approval request ID:

```bash
curl -s "http://localhost:8081/v1/approvals/pending?tenant_id=$TENANT_ID" \
  -H "X-Internal-Token: dev-internal-token-change-me"
```

Save the `id` of the pending request:

```bash
export APPROVAL_ID="<paste id>"
```

Approve it (you need the approver assigned as `role='approver'` for your `$TENANT_ID` in the Console UI):

```bash
curl -s "http://localhost:8081/v1/approvals/requests/$APPROVAL_ID/approve" \
  -H "X-Internal-Token: dev-internal-token-change-me" \
  -H "Content-Type: application/json" \
  -d '{"approver":"<platform-admin-email>","max_uses":1}'
```

> If you get `403 approver is not allowed for tenant`, confirm the approver user exists in the Console and has the `approver` role assigned for `$TENANT_ID`.

## 9. Execute the Approved Tool-Call

```bash
curl -s "http://localhost:8080/v1/toolcalls/$EVENT_ID/execute" \
  -X POST \
  -H "X-API-Key: sk-test-key-1"
```

Expected: `"decision": "allow"`, `"reason": "approved execution"`, with a `result`.

Alternatively, use the Console UI:
- Open the Approvals queue and select the approved request.
- Use the modal's **Copy execute command** helper and paste your tenant-scoped API key.

Call it again to verify idempotent replay returns the same `event_id` and prior execution `result`.

## 10. View Audit Trail via Console API

```bash
# List events
curl -s "http://localhost:8090/admin/events?tenant_id=$TENANT_ID" \
  -H "Authorization: Bearer $TOKEN"

# Get event detail
curl -s "http://localhost:8090/admin/events/$EVENT_ID" \
  -H "Authorization: Bearer $TOKEN"

# List observed sessions (derived from tool_events.session_id)
curl -s "http://localhost:8090/admin/sessions?tenant_id=$TENANT_ID" \
  -H "Authorization: Bearer $TOKEN"

# Narrow to one run by session_id
curl -s "http://localhost:8090/admin/sessions?tenant_id=$TENANT_ID&session_id=run-local-001" \
  -H "Authorization: Bearer $TOKEN"

# Inspect one session in detail
SESSION_ID="run-local-001"
curl -s "http://localhost:8090/admin/sessions/$SESSION_ID?tenant_id=$TENANT_ID" \
  -H "Authorization: Bearer $TOKEN"

# Platform-admin ambiguity recovery: omit tenant_id only if you know the session
# exists in one tenant. If not, the API returns 400 with `candidates` so the UI
# can prompt you to choose the right tenant.

# View the session timeline with approval/execution context
curl -s "http://localhost:8090/admin/sessions/$SESSION_ID/timeline?tenant_id=$TENANT_ID" \
  -H "Authorization: Bearer $TOKEN"

# Export the session as CSV or JSON (`404` if the session id or tenant scope is wrong)
curl -s "http://localhost:8090/admin/sessions/$SESSION_ID/export/csv?tenant_id=$TENANT_ID" \
  -H "Authorization: Bearer $TOKEN"

curl -s "http://localhost:8090/admin/sessions/$SESSION_ID/export/json?tenant_id=$TENANT_ID" \
  -H "Authorization: Bearer $TOKEN"

# List active console login sessions
curl -s "http://localhost:8090/admin/auth-sessions" \
  -H "Authorization: Bearer $TOKEN"

# Revoke an active console login session
SESSION_ID="<session_id from /admin/auth-sessions>"
curl -s -X POST "http://localhost:8090/admin/auth-sessions/$SESSION_ID/revoke" \
  -H "Authorization: Bearer $TOKEN"

# Log out the current console session
curl -s -X POST "http://localhost:8090/auth/logout" \
  -H "Authorization: Bearer $TOKEN"
```

## 11. Rate Limiting

Send 101+ rapid requests to trigger the rate limiter:

```bash
for i in $(seq 1 105); do
  curl -s -o /dev/null -w "%{http_code}\n" \
    http://localhost:8080/v1/toolcalls \
    -H "X-API-Key: sk-test-key-1" \
    -H "Content-Type: application/json" \
    -d "{\"tenant_id\":\"$TENANT_ID\",\"agent_id\":\"agent-1\",\"tool\":\"slack\",\"action\":\"channel.list\",\"risk_score\":1,\"idempotency_key\":\"rate-$i\"}"
done
```

You should see `429` responses with a `Retry-After: 1` header once the limit is hit.

## 12. Connectors List

```bash
curl -s http://localhost:8090/admin/connectors \
  -H "Authorization: Bearer $TOKEN"
```

Verify that all 8 connectors are listed even before any toolcalls exist, and that no `base_url` fields are present.

## 13. Disabled Tenant

```bash
# Disable tenant via console
curl -s "http://localhost:8090/admin/tenants/$TENANT_ID/status" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status":"disabled"}'

# Try using the API key — should get 403 "tenant disabled"
curl -s http://localhost:8080/v1/toolcalls \
  -H "X-API-Key: sk-test-key-1" \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"'$TENANT_ID'","agent_id":"agent-1","tool":"slack","action":"channel.list","risk_score":1,"idempotency_key":"disabled-test"}'

# Re-enable
curl -s "http://localhost:8090/admin/tenants/$TENANT_ID/status" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status":"active"}'
```

> Tenant status is enforced for **all** API keys (both env-based and DB-backed).
> The auth middleware checks tenant status in the database after key validation.

## 14. Policy Simulation

```bash
curl -s http://localhost:8090/admin/policy/simulate \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"tenant_id\": \"$TENANT_ID\",
    \"agent_id\": \"agent-1\",
    \"tool\": \"jira\",
    \"action\": \"issue.delete\",
    \"risk_score\": 9
  }"
```

Expected: `policy_result.result.decision` = `"approve"` (destructive action + high risk).

## 14a. Tenant Analytics (Tier 3 item 9)

After generating some allow/deny/approve tool-call events for your tenant, you can verify analytics via the tenant-scoped summary endpoint:

```bash
curl -s "http://localhost:8090/admin/tenants/$TENANT_ID/analytics/summary?range=24h&bucket_minutes=60&top_agents=5" \
  -H "Authorization: Bearer $TOKEN" | jq .
```

Expected JSON fields:
- `totals` (total_events, allow_count, deny_count, approve_count)
- `trend` (time buckets with allow/deny/approve counts)
- `risk_heatmap` (risk_score 0..10 with decision counts)
- `per_agent` (top agents by total event count)
- `onboarding_checklist` (has_api_key/has_approver lifetime; has_toolcall/has_approval/has_execution within the selected range)

## 14b. API Key Rotation + Metadata (Tier 3 item 10)

Rotate key for a tenant (create new key, make it primary, revoke old primary):

```bash
curl -s -X POST "http://localhost:8090/admin/tenants/$TENANT_ID/apikeys/rotate" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "rotated-local-test",
    "expires_at": "2030-01-01",
    "make_primary": true,
    "revoke_old_primary": true
  }'
```

Expected:
- response includes `raw_key` exactly once
- new key has `is_primary=true`
- old primary key is revoked when `revoke_old_primary=true`

Verify metadata:

```bash
curl -s "http://localhost:8090/admin/tenants/$TENANT_ID/apikeys" \
  -H "Authorization: Bearer $TOKEN" | jq .
```

Look for:
- `last_used_at` updates after requests with that key
- `expires_at` set when provided
- `is_primary` flag on the active primary key

## 14c. Policy Authoring UX (Tier 3 item 11)

Use tenant-scoped policy rule-builder APIs to verify preview/save/rollback behavior.

Create a baseline config snapshot:

```bash
BASE_CFG='{
  "max_risk_auto_approve": 7,
  "read_actions": ["jira.issue.list","slack.channel.list"],
  "write_actions": ["jira.issue.create","slack.msg.post"],
  "destructive_actions": ["jira.issue.delete"],
  "require_destructive_approval": true
}'

curl -s -X PUT "http://localhost:8090/admin/tenants/$TENANT_ID/policy/config" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "$BASE_CFG" | jq .

BEFORE_VERSION_ID=$(
  curl -s -X POST "http://localhost:8090/admin/tenants/$TENANT_ID/policy/versions" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg v "baseline-$(date -u +%Y%m%d%H%M%S)" --argjson pdata "$BASE_CFG" '{version:$v,notes:"baseline",policy_data:$pdata}')" \
  | jq -r '.id'
)
```

Apply a stricter config and preview:

```bash
TIGHT_CFG='{
  "max_risk_auto_approve": 2,
  "read_actions": ["jira.issue.list","slack.channel.list"],
  "write_actions": ["jira.issue.create","slack.msg.post"],
  "destructive_actions": ["jira.issue.delete"],
  "require_destructive_approval": true
}'

curl -s -X PUT "http://localhost:8090/admin/tenants/$TENANT_ID/policy/config" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "$TIGHT_CFG" | jq .

curl -s -X POST "http://localhost:8090/admin/tenants/$TENANT_ID/policy/simulate" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"agent_id":"agent-1","tool":"jira","action":"issue.create","resource":"project/OPS","risk_score":6}' \
  | jq '.policy_result.result'
```

Expected under this strict config: simulation and gateway decision should be `deny` for `jira.issue.create` with `risk_score=6`.

Rollback and verify behavior is restored:

```bash
curl -s -X POST "http://localhost:8090/admin/tenants/$TENANT_ID/policy/versions/$BEFORE_VERSION_ID/rollback" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}' | jq .
```

Expected after rollback: the same toolcall can return `allow` again when it is below the restored threshold and in allowlists.

## 14d. Helm Charts for Console Services (Tier 3 item 12)

Render Helm templates for the new console charts:

```bash
# If local helm is installed:
helm template oc-console-api ./deploy/helm/console-api | head
helm template oc-console-ui ./deploy/helm/console-ui | head
```

If `helm` is not installed locally, use a containerized Helm binary:

```bash
docker run --rm -v "$PWD":/work -w /work alpine/helm:3.16.3 template oc-console-api ./deploy/helm/console-api > /tmp/oc-console-api.yaml
docker run --rm -v "$PWD":/work -w /work alpine/helm:3.16.3 template oc-console-ui ./deploy/helm/console-ui > /tmp/oc-console-ui.yaml

wc -l /tmp/oc-console-api.yaml /tmp/oc-console-ui.yaml
```

Expected:
- both `helm template` commands exit successfully
- rendered YAML includes `Deployment`, `Service`, and optional `Ingress` manifests for each chart

## 15. Run Unit Tests

```bash
go test ./... -count=1          # all tests
go test -race ./... -count=1    # with race detector
```

## 16. Tear Down

```bash
make dev-down
# or
docker compose -f deploy/docker-compose.yml down -v
```

## Windows PowerShell Tips

PowerShell `curl` is an alias for `Invoke-WebRequest`. Use `curl.exe` to invoke
the real curl binary, or use `Invoke-RestMethod` natively.

**Avoid quoting pitfalls** — write JSON to a temp file instead of inline:

```powershell
@'
{"tenant_id":"<TENANT_ID>","agent_id":"agent-1","tool":"slack","action":"channel.list","risk_score":1,"idempotency_key":"test-allow-1"}
'@ | Set-Content -NoNewline toolcall.json

curl.exe -s "http://localhost:8080/v1/toolcalls" `
  -H "X-API-Key: sk-test-key-1" `
  -H "Content-Type: application/json" `
  --data-binary "@toolcall.json"
```

**Login and capture token:**

```powershell
$body = @{ email = "<platform-admin-email>"; password = "<platform-admin-password>" } | ConvertTo-Json
$resp = Invoke-RestMethod -Method Post -Uri "http://localhost:8090/auth/login" `
  -ContentType "application/json" -Body $body
$TOKEN = $resp.token
```

**Disable / re-enable tenant:**

```powershell
@'{"status":"disabled"}'@ | Set-Content -NoNewline tenant-disable.json
curl.exe -s "http://localhost:8090/admin/tenants/<TENANT_ID>/status" `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  --data-binary "@tenant-disable.json"
```

## Quick Reference: Ports

| Service | Port | Auth |
|---------|------|------|
| Gateway | 8080 | `X-API-Key` header |
| Approvals | 8081 | `X-Internal-Token` header |
| Slack connector | 8082 | `X-Internal-Token` header |
| Jira connector | 8083 | `X-Internal-Token` header |
| Console API | 8090 | `Authorization: Bearer <JWT>` |
| Console UI | 3000 | JWT via browser |
| OPA | 8181 | None |
| MinIO Console | 9001 | minioadmin / minioadmin |
| Postgres | 5432 | openclause / changeme |
