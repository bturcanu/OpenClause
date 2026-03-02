# Local Testing Guide

End-to-end walkthrough: boot the stack, seed data, and exercise every major flow.

## Prerequisites

- Docker Desktop running
- `curl` (or any HTTP client)
- Go 1.25+ (for `go test`)

## 1. Start the Stack

```bash
# From repo root (Linux/macOS)
make dev

# On Windows (PowerShell) — run commands manually:
docker compose -f deploy/docker-compose.yml up --build -d
# Wait for postgres, then run migrations:
Get-Content migrations/001_initial.sql | docker compose -f deploy/docker-compose.yml exec -T postgres `
  psql -U openclause -d openclause -v ON_ERROR_STOP=1
```

Verify health:

```bash
curl http://localhost:8080/healthz   # Gateway
curl http://localhost:8081/healthz   # Approvals
curl http://localhost:8090/healthz   # Console API
curl http://localhost:8181/health    # OPA
```

## 2. Seed Test Data

Since there is no seed migration, insert a tenant, agent, API key, and admin user manually.

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

-- Admin user (password: admin123)
INSERT INTO users (id, email, password_hash, name, status) VALUES
  ('user-admin', 'admin@openclause.dev',
   '$2a$10$3Kf3g5CCnYM1OaSq3GifI.PRWNyRu6KWEBRXwBRPeX1/ypbzLfxDu',
   'Admin', 'active')
ON CONFLICT (id) DO NOTHING;

INSERT INTO user_roles (id, user_id, tenant_id, role) VALUES
  ('role-admin', 'user-admin', NULL, 'platform_admin')
ON CONFLICT (id) DO NOTHING;

SQL
```

> **Hashes above**: API key hash is `SHA-256("sk-test-key-1")`. Password hash is `bcrypt("admin123")`.
> Your `.env` already has `API_KEYS=tenant1:sk-test-key-1` so the in-memory keystore also works.

On Windows PowerShell, pipe the SQL through docker directly:

```powershell
Get-Content docs\seed_dev.sql | docker compose -f deploy/docker-compose.yml exec -T postgres psql -U openclause -d openclause
```

## 3. Console Login

```bash
curl -s http://localhost:8090/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@openclause.dev","password":"admin123"}'
```

```powershell
$body = @{ email = "admin@openclause.dev"; password = "admin123" } | ConvertTo-Json
$token = (Invoke-RestMethod -Method Post -Uri "http://localhost:8090/auth/login" -ContentType "application/json" -Body $body).token
```

Save the `token` from the response:

```bash
export TOKEN="<paste token here>"
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
  -d '{
    "tenant_id": "tenant1",
    "agent_id": "agent-1",
    "tool": "slack",
    "action": "channel.list",
    "risk_score": 1,
    "idempotency_key": "test-allow-1"
  }'
```

Expected: `"decision": "allow"` with a `result` object.

## 6. Tool-Call: DENY Path (unlisted action)

```bash
curl -s http://localhost:8080/v1/toolcalls \
  -H "X-API-Key: sk-test-key-1" \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "tenant1",
    "agent_id": "agent-1",
    "tool": "database",
    "action": "query.run",
    "risk_score": 3,
    "idempotency_key": "test-deny-1"
  }'
```

Expected: `"decision": "deny"`, reason `"action not in allowlist"`.

## 7. Tool-Call: APPROVE Path (high risk)

```bash
curl -s http://localhost:8080/v1/toolcalls \
  -H "X-API-Key: sk-test-key-1" \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "tenant1",
    "agent_id": "agent-1",
    "tool": "jira",
    "action": "issue.delete",
    "resource": "project/OPS/issue/123",
    "risk_score": 8,
    "idempotency_key": "test-approve-1"
  }'
```

Expected: `"decision": "approve"` with an `approval_url`.

Save the `event_id`:

```bash
export EVENT_ID="<paste event_id>"
```

## 8. Approve the Request

First, list pending approvals to get the approval request ID:

```bash
curl -s "http://localhost:8081/v1/approvals/pending?tenant_id=tenant1" \
  -H "X-Internal-Token: dev-internal-token-change-me"
```

Save the `id` of the pending request:

```bash
export APPROVAL_ID="<paste id>"
```

Approve it (you need the approver in the allowlist, or clear the allowlist):

```bash
curl -s "http://localhost:8081/v1/approvals/requests/$APPROVAL_ID/approve" \
  -H "X-Internal-Token: dev-internal-token-change-me" \
  -H "Content-Type: application/json" \
  -d '{"approver":"admin@openclause.dev","max_uses":1}'
```

> If you get `403 approver is not allowed for tenant`, add the approver to your `.env`:
> `APPROVER_EMAIL_ALLOWLIST=tenant1:admin@openclause.dev`
> then restart the approvals service.

## 9. Execute the Approved Tool-Call

```bash
curl -s "http://localhost:8080/v1/toolcalls/$EVENT_ID/execute" \
  -X POST \
  -H "X-API-Key: sk-test-key-1"
```

Expected: `"decision": "allow"`, `"reason": "approved execution"`, with a `result`.

Call it again to verify idempotent replay returns the same `event_id`.

## 10. View Audit Trail via Console API

```bash
# List events
curl -s "http://localhost:8090/admin/events?tenant_id=tenant1" \
  -H "Authorization: Bearer $TOKEN"

# Get event detail
curl -s "http://localhost:8090/admin/events/$EVENT_ID" \
  -H "Authorization: Bearer $TOKEN"

# List sessions
curl -s "http://localhost:8090/admin/sessions?tenant_id=tenant1" \
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
    -d "{\"tenant_id\":\"tenant1\",\"agent_id\":\"agent-1\",\"tool\":\"slack\",\"action\":\"channel.list\",\"risk_score\":1,\"idempotency_key\":\"rate-$i\"}"
done
```

You should see `429` responses with a `Retry-After: 1` header once the limit is hit.

## 12. Connectors List

```bash
curl -s http://localhost:8080/v1/connectors \
  -H "X-API-Key: sk-test-key-1"
```

Verify that no `base_url` fields are present (internal URLs are no longer leaked).

## 13. Disabled Tenant

```bash
# Disable tenant1 via console
curl -s "http://localhost:8090/admin/tenants/tenant1/status" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status":"disabled"}'

# Try using the API key — should get 403 "tenant disabled"
curl -s http://localhost:8080/v1/toolcalls \
  -H "X-API-Key: sk-test-key-1" \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"tenant1","agent_id":"agent-1","tool":"slack","action":"channel.list","risk_score":1,"idempotency_key":"disabled-test"}'

# Re-enable
curl -s "http://localhost:8090/admin/tenants/tenant1/status" \
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
  -d '{
    "tenant_id": "tenant1",
    "agent_id": "agent-1",
    "tool": "jira",
    "action": "issue.delete",
    "risk_score": 9
  }'
```

Expected: `policy_result.result.decision` = `"approve"` (destructive action + high risk).

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
{"tenant_id":"tenant1","agent_id":"agent-1","tool":"slack","action":"channel.list","risk_score":1,"idempotency_key":"test-allow-1"}
'@ | Set-Content -NoNewline toolcall.json

curl.exe -s "http://localhost:8080/v1/toolcalls" `
  -H "X-API-Key: sk-test-key-1" `
  -H "Content-Type: application/json" `
  --data-binary "@toolcall.json"
```

**Login and capture token:**

```powershell
$body = @{ email = "admin@openclause.dev"; password = "admin123" } | ConvertTo-Json
$resp = Invoke-RestMethod -Method Post -Uri "http://localhost:8090/auth/login" `
  -ContentType "application/json" -Body $body
$TOKEN = $resp.token
```

**Disable / re-enable tenant:**

```powershell
@'{"status":"disabled"}'@ | Set-Content -NoNewline tenant-disable.json
curl.exe -s "http://localhost:8090/admin/tenants/tenant1/status" `
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
