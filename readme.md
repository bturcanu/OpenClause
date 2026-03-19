# OpenClause

A policy-driven governance layer for AI agent tool calls. Every action an agent takes — posting a Slack message, creating a Jira ticket, querying a database — flows through OpenClause, where it is validated, evaluated against OPA policy, optionally routed for human approval, executed via pluggable connectors, and recorded as tamper-evident audit evidence.

**v0.2** adds a web admin console, self-service tenant onboarding, multi-language SDKs, a connector marketplace, policy simulation, compliance exports, and more.

---

## Table of Contents

- [Why](#why)
- [Architecture](#architecture)
- [Services](#services)
- [Quick Start](#quick-start)
- [Web Console](#web-console)
- [API Reference](#api-reference)
- [SDKs](#sdks)
- [Policy System](#policy-system)
- [Approval Workflow](#approval-workflow)
- [Evidence & Audit Trail](#evidence--audit-trail)
- [Authentication](#authentication)
- [Connectors](#connectors)
- [Observability](#observability)
- [Configuration](#configuration)
- [Project Structure](#project-structure)
- [Development](#development)
- [Deployment](#deployment)

---

## Why

AI agents are being given access to production tools — Slack, Jira, cloud APIs, databases. Without a governance layer, there is no visibility into what agents do, no way to enforce policy, and no audit trail for compliance. OpenClause solves this:

- **Default-deny policy** — agents can only take actions explicitly allowed by OPA rules.
- **Human-in-the-loop** — high-risk or destructive actions are routed for human approval before execution.
- **Tamper-evident audit** — every request, decision, and execution result is recorded with a SHA-256 hash chain.
- **Idempotent by design** — duplicate requests return the same result without re-executing.
- **Pluggable connectors** — add new tool integrations by implementing a single interface.
- **Web console** — manage tenants, agents, API keys, approvals, and policies from a browser.
- **Multi-language SDKs** — Python, TypeScript, Java, and Go clients for agent integration.

---

## Architecture

```
┌─────────────┐      ┌──────────┐      ┌───────────────────┐
│  AI Agent   │─────▶│ Gateway  │────▶│  OPA (Policy)     │
│  (SDK)      │      │ :8080    │      │  :8181            │
└─────────────┘      └────┬─────┘      └───────────────────┘
                          │
               ┌──────────┼──────────┐
               │          │          │
               ▼          ▼          ▼
        ┌──────────┐ ┌─────────┐ ┌─────────────────┐
        │Approvals │ │Connector│ │ Built-in         │
        │ :8081    │ │ Slack   │ │ Connectors       │
        │          │ │ :8082   │ │ (GitHub, AWS,    │
        └────┬─────┘ └─────────┘ │ ServiceNow, ...) │
             │                    └─────────────────┘
             ▼
        ┌──────────┐      ┌──────────┐
        │ Postgres │      │  MinIO   │
        │ :5432    │      │  :9000/:9001 │
        └──────────┘      └──────────┘

┌──────────────┐      ┌──────────────┐
│ Console UI   │─────▶│ Console API  │
│ :3000        │      │ :8090        │
└──────────────┘      └──────────────┘
```

**Request flow:**

1. Agent sends `POST /v1/toolcalls` with a canonical `ToolCallRequest`.
2. Gateway validates the request, checks idempotency, and calls OPA.
3. OPA returns one of `allow`, `deny`, or `approve`.
4. On **allow** — gateway executes via the appropriate connector and returns the result.
5. On **deny** — gateway records the denial and returns immediately.
6. On **approve** — gateway creates an approval request and returns an `approval_url`.
7. Every step is recorded as evidence with a per-tenant hash chain.

---

## Services

| Service | Default Port | Description |
|---|---|---|
| **Gateway** | `:8080` | Entrypoint for all tool-call requests. Validates, evaluates policy, routes to connectors. |
| **Console API** | `:8090` | Admin console backend. JWT auth, RBAC, tenant/agent/key management, analytics, policy simulation. |
| **Console UI** | `:3000` | React single-page app for the admin console. |
| **Approvals** | `:8081` | Manages approval requests and grants. Includes Slack interactive approvals. |
| **Connector-Slack** | `:8082` | Executes Slack actions (`msg.post`, `channel.list`, `approval.request`). Supports mock mode. |
| **Connector-Jira** | `:8083` | Executes Jira actions (`issue.create`, `issue.list`). Supports mock mode. |
| **Built-in Connectors** | — | In-process: GitHub, AWS, ServiceNow, Email, Postgres (read-only), Webhook. |
| **OPA** | `:8181` | Open Policy Agent evaluating Rego policy bundles. |
| **Archiver** | — | Evidence archival worker (not started by `docker-compose`; run `cmd/archiver` separately or on a schedule). |
| **Postgres** | `:5432` | Stores events, results, approvals, grants, users, keys, and hash chain. |
| **MinIO** | `:9000` | S3-compatible object storage (console at `:9001`) for evidence archival. |

---

## Quick Start

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose
- [Go 1.25+](https://go.dev/dl/) (for local development)
- [Node.js 22+](https://nodejs.org/) (for console UI development)
- [OPA CLI](https://www.openpolicyagent.org/docs/latest/#running-opa) (for policy tests)

### 1. Clone and configure

```bash
git clone https://github.com/bturcanu/OpenClause.git && cd OpenClause
cp .env.example .env
```

### 2. Start everything

```bash
make dev
```

This builds all services, starts Docker Compose (Postgres, OPA, MinIO, all Go services, console UI), runs migrations, and prints health-check URLs.

### 3. Verify services

```bash
curl http://localhost:8080/healthz   # Gateway
curl http://localhost:8090/healthz   # Console API
```

If this is your first run, start by opening the console UI at `http://localhost:3000` and completing the First-run Setup Wizard (it creates the initial platform admin and first tenant). Use the email/password you set there to log in below.

### 4. Log into the console

Open http://localhost:3000 in your browser, or use the API:

```bash
curl -s -X POST http://localhost:8090/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"<platform-admin-email>","password":"<platform-admin-password>"}' | jq
```

### 5. Create a tenant, agent, and API key

```bash
TOKEN="<token from login response>"

# Create tenant
curl -s -X POST http://localhost:8090/admin/tenants \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Demo Corp"}' | jq

# Save the created tenant_id as TENANT_ID (JSON field: `id`)
export TENANT_ID="<tenant.id from response>"

# Create agent (use tenant_id from response)
curl -s -X POST http://localhost:8090/admin/tenants/$TENANT_ID/agents \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Demo Agent"}' | jq

# Save the created agent id as AGENT_ID (JSON field: `id`)
export AGENT_ID="<agent.id from response>"

# Create API key
curl -s -X POST http://localhost:8090/admin/tenants/$TENANT_ID/apikeys \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Demo Key"}' | jq
# Save the raw_key from the response — it is shown only once!
export RAW_KEY="<raw_key from response>"
```

### 6. Send a test tool call

```bash
curl -s -X POST http://localhost:8080/v1/toolcalls \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $RAW_KEY" \
  -d '{
    "tenant_id": "$TENANT_ID",
    "agent_id": "$AGENT_ID",
    "tool": "slack",
    "action": "msg.post",
    "params": {"channel": "#general", "text": "Hello from agent"},
    "risk_score": 3,
    "idempotency_key": "demo-001"
  }' | jq
```

Expected response (mock mode):

```json
{
  "event_id": "c5f8a...",
  "decision": "allow",
  "result": {
    "status": "success",
    "output_json": {"ok": true, "channel": "#general", "mock": true},
    "duration_ms": 2
  }
}
```

### 7. Test a high-risk action (triggers approval)

```bash
curl -s -X POST http://localhost:8080/v1/toolcalls \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $RAW_KEY" \
  -d '{
    "tenant_id": "$TENANT_ID",
    "agent_id": "$AGENT_ID",
    "tool": "github",
    "action": "issue.create",
    "params": {"title": "Test issue"},
    "risk_score": 8,
    "idempotency_key": "demo-002"
  }' | jq
```

Approve from the console UI at http://localhost:3000/approvals, then execute:

```bash
curl -s -X POST http://localhost:8080/v1/toolcalls/<event_id>/execute \
  -H "X-API-Key: $RAW_KEY" | jq
```

Tip: in the Approvals UI modal, you can use the **Copy execute command** helper to copy the correct `.../<event_id>/execute` curl request, then paste your tenant-scoped API key.

### 8. Stop

```bash
make dev-down
```

---

## Web Console

The admin console (http://localhost:3000) provides:

| Page | Description |
|---|---|
| **Overview** | Decision analytics — allow/deny/approve counts, risk trends, pending approvals |
| **Approvals** | Pending approval queue with approve/deny actions and detail view |
| **Audit Trail** | Searchable event list with filters (tenant, tool, action, decision) + event detail |
| **Tenants** | Create/list/disable tenants, view config and usage |
| **Tenant Detail** | Manage agents, API keys, and tenant-scoped approvers |
| **Sessions** | Session list with event counts, click into timeline view |
| **Policies** | Policy versions, create new versions, policy simulator |
| **Alerts** | Alert rules (deny spike, approve backlog, etc.) and alert events |
| **Connectors** | Installed connectors with supported actions |
| **Users** | Invite users, assign/remove roles, and handle invite acceptance + password reset flows |

### Console Auth

- **Login**: email + password (bcrypt hashed)
- **JWT**: HS256 tokens with configurable expiry
- **RBAC roles**: `platform_admin`, `tenant_admin`, `approver`, `viewer`
- Setup Wizard default email: `admin@openclause.dev` (password is chosen during initialization)

### User, Invite, and Password Reset Flows

The console UI includes token-based pages for invite acceptance and password reset (`/invite/accept` and `/reset`).

Admin-side, users/invites/password resets are created via the console API (`POST /admin/invites`, `POST /auth/reset/request`), and the token is consumed by `POST /auth/invite/accept` / `POST /auth/reset/confirm`.

---

## API Reference

Full OpenAPI 3.1 spec: [`api/openapi.yaml`](api/openapi.yaml)

### Gateway (`:8080`)

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/v1/toolcalls` | Submit a tool-call request |
| `GET` | `/v1/toolcalls/{event_id}` | Fetch event by ID |
| `POST` | `/v1/toolcalls/{event_id}/execute` | Resume approved request and execute exactly-once by parent event |
| `GET` | `/v1/connectors` | List all registered connectors |
| `GET` | `/healthz` | Liveness probe |
| `GET` | `/readyz` | Readiness probe (checks Postgres) |

Prometheus metrics are served on a **separate internal-only listener** (default `127.0.0.1:9090/metrics`, see `METRICS_ADDR`).

### Console API (`:8090`)

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| `GET` | `/setup/status` | — | First-run setup status (used by console UI wizard) |
| `POST` | `/setup/initialize` | — | Initialize console bootstrap (creates initial platform admin + first tenant; only works when DB has zero users) |
| `POST` | `/auth/login` | — | Authenticate with email/password, receive JWT |
| `POST` | `/auth/invite/accept` | — (token-based) | Accept an invite token (creates user + assigns role/tenant scope) |
| `POST` | `/auth/reset/request` | — (token-based) | Request a password reset token |
| `POST` | `/auth/reset/confirm` | — (token-based) | Confirm password reset token and set a new password |
| `GET` | `/admin/users` | `platform_admin` or `tenant_admin` | List users (scoped by caller's tenant scope) |
| `POST` | `/admin/users` | `platform_admin` or `tenant_admin` | Create a user |
| `POST` | `/admin/users/{id}/roles` | `platform_admin` or `tenant_admin` | Assign a role to a user |
| `DELETE` | `/admin/users/{id}/roles/{role_id}` | `platform_admin` or `tenant_admin` | Remove a role assignment from a user |
| `POST` | `/admin/invites` | `platform_admin` or `tenant_admin` | Create an invite token (email + tenant + role) |
| `GET` | `/admin/invites` | `platform_admin` or `tenant_admin` | List pending invites |
| `GET` | `/admin/analytics/overview` | JWT | Decision counts, pending approvals, active tenants/agents |
| `GET` | `/admin/analytics/timeseries` | JWT | Time-bucketed decision counts |
| `POST` | `/admin/tenants` | platform_admin | Create tenant |
| `GET` | `/admin/tenants` | JWT | List tenants (scoped by role) |
| `GET` | `/admin/tenants/{id}` | JWT | Get tenant detail |
| `POST` | `/admin/tenants/{id}/agents` | tenant_admin | Register agent |
| `GET` | `/admin/tenants/{id}/agents` | JWT | List agents |
| `POST` | `/admin/tenants/{id}/apikeys` | tenant_admin | Create API key (returns raw key once) |
| `GET` | `/admin/tenants/{id}/apikeys` | JWT | List API keys (never returns hashes) |
| `POST` | `/admin/tenants/{id}/apikeys/{key_id}/revoke` | tenant_admin | Revoke API key |
| `GET` | `/admin/tenants/{tenant_id}/approvers` | tenant_admin | List tenant-scoped approvers |
| `POST` | `/admin/tenants/{tenant_id}/approvers` | tenant_admin | Upsert a tenant-scoped approver |
| `DELETE` | `/admin/tenants/{tenant_id}/approvers/{user_id}` | tenant_admin | Remove a tenant-scoped approver |
| `GET` | `/admin/approvals/pending` | JWT | List pending approvals |
| `POST` | `/admin/approvals/{id}/approve` | approver | Approve request (transactional with grant) |
| `POST` | `/admin/approvals/{id}/deny` | approver | Deny request |
| `GET` | `/admin/events` | JWT | List events (filterable) |
| `GET` | `/admin/events/{id}` | JWT | Event detail with policy result + hash chain |
| `GET` | `/admin/events/export/csv` | JWT | Export events as CSV |
| `GET` | `/admin/reports/export/bundle` | JWT | Export evidence bundle JSON |
| `GET` | `/admin/sessions` | JWT | List sessions with event counts |
| `GET` | `/admin/sessions/{id}/timeline` | JWT | Session event timeline |
| `GET/POST` | `/admin/policy/versions` | JWT | List/create policy versions |
| `POST` | `/admin/policy/simulate` | JWT | Simulate policy against OPA |
| `GET/POST` | `/admin/alerts/rules` | JWT | List/create alert rules |
| `GET` | `/admin/alerts/events` | JWT | List alert events |

### Approvals (`:8081`)

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/v1/approvals/requests` | Create an approval request (internal) |
| `GET` | `/v1/approvals/requests/{id}` | Get approval request details |
| `POST` | `/v1/approvals/requests/{id}/approve` | Approve a pending request |
| `POST` | `/v1/approvals/requests/{id}/deny` | Deny a pending request |
| `GET` | `/v1/approvals/pending?tenant_id=...&limit=...&offset=...` | List pending approvals (paginated, default limit 200) |
| `POST` | `/v1/integrations/slack/interactions` | Slack Block Kit approve/deny callback endpoint |
| `GET` | `/ui/pending?tenant_id=...` | Web UI for pending approvals |

### ToolCallRequest Schema

```json
{
  "tenant_id":       "string (required)",
  "agent_id":        "string (required)",
  "tool":            "string (required) — e.g. slack",
  "action":          "string (required) — e.g. msg.post",
  "params":          {},
  "resource":        "string (max 2KB)",
  "risk_score":      0,
  "risk_factors":    ["string"],
  "user_id":         "string",
  "session_id":      "string",
  "labels":          {"key": "value"},
  "source_ip":       "string",
  "trace_id":        "string",
  "idempotency_key": "string (required)",
  "requested_at":    "RFC 3339 timestamp",
  "schema_version":  "1.0"
}
```

**Validation rules:**
- `tenant_id`, `agent_id`, `tool`, `action`, `idempotency_key` are required.
- `params` must be <= 64 KB, `resource` <= 2 KB (byte length), `labels` <= 50 entries.
- `idempotency_key` must be <= 256 bytes.
- `risk_score` must be 0–10. If omitted, it defaults to `0` (server-side `int` zero value).
- `schema_version` must be `"1.0"` or omitted (defaults to `"1.0"`). Unknown versions are rejected.
- `tool` and `action` are normalized to lowercase and must match `^[a-z0-9][a-z0-9._-]{0,63}$`.

---

## SDKs

Multi-language SDKs are available in the `sdk/` directory.

Note: the SDK examples below are written for the local dev seed data (`tenant_id="tenant1"`, `agent_id="agent-1"`, `api_key="sk-test-key-1"`). If you initialized via the Setup Wizard, replace these with your real `tenant_id`, `agent_id`, and raw API key.

### Python

```bash
cd sdk/python && pip install -e .
```

```python
from openclause import OpenClauseClient, ToolCallRequest

client = OpenClauseClient(base_url="http://localhost:8080", api_key="<raw_api_key>")

response = client.submit_tool_call(ToolCallRequest(
    tenant_id="<tenant_id>", agent_id="<agent_id>",
    tool="slack", action="msg.post",
    idempotency_key=OpenClauseClient.generate_idempotency_key(),
    params={"channel": "#general", "text": "Hello!"},
    risk_score=3
))

if response.decision == "approve":
    result = client.wait_for_approval(response.event_id)
```

LangChain integration: `from openclause.langchain import OpenClauseTool`

### TypeScript

```bash
cd sdk/typescript && npm install && npm run build
```

```typescript
import { OpenClauseClient } from 'openclause';

const client = new OpenClauseClient({
  baseUrl: 'http://localhost:8080',
  apiKey: '<raw_api_key>'
});

const response = await client.submitToolCall({
  tenant_id: '<tenant_id>', agent_id: '<agent_id>',
  tool: 'slack', action: 'msg.post',
  idempotency_key: OpenClauseClient.generateIdempotencyKey(),
  params: { channel: '#general', text: 'Hello!' },
  risk_score: 3
});
```

MCP server stub: `import { createMCPToolDefinitions } from 'openclause';`

### Java

```java
OpenClauseClient client = new OpenClauseClient("http://localhost:8080", "<raw_api_key>");

ToolCallRequest req = new ToolCallRequest.Builder()
    .tenantId("<tenant_id>").agentId("<agent_id>")
    .tool("slack").action("msg.post")
    .idempotencyKey(OpenClauseClient.generateIdempotencyKey())
    .riskScore(3)
    .build();

ToolCallResponse response = client.submitToolCall(req);
```

### Go

```go
import "github.com/bturcanu/OpenClause/pkg/sdk/client"

c := client.New("http://localhost:8080", "<raw_api_key>")
resp, err := c.Submit(ctx, req)
```

---

## Policy System

OpenClause uses [Open Policy Agent](https://www.openpolicyagent.org/) with Rego policies loaded as bundles.

### Default Policy (`policy/bundles/v0/main.rego`)

| Condition | Decision |
|---|---|
| Risk score >= 7 | **approve** (requires human) |
| Action on destructive list | **approve** (requires human) |
| Action on read allowlist + risk < tenant threshold | **allow** |
| Action on write allowlist + risk < tenant threshold | **allow** |
| Everything else | **deny** |

The auto-allow threshold is configurable per tenant via `max_risk_auto_approve` in `data.json` (default 7 for unknown tenants).

### Data-driven allowlists (`policy/bundles/v0/data.json`)

```json
{
  "allowlist": {
    "read_actions":        ["jira.issue.list", "slack.channel.list", ...],
    "write_actions":       ["slack.msg.post", "jira.issue.create", ...],
    "destructive_actions": ["jira.issue.delete", "slack.channel.delete", ...]
  },
  "tenants": {
    "tenant1": { "name": "Acme Corp", "max_risk_auto_approve": 5 },
    "tenant2": { "name": "Globex Inc", "max_risk_auto_approve": 3 }
  }
}
```

Changing the data file or Rego rules changes gateway behavior with zero code changes.

### Policy Simulation

Test policy decisions without executing actions:

```bash
curl -s -X POST http://localhost:8090/admin/policy/simulate \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "$TENANT_ID",
    "tool": "slack",
    "action": "msg.post",
    "risk_score": 3
  }' | jq
```

### Policy Versioning

Policy versions are tracked per tenant with deployment metadata. Create, list, and rollback versions through the console API or UI.

### Running policy tests

```bash
make policy-test
# or
opa test policy/bundles/v0/ policy/tests/ -v
```

---

## Approval Workflow

OpenClause uses a strict two-phase approval flow:

1. `POST /v1/toolcalls` returns `decision=approve` and `approval_url` for high-risk requests.
2. A human approves/denies via the **Console UI**, API, or **Slack interactive buttons**.
3. Agent calls `POST /v1/toolcalls/{event_id}/execute` to resume execution.
4. Gateway atomically consumes a matching grant and executes connector only once per parent event.
5. Repeated `/execute` calls return the prior execution response (idempotent replay by parent event).

Important behavior:
- Gateway does not overwrite original evidence rows from phase 1.
- Execution evidence is append-only and linked via `tool_executions`.
- If grant is missing, `/execute` returns `409 awaiting approval` (fail-closed).
- If replay/idempotency storage checks fail, gateway returns `500` (no best-effort fallback).

---

## Evidence & Audit Trail

Every tool call is recorded in the `tool_events` table with:

- Full canonical request payload
- Policy decision and reasoning
- Execution result (if allowed)
- SHA-256 **hash chain** linking each event to the previous one per tenant

### Hash chain

Each field is length-prefixed (8-byte big-endian) for domain separation, with a version tag as the first field:

```
hash[n] = SHA-256( len("openclause:chain:v1") || "openclause:chain:v1" || len(hash[n-1]) || hash[n-1] || len(payload) || payload || len(result) || result )
```

This provides tamper evidence — if any row is modified or deleted, the chain breaks. The hash chain is serialised per tenant via a Postgres advisory lock to prevent concurrent writers from forking it. Verification:

```go
evidence.VerifyChain(events) // returns error if chain is broken
```

### Compliance Exports

- **CSV export**: `GET /admin/events/export/csv`
- **Evidence bundle**: `GET /admin/reports/export/bundle?tenant_id=...`
- **Verify bundle**: `go run ./cmd/verify --bundle <file>`

### Database tables

| Table | Purpose |
|---|---|
| `tenants` | Tenant metadata, status, and configuration |
| `agents` | Agent registration per tenant |
| `api_keys` | Hashed API keys with prefix-based lookup |
| `users` | Console user accounts (bcrypt passwords) |
| `user_roles` | RBAC role assignments (platform_admin, tenant_admin, approver, viewer) |
| `sessions` | Agent conversation sessions |
| `tool_events` | One row per incoming request (payload, decision, hash) |
| `tool_results` | Execution outcomes (status, output, duration) |
| `tool_executions` | Links original approved event to append-only execution event |
| `approval_requests` | Pending/approved/denied approval requests |
| `approval_grants` | Granted approvals with scope and usage tracking |
| `approval_notification_outbox` | Transactional webhook/Slack notification outbox |
| `evidence_archive_checkpoints` | Incremental archival checkpoints per tenant |
| `policy_versions` | Policy bundle versions with deployment metadata |
| `alert_rules` | Configurable alert rules per tenant |
| `alert_events` | Triggered alert events |
| `usage_counters` | Daily per-tenant usage counters |

---

## Authentication

### API Key Authentication (Gateway)

API keys are validated via **two stores** in sequence:

1. **Environment keys**: `API_KEYS=tenant1:sk-test-key-1,tenant2:sk-test-key-2`
   - Update `tenant_id` values to match tenant IDs present in your database. The example assumes the dev seed (`docs/seed_dev.sql`) was applied.
2. **Database keys**: Created through the Console API, stored as SHA-256 hashes with an 8-character prefix for indexed lookup.

Pass keys via `X-API-Key` header or `Authorization: Bearer <key>`.

Health endpoints (`/healthz`, `/readyz`) are unauthenticated. Metrics are served on a separate internal-only port (not exposed on the gateway port).

### Console Authentication (JWT)

The Console API uses JWT (HS256) tokens issued via `POST /auth/login`. Tokens include user ID, email, roles, and optional tenant scope. Configure the signing secret via `CONSOLE_JWT_SECRET`.

### RBAC Roles

| Role | Scope | Permissions |
|---|---|---|
| `platform_admin` | Global | Full access to all tenants and operations |
| `tenant_admin` | Per-tenant | Manage agents, keys, policies for their tenant |
| `approver` | Per-tenant | Approve/deny requests |
| `viewer` | Per-tenant | Read-only access |

### Internal Service Authentication

Approvals and connector services **require** an `X-Internal-Token` header for service-to-service calls. Configure via:

```
INTERNAL_AUTH_TOKEN=your-shared-secret
```

**Required** — all services will refuse to start if this is empty. Token comparisons use constant-time comparison to prevent timing attacks.

---

## Connectors

### Remote Connectors (HTTP services)

Each remote connector is a standalone HTTP service with a single `POST /exec` endpoint.

| Connector | Actions |
|---|---|
| **Slack** | `msg.post`, `channel.list`, `approval.request` |
| **Jira** | `issue.create`, `issue.list` |

### Built-in Connectors (in-process)

| Connector | Actions | Mode |
|---|---|---|
| **GitHub** | `issue.create`, `issue.comment`, `repo.list`, `repo.readme` | Mock |
| **AWS** | `s3.list_buckets`, `s3.get_object`, `iam.list_users`, `iam.get_role` | Mock |
| **ServiceNow** | `incident.create`, `incident.list`, `incident.get` | Mock |
| **Email** | `send`, `list_inbox` | Mock |
| **Postgres** | `query.readonly` | Mock |
| **Webhook** | `post` (with SSRF protection) | Mock |

### Connector Discovery

```bash
curl http://localhost:8080/v1/connectors | jq
```

Returns all registered connectors with their names, types, and supported actions.

### Mock Mode

Set `MOCK_CONNECTORS=true` in `.env` to run connectors without real credentials. Mock responses are deterministic and suitable for testing.

### Adding a New Connector

See [`docs/CONNECTORS.md`](docs/CONNECTORS.md) for the full guide, or [`CONTRIBUTING.md`](CONTRIBUTING.md) for contribution instructions.

1. Create `cmd/connector-<name>/main.go` (see `cmd/connector-template`).
2. Implement the `POST /exec` handler using `pkg/connectors/sdk`.
3. Register the tool in the gateway's connector registry.
4. Add the new connector to `docker-compose.yml`.

### Webhook Notifications (CloudEvents + HMAC)

When approval requests are created, notifications are enqueued transactionally and dispatched from `approval_notification_outbox`.

- Event type: `oc.approval.requested`
- Content-Type: `application/cloudevents+json` (structured mode)
- Signature header: `X-OC-Signature-256: sha256=<hex(hmac_sha256(secret, raw_body))>`

### Slack Interactive Approvals

- Endpoint: `POST /v1/integrations/slack/interactions`
- Security: Slack signature verification (`X-Slack-Signature`, `X-Slack-Request-Timestamp`) against `SLACK_SIGNING_SECRET`.
- RBAC is enforced via DB-backed approvers (`users` + `user_roles` with `role='approver'`) scoped to the tenant.
  - Default: DB-only (`ALLOWLIST_SOURCE=db`).
  - Dev bootstrap fallback (optional): env allowlists via `ALLOWLIST_SOURCE=env|both` using `APPROVER_SLACK_ALLOWLIST` / `APPROVER_EMAIL_ALLOWLIST`.
- Approvers are managed in the console UI under `Tenants -> (select tenant) -> Approvers`.

### Evidence Archival

- `cmd/archiver` verifies each tenant hash chain and uploads bundles to MinIO/S3.
- Incremental progress is tracked in `evidence_archive_checkpoints`.
- One-shot local run:
  `ARCHIVER_RUN_ONCE=true ARCHIVER_TENANT_ID=$TENANT_ID go run ./cmd/archiver`

---

## Observability

### Metrics (Prometheus)

Available at `GET /metrics` on the internal metrics listener (default `127.0.0.1:9090`). Key metrics:

- `oc_decisions_total` — decisions by type (allow/deny/approve)
- `oc_policy_eval_duration_seconds` — policy evaluation latency
- `oc_connector_duration_seconds` — connector call latency by tool
- `oc_connector_errors_total` — connector errors by tool
- `oc_approvals_total` — approvals by status
- `oc_idempotency_hits_total` — idempotency cache hit rate
- `oc_requests_total` — request rate by tenant

### Tracing (OpenTelemetry)

Set `OTEL_EXPORTER_OTLP_ENDPOINT` to enable distributed tracing via OTLP/HTTP. Traces propagate across all services using W3C TraceContext.

### Grafana Dashboard

A pre-built dashboard is provided at `deploy/dashboards/gateway.json`. Import it into Grafana pointing at your Prometheus data source.

---

## Configuration

All configuration is via environment variables. See [`.env.example`](.env.example) for the full list.

| Variable | Default | Description |
|---|---|---|
| `POSTGRES_HOST` | `localhost` | Postgres host |
| `POSTGRES_PORT` | `5432` | Postgres port |
| `POSTGRES_USER` | `openclause` | Postgres user |
| `POSTGRES_PASSWORD` | `changeme` | Postgres password |
| `POSTGRES_DB` | `openclause` | Postgres database name |
| `POSTGRES_SSLMODE` | `disable` | Postgres SSL mode (`disable`, `require`, `verify-full`, etc.) |
| `OPA_URL` | `http://localhost:8181` | OPA server URL |
| `GATEWAY_ADDR` | `:8080` | Gateway listen address |
| `CONSOLE_API_ADDR` | `:8090` | Console API listen address |
| `CONSOLE_JWT_SECRET` | — | **Required for production.** JWT signing secret for console |
| `CONSOLE_JWT_EXPIRY_HOURS` | `24` | JWT token expiry in hours |
| `APPROVALS_ADDR` | `:8081` | Approvals service listen address |
| `APPROVALS_URL` | `http://localhost:8081` | Approvals service URL (for gateway) |
| `CONNECTOR_SLACK_URL` | `http://localhost:8082` | Slack connector URL |
| `CONNECTOR_JIRA_URL` | `http://localhost:8083` | Jira connector URL |
| `API_KEYS` | — | Comma-separated `tenant:key` pairs (env-var auth) |
| `INTERNAL_AUTH_TOKEN` | — | **Required.** Shared secret for service-to-service auth |
| `ALLOWLIST_SOURCE` | `db` | Approver authorization source (`db`, `env`, `both`). Default is DB-backed approvers. |
| `APPROVER_EMAIL_ALLOWLIST` | — | Dev bootstrap fallback (used only when `ALLOWLIST_SOURCE=env|both`) |
| `APPROVER_SLACK_ALLOWLIST` | — | Dev bootstrap fallback (used only when `ALLOWLIST_SOURCE=env|both`) |
| `MOCK_CONNECTORS` | `true` | Use mock connectors (no real API calls) |
| `SLACK_SIGNING_SECRET` | — | Slack signing secret for interactions endpoint |
| `APPROVALS_NOTIFIER_ENABLED` | `true` | Enable transactional outbox dispatcher |
| `WEBHOOK_SECRET_REFS` | — | Mapping `secret_ref=secret` used for HMAC signatures |
| `EVIDENCE_S3_ENDPOINT` | `localhost:9000` | MinIO/S3 endpoint for archiver |
| `EVIDENCE_S3_BUCKET` | `openclause-evidence` | Bucket for archived bundles |
| `RATE_LIMIT_PER_TENANT` | `100` | Max requests/sec per tenant |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OTLP endpoint for traces |
| `METRICS_ADDR` | `127.0.0.1:9090` | Internal Prometheus metrics listener address |

---

## Project Structure

```
OpenClause/
├── api/openapi.yaml                # OpenAPI 3.1 specification
├── cmd/
│   ├── gateway/                    # Gateway service (:8080)
│   ├── console-api/                # Console API service (:8090)
│   ├── approvals/                  # Approvals service (:8081)
│   ├── connector-slack/            # Slack connector
│   ├── connector-jira/             # Jira connector
│   ├── connector-template/         # Example connector using SDK
│   ├── archiver/                   # Evidence archival worker/CLI
│   ├── verify/                     # Evidence bundle verification CLI
│   └── llm-summarizer/            # Optional LLM summarizer (Python FastAPI)
├── pkg/
│   ├── types/                      # Canonical schema, validation, errors
│   ├── policy/                     # OPA HTTP client + shadow evaluator
│   ├── evidence/                   # Canonicalization, hash chain, Postgres store
│   ├── auth/                       # API key middleware (env + DB), composite store
│   ├── console/                    # Console store (CRUD, analytics, JWT)
│   ├── connectors/                 # Connector interface, registry, routing
│   │   ├── sdk/                    # Connector SDK helper
│   │   └── builtins/              # Built-in connectors (GitHub, AWS, etc.)
│   ├── approvals/                  # Approval types, store, handlers, summary
│   ├── risk/                       # Risk scoring interfaces + implementations
│   ├── archiver/                   # Bundle builder + archival service
│   ├── otel/                       # OpenTelemetry setup
│   ├── config/                     # Shared environment variable helpers
│   └── sdk/client/                 # Go client SDK
├── sdk/
│   ├── python/                     # Python SDK (pip)
│   ├── typescript/                 # TypeScript SDK (npm)
│   └── java/                       # Java SDK (gradle)
├── web/console/                    # React admin console (Vite + TypeScript)
├── policy/
│   ├── bundles/v0/                 # OPA policy bundle (main.rego + data.json)
│   └── tests/                      # OPA policy tests
├── migrations/
│   ├── 001_initial.sql             # Postgres schema
├── docs/
│   ├── CONNECTORS.md              # Connector development guide
│   ├── LOCAL_TESTING.md          # Local testing guide + curl recipes
│   └── seed_dev.sql              # Optional development seed SQL (tenant/agent/api-key)
├── scripts/
│   ├── dev.ps1
│   ├── dev.sh
│   ├── migrate.ps1
│   ├── migrate.sh
│   ├── gen-secret.ps1
│   ├── gen-secret.sh
│   ├── seed-dev.ps1
│   └── seed-dev.sh
├── deploy/
│   ├── docker-compose.yml          # Local development stack
│   ├── helm/                       # Helm charts (gateway, approvals, connectors)
│   ├── terraform/                  # AWS infrastructure (EKS, RDS, S3, ALB)
│   └── dashboards/                 # Grafana dashboard JSON
├── CONTRIBUTING.md                 # Contribution guide
├── .github/workflows/ci.yml        # CI: test, lint, policy-test, build, deploy
├── Dockerfile                      # Multi-stage build (one binary per image, non-root)
├── LICENSE                         # License text (Apache-2.0 + Commons Clause)
├── Makefile                        # dev, test, build, deploy targets
├── .env.example                    # Environment variable reference
└── readme.md                       # This file
```

---

## Development

### Make Targets

| Target | Description |
|---|---|
| `make dev` | Start full stack locally (Docker Compose) |
| `make dev-down` | Stop and remove all containers + volumes |
| `make logs` | Tail logs from all services |
| `make migrate` | Run Postgres migrations |
| `make test` | Run all tests (Go + policy) |
| `make go-test` | Run Go unit tests only |
| `make policy-test` | Run OPA policy tests only |
| `make lint` | Run golangci-lint |
| `make build` | Build all Go binaries to `bin/` |
| `make docker-build` | Build Docker images locally |
| `make clean` | Remove build artifacts and containers |

### Windows + macOS/Linux Quickstart

For first-run developer setup (macOS/Linux):

```bash
./scripts/dev.sh
```

For Windows (PowerShell):

```powershell
.\scripts\dev.ps1
```

If you need migrations only:

```bash
./scripts/migrate.sh
```

```powershell
.\scripts\migrate.ps1
```

On console-ui startup, the app will call `GET /api/setup/status` and show a first-run setup wizard when the DB is not initialized yet. This replaces the need to manually seed the initial platform admin + first tenant SQL on first run.

For local development, `./scripts/dev.sh` / `./scripts/dev.ps1` also ensure `CONSOLE_JWT_SECRET` is set in `.env` (generating a strong secret if missing) and run `docker compose` with `--env-file` so compose interpolation uses the correct values.

### Running tests

```bash
go test ./...             # All Go tests
go test -race ./...       # With race detector
opa test policy/bundles/v0/ policy/tests/ -v   # Policy tests
```

### Console UI development

```bash
cd web/console
npm install
npm run dev    # Starts on http://localhost:3000 with API proxy to :8090
```

### Building locally

```bash
make build
# Binaries: bin/gateway, bin/approvals, bin/console-api, bin/connector-*, bin/archiver, bin/verify
```

---

## Deployment

### Local (Docker Compose)

```bash
./scripts/dev.sh
```

Runs all services including the console UI. See `deploy/docker-compose.yml`.

### Kubernetes (Helm)

Helm charts are in `deploy/helm/` for each service. All charts include:

- Deployments with liveness (`/healthz`) and readiness (`/readyz`) probes
- Pod and container security contexts (`runAsNonRoot`, `readOnlyRootFilesystem`, `drop ALL`)
- ClusterIP services
- Deny-by-default NetworkPolicies
- Optional `secretRef` for loading secrets from Kubernetes Secrets

### Cloud (Terraform)

Terraform modules in `deploy/terraform/` provision AWS infrastructure:

| Module | Resources |
|---|---|
| `cluster` | EKS cluster + node group |
| `database` | RDS PostgreSQL 16 |
| `storage` | S3 bucket with versioning + encryption |
| `secrets` | Secrets Manager for credentials |
| `loadbalancer` | ALB + ACM certificate |

### CI/CD

GitHub Actions (`.github/workflows/ci.yml`) runs on push/PR to `main`:

1. **test** — `go test ./...` + `go vet ./...`
2. **policy-test** — `opa test` on policy bundles
3. **lint** — `golangci-lint`
4. **build** — Docker images pushed to `ghcr.io` (main branch only)
5. **deploy** — Cluster deployment (main branch only)

---

## License

Copyright © 2026 Bogdan Turcanu.

Licensed under the **Apache License 2.0** with the **Commons Clause License Condition v1.0**.  
You may use, modify, and redistribute this software under Apache 2.0, but you may **not "Sell"** the software (including offering it as part of a paid product/service) without a separate commercial license from the licensor. See the `LICENSE` file for full terms.
