# OpenClause

A policy-driven governance layer for AI agent tool calls. Every action an agent takes — posting a Slack message, creating a Jira ticket, querying a database — flows through OpenClause, where it is validated, evaluated against OPA policy, optionally routed for human approval, executed via pluggable connectors, and recorded as tamper-evident audit evidence.

---

## Table of Contents

- [Why](#why)
- [Architecture](#architecture)
- [Services](#services)
- [Quick Start](#quick-start)
- [API Reference](#api-reference)
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
- [Build Plan](#build-plan)

---

## Why

AI agents are being given access to production tools — Slack, Jira, cloud APIs, databases. Without a governance layer, there is no visibility into what agents do, no way to enforce policy, and no audit trail for compliance. OpenClause solves this:

- **Default-deny policy** — agents can only take actions explicitly allowed by OPA rules.
- **Human-in-the-loop** — high-risk or destructive actions are routed for human approval before execution.
- **Tamper-evident audit** — every request, decision, and execution result is recorded with a SHA-256 hash chain.
- **Idempotent by design** — duplicate requests return the same result without re-executing.
- **Pluggable connectors** — add new tool integrations by implementing a single interface.

---

## Architecture

```
┌─────────────┐      ┌──────────┐      ┌───────────────────┐
│  AI Agent   │─────▶│ Gateway  │────▶│  OPA (Policy)     │
│             │      │ :8080    │      │  :8181            │
└─────────────┘      └────┬─────┘      └───────────────────┘
                          │
               ┌──────────┼──────────┐
               │          │          │
               ▼          ▼          ▼
        ┌──────────┐ ┌─────────┐ ┌─────────┐
        │Approvals │ │Connector│ │Connector│
        │ :8081    │ │ Slack   │ │ Jira    │
        │          │ │ :8082   │ │ :8083   │
        └────┬─────┘ └─────────┘ └─────────┘
             │
             ▼
        ┌──────────┐      ┌──────────┐
        │ Postgres │      │  MinIO   │
        │ :5432    │      │  :9000   │
        └──────────┘      └──────────┘
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
| **Approvals** | `:8081` | Manages approval requests and grants. Includes a minimal web UI. |
| **Connector-Slack** | `:8082` | Executes Slack actions (`msg.post`). Supports mock mode. |
| **Connector-Jira** | `:8083` | Executes Jira actions (`issue.create`). Supports mock mode. |
| **OPA** | `:8181` | Open Policy Agent evaluating Rego policy bundles. |
| **Postgres** | `:5432` | Stores events, results, approvals, grants, and hash chain. |
| **MinIO** | `:9000` | S3-compatible object storage for evidence archival. |

---

## Quick Start

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose
- [Go 1.25+](https://go.dev/dl/) (for local development)
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

This builds all services, starts Docker Compose (Postgres, OPA, MinIO, all 4 Go services), runs migrations, and prints health-check URLs.

### 3. Verify

```bash
curl http://localhost:8080/healthz
# OK
```

### 4. Send a test tool call

```bash
curl -s -X POST http://localhost:8080/v1/toolcalls \
  -H "Content-Type: application/json" \
  -H "X-API-Key: sk-test-key-1" \
  -d '{
    "tenant_id": "tenant1",
    "agent_id": "agent-1",
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

### 5. Test a high-risk action (triggers approval)

```bash
curl -s -X POST http://localhost:8080/v1/toolcalls \
  -H "Content-Type: application/json" \
  -H "X-API-Key: sk-test-key-1" \
  -d '{
    "tenant_id": "tenant1",
    "agent_id": "agent-1",
    "tool": "jira",
    "action": "issue.delete",
    "risk_score": 8,
    "idempotency_key": "demo-002"
  }' | jq
```

Response includes an `approval_url` — follow it to approve or deny.

### 6. Stop

```bash
make dev-down
```

---

## API Reference

Full OpenAPI 3.1 spec: [`api/openapi.yaml`](api/openapi.yaml)

### Gateway

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/v1/toolcalls` | Submit a tool-call request |
| `GET` | `/v1/toolcalls/{event_id}` | Fetch event by ID |
| `GET` | `/healthz` | Liveness probe |
| `GET` | `/readyz` | Readiness probe (checks Postgres) |
| `GET` | `/metrics` | Prometheus metrics |

### Approvals

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/v1/approvals/requests` | Create an approval request (internal) |
| `GET` | `/v1/approvals/requests/{id}` | Get approval request details |
| `POST` | `/v1/approvals/requests/{id}/approve` | Approve a pending request |
| `POST` | `/v1/approvals/requests/{id}/deny` | Deny a pending request |
| `GET` | `/v1/approvals/pending?tenant_id=...&limit=...&offset=...` | List pending approvals (paginated, default limit 200) |
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
- `risk_score` must be 0–10. Omitting it will result in a policy deny (OPA comparisons against undefined produce false).
- `schema_version` must be `"1.0"` or omitted (defaults to `"1.0"`). Unknown versions are rejected.
- `tool` and `action` are normalized to lowercase.

---

## Policy System

OpenClause uses [Open Policy Agent](https://www.openpolicyagent.org/) with Rego policies loaded as bundles.

### Default Policy (`policy/bundles/v0/main.rego`)

| Condition | Decision |
|---|---|
| Action on read allowlist + risk <= 3 | **allow** |
| Action on write allowlist + risk < 7 | **allow** |
| Risk score >= 7 | **approve** (requires human) |
| Action on destructive list | **approve** (requires human) |
| Everything else | **deny** |

### Data-driven allowlists (`policy/bundles/v0/data.json`)

```json
{
  "allowlist": {
    "read_actions":        ["jira.issue.list", "slack.channel.list", ...],
    "write_actions":       ["slack.msg.post", "jira.issue.create", ...],
    "destructive_actions": ["jira.issue.delete", "slack.channel.delete", ...]
  }
}
```

Changing the data file or Rego rules changes gateway behavior with zero code changes.

### Running policy tests

```bash
make policy-test
# or
opa test policy/bundles/v0/ policy/tests/ -v
```

---

## Approval Workflow

When policy returns `approve`:

1. Gateway creates an `ApprovalRequest` with details (tool, action, agent, risk score).
2. Response includes an `approval_url` pointing to the approvals service.
3. A human reviews the request via the API or the web UI (`/ui/pending`).
4. On approve, an `ApprovalGrant` is created with a defined **scope**:
   - `tool` / `action` (exact or `*` wildcard)
   - `resource_pattern` (glob match)
   - `max_uses` (default 1 — single use)
   - `expires_at`
5. The agent (or gateway) can now re-submit. The gateway finds and consumes the matching grant atomically.

---

## Evidence & Audit Trail

Every tool call is recorded in the `tool_events` table with:

- Full canonical request payload
- Policy decision and reasoning
- Execution result (if allowed)
- SHA-256 **hash chain** linking each event to the previous one per tenant

### Hash chain

Each field is length-prefixed (8-byte big-endian) for domain separation:

```
hash[n] = SHA-256( len(hash[n-1]) || hash[n-1] || len(payload) || payload || len(result) || result )
```

This provides tamper evidence — if any row is modified or deleted, the chain breaks. The hash chain is serialised per tenant via a Postgres advisory lock to prevent concurrent writers from forking it. Verification:

```go
evidence.VerifyChain(events) // returns error if chain is broken
```

### Database tables

| Table | Purpose |
|---|---|
| `tool_events` | One row per incoming request (payload, decision, hash) |
| `tool_results` | Execution outcomes (status, output, duration) |
| `approval_requests` | Pending/approved/denied approval requests |
| `approval_grants` | Granted approvals with scope and usage tracking |
| `tenants` | Tenant metadata and configuration |
| `agents` | Agent registration per tenant |
| `policy_versions` | Bundle deployment tracking |

---

## Authentication

### API Key Authentication (Gateway)

Pass tenant API keys via the `X-API-Key` header or `Authorization: Bearer <key>`.

Configure keys in `.env`:

```
API_KEYS=tenant1:sk-test-key-1,tenant2:sk-test-key-2
```

The middleware maps the key to a `tenant_id` and injects it into the request context. Keys are stored in memory as SHA-256 hashes — raw keys never persist.

Health and metrics endpoints (`/healthz`, `/readyz`, `/metrics`) are unauthenticated.

### Internal Service Authentication

Approvals and connector services require an `X-Internal-Token` header for service-to-service calls. Configure via:

```
INTERNAL_AUTH_TOKEN=your-shared-secret
```

Leave empty to disable (development only). The gateway does not currently forward this header automatically — configure it in service environment variables for production.

---

## Connectors

Connectors implement tool integrations. Each one is a standalone HTTP service with a single `POST /exec` endpoint.

### Supported Actions

| Connector | Action | Description |
|---|---|---|
| **Slack** | `slack.msg.post` | Post a message to a channel |
| **Jira** | `jira.issue.create` | Create a Jira issue |

### Mock Mode

Set `MOCK_CONNECTORS=true` in `.env` to run connectors without real credentials. Mock responses are deterministic and suitable for testing.

### Adding a New Connector

1. Create `cmd/connector-<name>/main.go`.
2. Implement the `POST /exec` handler accepting `connectors.ExecRequest` and returning `connectors.ExecResponse`.
3. Register the tool in the gateway's connector registry.
4. Add the new connector to `docker-compose.yml`.

---

## Observability

### Metrics (Prometheus)

Available at `GET /metrics` on the gateway. Key metrics:

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
| `OPA_URL` | `http://localhost:8181` | OPA server URL |
| `GATEWAY_ADDR` | `:8080` | Gateway listen address |
| `APPROVALS_ADDR` | `:8081` | Approvals service listen address |
| `APPROVALS_URL` | `http://localhost:8081` | Approvals service URL (for gateway) |
| `CONNECTOR_SLACK_URL` | `http://localhost:8082` | Slack connector URL |
| `CONNECTOR_JIRA_URL` | `http://localhost:8083` | Jira connector URL |
| `API_KEYS` | — | Comma-separated `tenant:key` pairs |
| `INTERNAL_AUTH_TOKEN` | — | Shared secret for service-to-service auth (approvals, connectors) |
| `MOCK_CONNECTORS` | `true` | Use mock connectors (no real API calls) |
| `SLACK_BOT_TOKEN` | — | Slack bot OAuth token |
| `JIRA_BASE_URL` | — | Jira instance URL |
| `JIRA_EMAIL` | — | Jira auth email |
| `JIRA_API_TOKEN` | — | Jira API token |
| `RATE_LIMIT_PER_TENANT` | `100` | Max requests/sec per tenant |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OTLP endpoint for traces |
| `OTEL_SERVICE_NAME` | `oc-gateway` | OpenTelemetry service name |

---

## Project Structure

```
OpenClause/
├── api/
│   └── openapi.yaml              # OpenAPI 3.1 specification
├── cmd/
│   ├── gateway/                   # Gateway service
│   ├── approvals/                 # Approvals service (+ web UI)
│   ├── connector-slack/           # Slack connector
│   └── connector-jira/            # Jira connector
├── pkg/
│   ├── types/                     # Canonical schema, validation, errors
│   ├── policy/                    # OPA HTTP client
│   ├── evidence/                  # Canonicalization, hash chain, Postgres store
│   ├── auth/                      # API key middleware, internal auth
│   ├── otel/                      # OpenTelemetry setup
│   ├── config/                    # Shared environment variable helpers
│   ├── connectors/                # Connector interface, registry, routing
│   └── approvals/                 # Approval types, store, handlers
├── policy/
│   ├── bundles/v0/                # OPA policy bundle (main.rego + data.json)
│   └── tests/                     # OPA policy tests
├── migrations/
│   ├── 001_initial.sql            # Postgres schema (DDL only)
│   └── 002_seed.sql               # Development seed data (tenants, agents)
├── deploy/
│   ├── docker-compose.yml         # Local development stack
│   ├── helm/                      # Helm charts (gateway, approvals, connectors)
│   ├── terraform/                 # AWS infrastructure (EKS, RDS, S3, ALB)
│   └── dashboards/                # Grafana dashboard JSON
├── .github/workflows/
│   └── ci.yml                     # CI: test, lint, policy-test, build, deploy
├── Dockerfile                     # Multi-stage build (one binary per image, non-root)
├── Makefile                       # dev, test, build, deploy targets
├── .env.example                   # Environment variable reference
└── readme.md                      # This file
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

### Running tests

```bash
# All tests
make test

# Go unit tests only
go test ./... -v

# Policy tests only
opa test policy/bundles/v0/ policy/tests/ -v
```

### Building locally (without Docker)

```bash
make build
# Binaries output to bin/gateway, bin/approvals, bin/connector-slack, bin/connector-jira
```

---

## Deployment

### Local (Docker Compose)

```bash
make dev
```

Runs gateway, approvals, 2 connectors, OPA, Postgres, and MinIO. See `deploy/docker-compose.yml`.

### Kubernetes (Helm)

Helm charts are in `deploy/helm/` for each service. All charts include:

- Deployments with liveness (`/healthz`) and readiness (`/readyz`) probes
- Pod and container security contexts (`runAsNonRoot`, `readOnlyRootFilesystem`, `drop ALL`)
- ClusterIP services
- Deny-by-default NetworkPolicies (connectors allow TCP 443 egress for external APIs)
- Optional `secretRef` for loading secrets from Kubernetes Secrets (`values.secretRef`)
- Gateway chart includes Ingress with TLS

```bash
helm install oc-gateway deploy/helm/gateway/ -f custom-values.yaml
helm install oc-approvals deploy/helm/approvals/
helm install oc-connector-slack deploy/helm/connector-slack/
helm install oc-connector-jira deploy/helm/connector-jira/
```

### Cloud (Terraform)

Terraform modules in `deploy/terraform/` provision AWS infrastructure:

| Module | Resources |
|---|---|
| `cluster` | EKS cluster + node group |
| `database` | RDS PostgreSQL 16 |
| `storage` | S3 bucket with versioning + encryption |
| `secrets` | Secrets Manager for credentials |
| `loadbalancer` | ALB + ACM certificate |

```bash
cd deploy/terraform
terraform init
terraform plan -var-file=prod.tfvars
terraform apply
```

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
You may use, modify, and redistribute this software under Apache 2.0, but you may **not “Sell”** the software (including offering it as part of a paid product/service) without a separate commercial license from the licensor. See the `LICENSE` file for full terms.
