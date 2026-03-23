# OpenClause

A policy-driven governance layer for AI agent tool calls. Every action an agent takes — posting a Slack message, creating a Jira ticket, querying a database — flows through OpenClause, where it is validated, evaluated against OPA policy, optionally routed for human approval, executed via pluggable connectors, and recorded as tamper-evident audit evidence.

**v0.2** adds a web admin console, self-service tenant onboarding, multi-language SDKs, a connector marketplace, policy simulation, compliance exports, and more.

**v0.3** adds DB-backed tenant-scoped approver/user/invite/reset + setup wizard flows, SSO/OIDC auth-provider seams, persistent notification routing, a full deny-spike alert worker and UI, tenant analytics dashboards, API key rotation/primary/expiry metadata UX, tenant policy rule-builder with version diff/rollback + enforcement wiring, Helm charts for console services, and deep usability/correctness fixes from the demo/usability trackers (SDK endpoint + wait semantics, export/error-contract consistency, race/stale UI fixes, invite UX/token visibility, safer execute/tenant-disable handling, and robust date rendering).

**v0.4** adds a gateway-backed connector catalog in the console, server-tracked console auth sessions with admin revocation, an operator-grade Sessions explorer with exports and attribution, real invite email delivery with absolute links and delivery status, Java Gradle-wrapper builds, console-wide UX polish, and a final correctness pass across API-client behavior, docs, demo flow, and logic/evidence edge cases.

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
| **Alert Worker** | — | Background worker evaluating tenant alert rules (currently `deny_spike`) and dispatching notifications. |
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
- Java 11+ and Python 3.9+ if you plan to run the Java/Python SDK tests locally

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

The login response includes both `token` and `session_id`. New console JWTs are tracked server-side so admins can revoke active logins from the Users page, and `POST /auth/logout` revokes the current login session on sign-out.

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
  -d "{
    \"tenant_id\": \"$TENANT_ID\",
    \"agent_id\": \"$AGENT_ID\",
    \"user_id\": \"user-123\",
    \"session_id\": \"run-demo-001\",
    \"trace_id\": \"trace-demo-001\",
    \"labels\": {\"user_name\": \"Avery Analyst\", \"user_email\": \"avery@example.com\"},
    \"tool\": \"slack\",
    \"action\": \"msg.post\",
    \"params\": {\"channel\": \"#general\", \"text\": \"Hello from agent\"},
    \"risk_score\": 3,
    \"idempotency_key\": \"demo-001\"
  }" | jq
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
  -d "{
    \"tenant_id\": \"$TENANT_ID\",
    \"agent_id\": \"$AGENT_ID\",
    \"user_id\": \"user-123\",
    \"session_id\": \"run-demo-001\",
    \"trace_id\": \"trace-demo-001\",
    \"labels\": {\"user_name\": \"Avery Analyst\", \"user_email\": \"avery@example.com\"},
    \"tool\": \"github\",
    \"action\": \"issue.create\",
    \"params\": {\"title\": \"Test issue\"},
    \"risk_score\": 8,
    \"idempotency_key\": \"demo-002\"
  }" | jq
```

Save the `event_id` from the response, then approve via the Console API (platform admins can approve for any tenant):

```bash
export EVENT_ID="<event_id from response>"

# Find the approval request
APPROVAL_ID=$(curl -s http://localhost:8090/admin/approvals/pending \
  -H "Authorization: Bearer $TOKEN" | jq -r --arg eid "$EVENT_ID" '.[] | select(.event_id==$eid) | .id')

# Approve it
curl -s -X POST "http://localhost:8090/admin/approvals/$APPROVAL_ID/approve" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" -d '{}' | jq
```

Then execute the approved action:

```bash
curl -s -X POST "http://localhost:8080/v1/toolcalls/$EVENT_ID/execute" \
  -H "X-API-Key: $RAW_KEY" | jq
```

You can also approve from the console UI at http://localhost:3000/approvals.

### 8. Stop

```bash
make dev-down
```

---

## Web Console

The admin console (http://localhost:3000) provides:

| Page | Description |
|---|---|
| **Overview** | Decision analytics — allow/deny/approve counts, decision timeseries, pending approvals |
| **Approvals** | Pending approval queue with user/agent/session attribution, approve/deny actions, and execution handoff help |
| **Audit Trail** | Searchable event list with tenant, user, agent, trace, session, decision, and risk filters plus event detail |
| **Tenants** | Create/list/disable tenants, view config and usage |
| **Tenant Detail** | Manage agents, API keys, tenant-scoped approvers, analytics, alerts, and notification routing |
| **Sessions** | Operator-grade run explorer derived from `tool_events.session_id`, with approval/execution chain detail, explain summaries, and CSV/JSON export |
| **Policies** | Tenant rule builder, policy versions, diff/rollback, and policy simulation |
| **Alerts** | Tenant alert rules (`deny_spike`) and alert events |
| **Connectors** | Registered connector catalog with supported actions |
| **Users** | Manage console users, roles, invite delivery status, and active console login sessions; invite acceptance and password reset happen on dedicated public routes |

Tenant Detail / Agents notes:
- Agents can be disabled or re-enabled; the product keeps audit history instead of hard-deleting agents.
- Tenant Detail includes a `Hide disabled` toggle. The default view keeps disabled agents visible for operator awareness; checking the toggle hides them by requesting `include_disabled=false`.

### Console Auth

- **Login**: email + password (bcrypt hashed)
- **JWT**: HS256 tokens with configurable expiry
- **RBAC roles**: `platform_admin`, `tenant_admin`, `approver`, `viewer`
- Setup Wizard default email: `admin@openclause.dev` (password is chosen during initialization)

### User, Invite, and Password Reset Flows

The console UI includes token-based pages for invite acceptance and password reset (`/invite/accept` and `/reset`).

Admin-side invites are created from the Users page via `POST /admin/invites`. Create-invite returns the raw invite token once, an absolute `accept_url`, and `email_status` / `email_error` fields so operators can tell whether SMTP delivery succeeded, failed, or was only logged for dev. `GET /admin/invites` exposes pending invites plus delivery status, but never the raw token after creation because invite tokens are hashed at rest. The same Users page shows active console login sessions and lets tenant admins or platform admins revoke them immediately. Password resets are self-service via `POST /auth/reset/request`, and both flows are completed by `POST /auth/invite/accept` / `POST /auth/reset/confirm`. In local development, console-api can log raw invite/reset URLs unless `CONSOLE_DEV_LOG_RAW_TOKENS=false`.

---

## API Reference

OpenAPI 3.1 spec for the core gateway + approvals surface: [`api/openapi.yaml`](api/openapi.yaml)

The console-api surface has outgrown the generated spec, so the current admin/auth endpoints are documented below.

### Gateway (`:8080`)

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/v1/toolcalls` | Submit a tool-call request |
| `GET` | `/v1/toolcalls/{event_id}` | Fetch event by ID |
| `POST` | `/v1/toolcalls/{event_id}/execute` | Resume approved request and execute exactly-once by parent event |
| `GET` | `/v1/connectors` | List all registered connectors |
| `GET` | `/healthz` | Liveness probe |
| `GET` | `/readyz` | Readiness probe (checks Postgres) |

Gateway auth notes:
- `POST /v1/toolcalls`, `GET /v1/toolcalls/{event_id}`, and `POST /v1/toolcalls/{event_id}/execute` require an API key via `X-API-Key` or `Authorization: Bearer <key>`.
- `/healthz`, `/readyz`, and `/v1/connectors` are unauthenticated.

Prometheus metrics are served on a **separate internal-only listener** (default `127.0.0.1:9090/metrics`, see `METRICS_ADDR`).

### Console API (`:8090`)

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| `GET` | `/healthz` | — | Liveness probe |
| `GET` | `/readyz` | — | Readiness probe (checks Postgres) |
| `GET` | `/setup/status` | — | First-run setup status (used by console UI wizard) |
| `POST` | `/setup/initialize` | — | Initialize console bootstrap (creates initial platform admin + first tenant; only works when DB has zero users) |
| `POST` | `/auth/login` | — | Authenticate with email/password, receive JWT |
| `POST` | `/auth/logout` | JWT | Revoke the current console login session and sign out |
| `POST` | `/auth/invite/accept` | — (token-based) | Accept an invite token (creates user + assigns role/tenant scope) |
| `POST` | `/auth/reset/request` | — | Request a password reset token |
| `POST` | `/auth/reset/confirm` | — (token-based) | Confirm password reset token and set a new password |
| `GET` | `/admin/users` | `platform_admin` or `tenant_admin` | List users (scoped by caller's tenant scope) |
| `POST` | `/admin/users` | `platform_admin` or `tenant_admin` | Create a user |
| `POST` | `/admin/users/{id}/roles` | `platform_admin` or `tenant_admin` | Assign a role to a user |
| `DELETE` | `/admin/users/{id}/roles/{role_id}` | `platform_admin` or `tenant_admin` | Remove a role assignment from a user |
| `POST` | `/admin/invites` | `platform_admin` or `tenant_admin` | Create an invite token, return an absolute `accept_url`, and attempt email delivery (`email_status`, `email_error`) |
| `GET` | `/admin/invites` | `platform_admin` or `tenant_admin` | List pending invites with delivery status (never returns the raw invite token) |
| `GET` | `/admin/auth-sessions` | `platform_admin` or `tenant_admin` | List active console login sessions (optionally filter by `user_id`) |
| `POST` | `/admin/auth-sessions/{session_id}/revoke` | `platform_admin` or `tenant_admin` | Revoke an active console login session |
| `GET` | `/admin/analytics/overview` | JWT (scoped by role) | Decision counts, pending approvals, active tenants/agents |
| `GET` | `/admin/analytics/timeseries` | JWT (scoped by role) | Time-bucketed decision counts |
| `GET` | `/admin/tenants/{tenant_id}/analytics/summary` | `tenant_admin` or `platform_admin` | Tenant-scoped analytics summary for dashboards (range, buckets, heatmap, onboarding) |
| `POST` | `/admin/tenants` | platform_admin | Create tenant |
| `GET` | `/admin/tenants` | JWT | List tenants (scoped by role) |
| `GET` | `/admin/tenants/{tenant_id}` | JWT | Get tenant detail |
| `POST` | `/admin/tenants/{tenant_id}/status` | platform_admin | Set tenant status to `active` or `disabled` |
| `POST` | `/admin/tenants/{tenant_id}/agents` | `tenant_admin` or `platform_admin` | Register agent |
| `GET` | `/admin/tenants/{tenant_id}/agents` | JWT (tenant access) | List agents |
| `POST` | `/admin/tenants/{tenant_id}/apikeys` | `tenant_admin` or `platform_admin` | Create API key (returns raw key once) |
| `GET` | `/admin/tenants/{tenant_id}/apikeys` | JWT (tenant access) | List API keys (never returns hashes) |
| `POST` | `/admin/tenants/{tenant_id}/apikeys/{key_id}/revoke` | `tenant_admin` or `platform_admin` | Revoke API key |
| `POST` | `/admin/tenants/{tenant_id}/apikeys/rotate` | `tenant_admin` or `platform_admin` | Rotate key (create new, optional primary switch, optional revoke old primary) |
| `GET` | `/admin/tenants/{tenant_id}/notification-config` | `tenant_admin` or `platform_admin` | Get per-tenant notification routing config |
| `PUT` | `/admin/tenants/{tenant_id}/notification-config` | `tenant_admin` or `platform_admin` | Update per-tenant notification routing config |
| `GET` | `/admin/tenants/{tenant_id}/approvers` | `tenant_admin` or `platform_admin` | List tenant-scoped approvers |
| `POST` | `/admin/tenants/{tenant_id}/approvers` | `tenant_admin` or `platform_admin` | Upsert a tenant-scoped approver |
| `DELETE` | `/admin/tenants/{tenant_id}/approvers/{user_id}` | `tenant_admin` or `platform_admin` | Remove a tenant-scoped approver |
| `GET` | `/admin/approvals/pending` | JWT | List pending approvals with user/session/trace attribution from the originating event |
| `POST` | `/admin/approvals/{id}/approve` | `approver` or `platform_admin` | Approve request (transactional with grant) |
| `POST` | `/admin/approvals/{id}/deny` | `approver` or `platform_admin` | Deny request |
| `GET` | `/admin/events` | JWT | List events (filterable by tenant, user, agent, trace, tool, action, decision, session, and risk range) |
| `GET` | `/admin/events/{event_id}` | JWT | Event detail with policy result + hash chain |
| `GET` | `/admin/events/export/csv` | JWT | Export events as CSV (honors `since` / `until`; `tenant_id` required for platform admins) |
| `GET` | `/admin/reports/export/bundle` | JWT | Export evidence bundle JSON (`tenant_id` required for platform admins; honors `since` / `until`; rejects ranges over 10,000 events) |
| `GET` | `/admin/reports/activity` | JWT | Legacy alias for `/admin/events` |
| `GET` | `/admin/reports/export/csv` | JWT | Legacy alias for `/admin/events/export/csv` |
| `GET` | `/admin/sessions` | JWT | List observed runs derived from `tool_events.session_id` with user/agent attribution, decision counts, and last action summary |
| `GET` | `/admin/sessions/{session_id}` | JWT | Get session summary; ambiguous platform-admin lookups return `400` with tenant `candidates` so the UI can prompt for `tenant_id` |
| `GET` | `/admin/connectors` | JWT | List the full connector registry for the console (works before any toolcalls) |
| `GET` | `/v1/connectors` | JWT | Legacy console-api alias for `/admin/connectors` |
| `GET` | `/admin/sessions/{session_id}/timeline` | JWT | Session timeline grouped by request plus related approval/execution context; ambiguous platform-admin lookups return tenant `candidates` |
| `GET` | `/admin/sessions/{session_id}/export/csv` | JWT | Export a session timeline as CSV (`404` if the session is missing or outside scope) |
| `GET` | `/admin/sessions/{session_id}/export/json` | JWT | Export a session summary + timeline as JSON (`404` if the session is missing or outside scope) |
| `GET` | `/admin/policy/versions` | JWT | List policy versions |
| `POST` | `/admin/policy/versions` | `tenant_admin` or `platform_admin` | Create policy version |
| `POST` | `/admin/policy/simulate` | JWT | Simulate policy against OPA |
| `GET` | `/admin/tenants/{tenant_id}/policy/config` | JWT (tenant access) | Get tenant rule-builder policy config |
| `PUT` | `/admin/tenants/{tenant_id}/policy/config` | `tenant_admin` or `platform_admin` | Update tenant rule-builder policy config |
| `GET` | `/admin/tenants/{tenant_id}/policy/versions` | JWT (tenant access) | List tenant policy config snapshots |
| `POST` | `/admin/tenants/{tenant_id}/policy/versions` | `tenant_admin` or `platform_admin` | Create tenant policy config snapshot |
| `POST` | `/admin/tenants/{tenant_id}/policy/versions/{version_id}/rollback` | `tenant_admin` or `platform_admin` | Roll back tenant policy config to a previous version |
| `POST` | `/admin/tenants/{tenant_id}/policy/simulate` | JWT (tenant access) | Simulate decision using tenant policy builder config |
| `GET` | `/admin/tenants/{tenant_id}/alerts/rules` | `tenant_admin` or `platform_admin` | List alert rules (currently `deny_spike`) |
| `POST` | `/admin/tenants/{tenant_id}/alerts/rules` | `tenant_admin` or `platform_admin` | Create alert rule |
| `PUT` | `/admin/tenants/{tenant_id}/alerts/rules/{rule_id}` | `tenant_admin` or `platform_admin` | Update alert rule |
| `DELETE` | `/admin/tenants/{tenant_id}/alerts/rules/{rule_id}` | `tenant_admin` or `platform_admin` | Delete alert rule |
| `GET` | `/admin/tenants/{tenant_id}/alerts/events` | `tenant_admin` or `platform_admin` | List alert events for a tenant, including delivery status, attempt count, `last_error`, and `next_attempt_at` for pending retries |
| `GET` | `/admin/alerts/rules` | JWT | Legacy global alert rule listing (scoped by caller role/tenant) |
| `POST` | `/admin/alerts/rules` | `tenant_admin` or `platform_admin` | Legacy/global alert rule creation endpoint |
| `GET` | `/admin/alerts/events` | JWT | Legacy global alert event listing (scoped by caller role/tenant, including retry metadata for pending deliveries) |

### Approvals (`:8081`)

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| `GET` | `/healthz` | — | Liveness probe |
| `POST` | `/v1/approvals/requests` | `X-Internal-Token` | Create an approval request (internal) |
| `GET` | `/v1/approvals/requests/{id}` | `X-Internal-Token` | Get approval request details |
| `POST` | `/v1/approvals/requests/{id}/approve` | `X-Internal-Token` | Approve a pending request |
| `POST` | `/v1/approvals/requests/{id}/deny` | `X-Internal-Token` | Deny a pending request |
| `GET` | `/v1/approvals/pending?tenant_id=...&limit=...&offset=...` | `X-Internal-Token` | List pending approvals (paginated, default limit 200) |
| `GET` | `/ui/pending?tenant_id=...` | `X-Internal-Token` | Minimal internal HTML view for pending approvals |
| `POST` | `/v1/integrations/slack/interactions` | Slack signature headers | Slack Block Kit approve/deny callback endpoint |

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

These SDKs wrap the gateway tool-call APIs. Console-admin flows such as invite delivery, session exports, and evidence bundle exports are documented in the Console API section above and in [`docs/LOCAL_TESTING.md`](docs/LOCAL_TESTING.md). Current console contracts to keep in mind:
- Session exports return `404` when the session id is missing or outside the caller's tenant scope.
- Evidence bundle export honors `since` / `until` and returns `400` when the requested window exceeds 10,000 events.
- `POST /admin/invites` returns the raw invite token once plus `accept_url` and `email_status`; later `GET /admin/invites` responses omit the raw token.

Note: the SDK examples below are written for the local dev seed data (`tenant_id="tenant1"`, `agent_id="agent-1"`, `api_key="sk-test-key-1"`). If you initialized via the Setup Wizard, replace these with your real `tenant_id`, `agent_id`, and raw API key.

### Python

```bash
cd sdk/python
# Core SDK supports Python 3.9+.
python3 -m pip install -U pip setuptools wheel
python3 -m pip install -e .

# Optional LangChain integration
python3 -m pip install -e ".[langchain]"
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

LangChain integration: `from openclause.langchain import OpenClauseTool` (install with `openclause[langchain]`)

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

```bash
cd sdk/java && ./gradlew test
```

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
  -d "{
    \"tenant_id\": \"$TENANT_ID\",
    \"tool\": \"slack\",
    \"action\": \"msg.post\",
    \"risk_score\": 3
  }" | jq
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
- Repeated `POST /v1/toolcalls` calls with the same idempotency key return the original `event_id` plus the prior execution `result` or `approval_url` when available.
- Gateway does not overwrite original evidence rows from phase 1.
- Execution evidence is append-only and linked via `tool_executions`.
- If grant is missing, `/execute` returns `409 awaiting approval` (fail-closed).
- If replay/idempotency storage checks fail, gateway returns `500` (no best-effort fallback).

### Sessions And Attribution

- `agent_id` is the tenant-scoped service identity for the caller.
- `session_id` groups related tool calls into an operator-facing run in the Sessions UI. If you omit it, the request still works, but the console will show `(none)` and cannot assemble a run timeline.
- Session IDs are resolved within tenant scope. If a platform admin opens a bare `session_id` that exists in multiple tenants, the API returns a `400` with tenant `candidates`, and the Console prompts them to choose one.
- `user_id` identifies the human end user behind the agent request when you have one.
- `labels.user_name` and `labels.user_email` are optional display helpers used by the Console on Sessions, Approvals, and Audit pages.
- `trace_id` gives operators a stable correlation key across logs, traces, approvals, and exports.

---

## Evidence & Audit Trail

Every tool call is recorded in the `tool_events` table with:

- Full canonical request payload
- Policy decision and reasoning
- Attribution fields such as `session_id`, `user_id`, `trace_id`, and optional display labels
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
  - Honors optional `since` / `until` query parameters.
  - Fails closed with `400` if the requested range exceeds 10,000 events, instead of silently truncating the bundle.
- **Verify bundle**: `go run ./cmd/verify --bundle <file>`

### Database tables

| Table | Purpose |
|---|---|
| `tenants` | Tenant metadata, status, and configuration |
| `agents` | Agent registration per tenant |
| `api_keys` | Hashed API keys with prefix-based lookup |
| `users` | Console user accounts (bcrypt passwords) |
| `user_roles` | RBAC role assignments (platform_admin, tenant_admin, approver, viewer) |
| `sessions` | Reserved table for explicit session metadata; current operator Sessions UI is derived from `tool_events.session_id` |
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

Health endpoints (`/healthz`, `/readyz`) and `GET /v1/connectors` are unauthenticated so the connector catalog can be discovered before any tenant/API key exists. Metrics are served on a separate internal-only port (not exposed on the gateway port).

### Console Authentication (JWT)

The Console API uses JWT (HS256) tokens issued via `POST /auth/login`. Tokens include user ID, email, roles, optional tenant scope, and a server-tracked session ID (`sid`) for revocation. Configure the signing secret via `CONSOLE_JWT_SECRET`.

Console login sessions are tracked in Postgres. Admins can inspect and revoke active sessions from the Users page or via `GET /admin/auth-sessions` and `POST /admin/auth-sessions/{session_id}/revoke`. The UI sign-out action calls `POST /auth/logout`, which revokes the current session instead of only clearing browser storage.

Current login behavior issues a single tenant-scoped JWT for non-platform users. If an account has roles across multiple tenants, login is rejected with a clear error instead of picking an arbitrary tenant scope.

### RBAC Roles

| Role | Scope | Permissions |
|---|---|---|
| `platform_admin` | Global | Full access to all tenants and operations |
| `tenant_admin` | Per-tenant | Manage agents, keys, policies for their tenant |
| `approver` | Per-tenant | Approve/deny requests |
| `viewer` | Per-tenant | Read-only access |

### Internal Service Authentication

Approvals and remote connector services use an `X-Internal-Token` header for service-to-service calls. Configure via:

```
INTERNAL_AUTH_TOKEN=your-shared-secret
```

`approvals`, `connector-slack`, `connector-jira`, and `alert-worker` require this to be set at startup; gateway uses it for downstream internal calls. Token comparisons use constant-time comparison to prevent timing attacks.

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

The admin console uses the console-api proxy endpoint instead:

```bash
curl -s http://localhost:8090/admin/connectors \
  -H "Authorization: Bearer $TOKEN" | jq
```

This returns the same full connector catalog even on a fresh install with zero toolcalls.

In the default dev stack this should return 8 connectors: 2 remote (`slack`, `jira`) and 6 built-in (`github`, `aws`, `servicenow`, `email`, `postgres`, `webhook`).

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

## Tier 2/3 Features
### Per-tenant Notification Routing (Tier 2 item 7)
OpenClause routes newly created approval notifications based on per-tenant configuration stored in `tenants.config.notification_config`.

### Tenant Alerting (Tier 2 item 8 - deny_spike)
OpenClause evaluates per-tenant `deny_spike` alert rules in the background (`cmd/alert-worker`).

When a rule fires, OpenClause creates an `alert_events` row and dispatches notifications using the same per-tenant notification sinks as approval notifications (Slack and/or HTTPS webhooks).

### How to configure Tenant Alerting (deny_spike)
1. Configure alert notification sinks for your tenant (same API as approval notifications):
   - `PUT /admin/tenants/{tenant_id}/notification-config` (JWT; `tenant_admin`)
2. Create a `deny_spike` rule:
   - `POST /admin/tenants/{tenant_id}/alerts/rules` (JWT; `tenant_admin`)
   - Example payload:
     ```json
     {
       "name": "deny-spike-smoke",
       "kind": "deny_spike",
       "enabled": true,
       "config_json": { "n": 3, "m_minutes": 5 }
     }
     ```
3. Generate deny tool-calls and verify alert events:
   - `GET /admin/tenants/{tenant_id}/alerts/events?limit=10`

Tuning:
- `ALERT_WORKER_INTERVAL_SEC` (default `30`) controls how quickly new denies are evaluated.

### Tenant Analytics Dashboard (Tier 3 item 9)
OpenClause provides a tenant-scoped analytics summary endpoint used by the Console UI Analytics tab:
- `GET /admin/tenants/{tenant_id}/analytics/summary?range=24h&bucket_minutes=60&top_agents=5`

The response includes:
- `totals` (allow/deny/approve counts)
- `trend` (time buckets with allow/deny/approve counts)
- `risk_heatmap` (risk_score decision distribution)
- `per_agent` (top agents by total events)
- `onboarding_checklist` (setup progress for the tenant)

### API Key Lifecycle UX (Tier 3 item 10)
API keys now support lifecycle metadata and safer rotation:
- Metadata fields returned by `GET /admin/tenants/{tenant_id}/apikeys`:
  - `last_used_at`
  - `expires_at` (nullable)
  - `is_primary`
- Rotation endpoint:
  - `POST /admin/tenants/{tenant_id}/apikeys/rotate`
  - Payload:
    ```json
    {
      "name": "rotated-2026-03",
      "expires_at": "2030-01-01",
      "make_primary": true,
      "revoke_old_primary": true
    }
    ```
  - Response includes `raw_key` once (must be copied immediately).

Gateway validation now rejects expired DB-backed API keys (`expires_at <= now`).

### Policy Authoring UX (Tier 3 item 11)
The Policies page now provides a tenant-scoped rule builder with versioning and rollback:
- Tenant-scoped config (`/admin/tenants/{tenant_id}/policy/config`)
  - `max_risk_auto_approve` (0..10)
  - `read_actions`, `write_actions`, `destructive_actions` allowlists
  - `require_destructive_approval` toggle
- Version snapshots (`/admin/tenants/{tenant_id}/policy/versions`)
- Rollback (`/admin/tenants/{tenant_id}/policy/versions/{version_id}/rollback`)
- Simulator preview (`/admin/tenants/{tenant_id}/policy/simulate`)

The gateway loads tenant policy config at request time, so saving or rolling back policy config changes enforcement behavior for subsequent toolcalls without restarting services.

### How to configure Policy Authoring (Rule Builder)
1. Select a tenant in `Policies` (platform admins must choose explicit `tenant_id`).
2. Set rule-builder knobs in the UI, then click **Save Tenant Policy Config**.
3. Use **Preview Decision** to simulate a request for that tenant.
4. Create version snapshots before/after major changes.
5. If needed, select an older version and click **Rollback to Selected Version**.

### Helm Charts for Console Services (Tier 3 item 12)
New Helm charts are available for console services:
- `deploy/helm/console-api`
- `deploy/helm/console-ui`

Each chart includes:
- `values.yaml` defaults
- `Deployment` with liveness/readiness probes
- `Service` (ClusterIP)
- Optional `Ingress`
- Environment variable injection and optional `secretRef`

#### How to install via Helm
```bash
# Console API
helm upgrade --install oc-console-api ./deploy/helm/console-api \
  --namespace openclause --create-namespace \
  --set image.repository=ghcr.io/<org>/openclause-console-api \
  --set image.tag=<tag> \
  --set secretRef=console-api-secrets

# Console UI
helm upgrade --install oc-console-ui ./deploy/helm/console-ui \
  --namespace openclause \
  --set image.repository=ghcr.io/<org>/openclause-console-ui \
  --set image.tag=<tag>
```

Render-only validation (without cluster apply):
```bash
helm template oc-console-api ./deploy/helm/console-api >/tmp/oc-console-api.yaml
helm template oc-console-ui ./deploy/helm/console-ui >/tmp/oc-console-ui.yaml
```

### How to configure Per-tenant Notification Routing
1. Log in as a `tenant_admin` for the target tenant.
2. Update routing via the Console API:
   - `PUT /admin/tenants/{tenant_id}/notification-config`
   - Payload example (Slack):
     ```json
     {
       "approver_group": "tenant_admin",
       "notify": [
         { "kind": "slack", "channel": "#team-alerts" }
       ]
     }
     ```
   - Payload example (webhook) with SSRF protection:
     ```json
     {
       "approver_group": "tenant_admin",
       "notify": [
         { "kind": "webhook", "url": "https://hooks.example.com/...", "secret_ref": "webhook_secret_name" }
       ]
     }
     ```
3. (Optional) Use the Console UI form: `Tenants -> (select tenant) -> API Keys -> Notification Routing`.

Webhook URLs are validated server-side (`https` only; private/loopback IPs rejected) to prevent SSRF.

### Evidence Archival

- `cmd/archiver` verifies each tenant hash chain and uploads bundles to MinIO/S3.
- Incremental progress is tracked in `evidence_archive_checkpoints`.
- One-shot local run:
  `ARCHIVER_RUN_ONCE=true ARCHIVER_TENANT_ID=$TENANT_ID go run ./cmd/archiver`

---

## Observability

### Metrics (Prometheus)

Gateway currently exposes Prometheus metrics at `GET /metrics` on the internal metrics listener (default `127.0.0.1:9090`). Key metrics:

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

Defaults below describe runtime behavior when a variable is unset. The checked-in `.env.example` may pre-populate some local-development values.

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
| `GATEWAY_URL` | `http://localhost:8080` | Gateway base URL used by console-api for connector discovery |
| `PUBLIC_APPROVALS_URL` | `APPROVALS_URL` | Public base URL embedded in `approval_url` responses |
| `PUBLIC_BASE_URL` | `http://localhost:3000` | Public console base URL used to build absolute invite/password-reset links |
| `CONSOLE_JWT_SECRET` | — | **Required at runtime.** JWT signing secret for console; local dev scripts generate one if missing |
| `CONSOLE_JWT_EXPIRY_HOURS` | `24` | JWT token expiry in hours |
| `CONSOLE_CORS_ORIGINS` | — | Comma-separated allowed origins for console-api CORS responses |
| `CONSOLE_AUTH_PROVIDER` | `email_password` | Console auth provider implementation (`email_password`, `password`, `local`) |
| `CONSOLE_DEV_LOG_RAW_TOKENS` | `true` | In dev, log raw invite/reset URLs unless explicitly disabled |
| `INVITE_RESET_TOKEN_HMAC_SECRET` | `CONSOLE_JWT_SECRET` | Keyed-HMAC secret for storing invite/reset tokens hashed at rest |
| `SMTP_HOST` | — | SMTP host for real invite email delivery; if unset, console-api logs invite links in dev/test instead |
| `SMTP_PORT` | `587` | SMTP port for invite delivery |
| `SMTP_USER` | — | Optional SMTP username |
| `SMTP_PASS` | — | Optional SMTP password |
| `SMTP_FROM` | — | Required sender address when SMTP delivery is enabled |
| `APPROVALS_ADDR` | `:8081` | Approvals service listen address |
| `APPROVALS_URL` | `http://localhost:8081` | Approvals service URL (for gateway) |
| `CONNECTOR_SLACK_URL` | `http://localhost:8082` | Slack connector URL |
| `CONNECTOR_JIRA_URL` | `http://localhost:8083` | Jira connector URL |
| `API_KEYS` | — | Comma-separated `tenant:key` pairs (env-var auth); env keys still respect tenant disable status in the DB |
| `INTERNAL_AUTH_TOKEN` | — | Shared secret for approvals/connectors/alert-worker internal auth; effectively required for a working stack |
| `ALLOWLIST_SOURCE` | `db` | Approver authorization source (`db`, `env`, `both`). Default is DB-backed approvers. |
| `APPROVER_EMAIL_ALLOWLIST` | — | Dev bootstrap fallback (used only when `ALLOWLIST_SOURCE=env|both`) |
| `APPROVER_SLACK_ALLOWLIST` | — | Dev bootstrap fallback (used only when `ALLOWLIST_SOURCE=env|both`) |
| `MOCK_CONNECTORS` | `true` | Use mock connectors (no real API calls) |
| `SLACK_SIGNING_SECRET` | — | Slack signing secret for interactions endpoint |
| `ALERT_WORKER_INTERVAL_SEC` | `30` | Poll interval for evaluating alert rules (`cmd/alert-worker`). |
| `ALERT_WORKER_BATCH_SIZE` | `100` | Batch size for retrying pending alert events. |
| `APPROVALS_NOTIFIER_ENABLED` | `true` | Enable transactional outbox dispatcher |
| `APPROVALS_NOTIFIER_INTERVAL_SEC` | `5` | Poll interval for approval notification dispatch + request expiry |
| `APPROVALS_NOTIFIER_SOURCE` | `oc://approvals` | CloudEvents source value for approval notifications |
| `WEBHOOK_SECRET_REFS` | — | Mapping `secret_ref=secret` used for HMAC signatures |
| `EVIDENCE_S3_ENDPOINT` | `localhost:9000` | MinIO/S3 endpoint for archiver |
| `EVIDENCE_S3_BUCKET` | `openclause-evidence` | Bucket for archived bundles |
| `EVIDENCE_S3_ACCESS_KEY` | `minioadmin` | MinIO/S3 access key for archiver uploads |
| `EVIDENCE_S3_SECRET_KEY` | `minioadmin` | MinIO/S3 secret key for archiver uploads |
| `EVIDENCE_S3_SECURE` | `false` | Whether archiver should use HTTPS for the S3 endpoint |
| `RATE_LIMIT_PER_TENANT` | `100` | Max requests/sec per tenant |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OTLP endpoint for traces |
| `METRICS_ADDR` | `127.0.0.1:9090` | Internal Prometheus metrics listener address |
| `ARCHIVER_RUN_ONCE` | `true` | Run archiver once and exit instead of polling |
| `ARCHIVER_INTERVAL_SEC` | `300` | Poll interval for background archival mode |
| `ARCHIVER_TENANT_ID` | — | Optional tenant filter for one-shot/local archive runs |

---

## Project Structure

```
OpenClause/
├── api/openapi.yaml                # OpenAPI 3.1 specification
├── cmd/
│   ├── alert-worker/               # Deny-spike alert evaluation + notification worker
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
│   ├── demo.sh
│   ├── dev.ps1
│   ├── dev.sh
│   ├── e2e-curl-happy-path.sh
│   ├── migrate.ps1
│   ├── migrate.sh
│   ├── gen-secret.ps1
│   ├── gen-secret.sh
│   ├── seed-dev.ps1
│   └── seed-dev.sh
├── deploy/
│   ├── docker-compose.yml          # Local development stack
│   ├── helm/                       # Helm charts (gateway, approvals, connectors, console-api, console-ui, alert-worker)
│   ├── terraform/                  # AWS infrastructure (EKS, RDS, S3, ALB)
│   └── dashboards/                 # Grafana dashboard JSON
├── CONTRIBUTING.md                 # Contribution guide
├── .github/workflows/ci.yml        # CI: Go/unit/UI/SDK/policy/browser checks + build
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

### Testing

The repo has broad automated coverage across the main data flows and contracts:

- Schema-isolated Postgres integration tests via [`internal/testdb`](internal/testdb)
- DB-backed handler/store coverage for console setup, auth, tenants, agents, API keys, sessions, exports, analytics, and notification flows
- Approvals and evidence concurrency/idempotency coverage, including grant consumption, hash-chain behavior, and duplicate-request handling
- Alert-worker loop tests covering retry scheduling and sent/failure transitions
- Gateway tests for allow/deny/approve paths, idempotency, and session/trace persistence
- SDK contract tests for Go, TypeScript, Python, and Java
- Policy verification via `opa test policy/bundles/v0/ policy/tests/ -v`

The detailed inventory lives in [`.ai/test-coverage-sweep.md`](.ai/test-coverage-sweep.md).

### Running tests

```bash
go test ./...             # All Go tests
go test -race ./...       # With race detector
go test ./cmd/console-api -run '^$' -fuzz=FuzzParseRangeDurationDoesNotReturnNonPositiveValues -fuzztime=2s  # Example fuzz smoke
opa test policy/bundles/v0/ policy/tests/ -v   # Policy tests
npm --prefix web/console run test              # Console UI tests
npm --prefix web/console run build             # Console UI build
npm --prefix web/console run test:e2e          # Browser smoke pack (after ./scripts/dev.sh + ./scripts/demo.sh)
npm --prefix sdk/typescript run build          # TypeScript SDK build
(npm --prefix sdk/typescript run test)         # TypeScript SDK tests
(cd sdk/java && ./gradlew test)                # Java SDK tests
PYTHONPATH=sdk/python python3 -m unittest discover -s sdk/python/tests -v   # Python SDK tests
python3.9 -c "import openclause; import openclause.client; import openclause.models"  # Python 3.9 install/import contract
```

Core Python SDK import/tests should run on Python 3.9+. Run any LangChain-specific checks in an environment where the `openclause[langchain]` extra is installed.
For local browser smokes on macOS, `web/console/playwright.config.ts` prefers the installed Google Chrome channel outside CI so the suite can work around bundled-Chromium launch restrictions on locked-down hosts. The `browser-smoke` CI job remains the canonical Linux verifier and uploads Playwright artifacts for debugging.

The frontend test inventory and remaining UI follow-ups live in [`.ai/console-ui-test-tracker.md`](.ai/console-ui-test-tracker.md).

### Console UI development

```bash
cd web/console
npm install
npm run dev    # Starts on http://localhost:3000 with API proxy to :8090
```

### Building locally

```bash
make build
# Binaries: bin/gateway, bin/approvals, bin/connector-slack, bin/connector-jira, bin/connector-template, bin/archiver
```

For binaries not covered by `make build`, use direct `go build`, for example:

```bash
go build ./cmd/console-api
go build ./cmd/alert-worker
go build ./cmd/verify
```

---

## Deployment

### Local (Docker Compose)

```bash
./scripts/dev.sh
```

Runs all services including the console UI. See `deploy/docker-compose.yml`.

### Kubernetes (Helm)

Helm charts are in `deploy/helm/` for each service. Current charts include `gateway`, `approvals`, `connector-jira`, `connector-slack`, `console-api`, `console-ui`, and `alert-worker`.

Chart capabilities (chart-specific):

- Deployments with health probes (service-specific paths, e.g. console-api `/healthz` + `/readyz`, console-ui `/`)
- Pod and container security contexts (`runAsNonRoot`, `readOnlyRootFilesystem`, `drop ALL`)
- ClusterIP services
- Optional ingresses
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

1. **go-test** — `go test ./...` + `go vet ./...`
2. **go-race** — `go test -race ./...`
3. **go-fuzz-smoke** — short Go fuzz runs for analytics, auth-header, and timestamp parsers
4. **console-ui** — Vitest + production build for `web/console`
5. **browser-smoke** — Playwright smoke pack against the local stack
6. **typescript-sdk** — TypeScript SDK build + Jest suite
7. **python-sdk-39** — Python 3.9 install/import contract
8. **python-sdk-tests** — Python SDK unit tests
9. **java-sdk** — Java SDK Gradle tests
10. **policy-test** — `opa test` on policy bundles
11. **lint** — `golangci-lint`
12. **build** — Docker images pushed to `ghcr.io` (main branch only)
13. **deploy** — Cluster deployment (main branch only)

---

## License

Copyright © 2026 Bogdan Turcanu.

Licensed under the **Apache License 2.0** with the **Commons Clause License Condition v1.0**.  
You may use, modify, and redistribute this software under Apache 2.0, but you may **not "Sell"** the software (including offering it as part of a paid product/service) without a separate commercial license from the licensor. See the `LICENSE` file for full terms.
