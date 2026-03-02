# OpenClause — Product Gap Tracker

## Current Capabilities Inventory

| Component | Status | Description |
|-----------|--------|-------------|
| Gateway (cmd/gateway) | ✅ Implemented | Tool-call entrypoint, policy eval, connector routing, evidence recording |
| Approvals (cmd/approvals) | ✅ Implemented | Approval lifecycle, Slack interactive approvals, notification dispatcher |
| Console API (cmd/console-api) | ✅ **NEW** | Admin console backend — JWT auth, RBAC, CRUD for tenants/agents/keys |
| Console UI (web/console) | ✅ **NEW** | React SPA — dashboard, approvals, audit trail, tenants, sessions, policies |
| Connector Slack | ✅ Implemented | msg.post, channel.list, approval.request |
| Connector Jira | ✅ Implemented | issue.create, issue.list |
| Built-in GitHub | ✅ **NEW** | issue.create, issue.comment, repo.list, repo.readme (mock) |
| Built-in AWS | ✅ **NEW** | s3.list_buckets, s3.get_object, iam.list_users, iam.get_role (mock) |
| Built-in ServiceNow | ✅ **NEW** | incident.create, incident.list, incident.get (mock) |
| Built-in Email | ✅ **NEW** | send, list_inbox (mock) |
| Built-in Postgres | ✅ **NEW** | query.readonly (mock) |
| Built-in Webhook | ✅ **NEW** | post with SSRF protection |
| Python SDK | ✅ **NEW** | OpenClauseClient with full lifecycle + LangChain integration |
| TypeScript SDK | ✅ **NEW** | OpenClauseClient with full lifecycle + MCP stub |
| Java SDK | ✅ **NEW** | OpenClauseClient with full lifecycle (builder pattern) |
| Go SDK (pkg/sdk/client) | ✅ Implemented | Submit, Execute, WaitForApprovalThenExecute |
| Verify CLI (cmd/verify) | ✅ **NEW** | Evidence bundle verification tool |
| LLM Summarizer (cmd/llm-summarizer) | ✅ **NEW** | FastAPI service scaffold with optional HF model |
| Evidence Store | ✅ Implemented | Hash-chain append-only audit log |
| Auth Middleware | ✅ Enhanced | API key auth — now supports DB-backed + env-var keys via CompositeKeyStore |
| Risk Scoring | ✅ **NEW** | RiskScorer interface + PassthroughScorer + RulesBasedScorer |
| Shadow Mode | ✅ **NEW** | ShadowEvaluator for policy A/B testing |
| SSRF Protection | ✅ Implemented | Webhook URL validation (HTTPS, no private IPs) |
| Rate Limiting | ✅ Implemented | Per-tenant with bounded LRU |

## Release Plan

**v0.2 "Sellable Demo"** = Tier 1 (all) + minimum viable Tier 2

---

## Tier 1 — Adoption Blockers (sellable demo)

- [x] **(1) Dashboard / Admin UI (web console)**
  - **Scope**: New `cmd/console-api` Go service + `web/console` Vite React frontend
  - **Files**: `cmd/console-api/main.go`, `web/console/`, `pkg/console/store.go`, `pkg/console/jwt.go`
  - **Pages**: Overview analytics, Approvals queue, Audit trail, Tenants, Agents, API Keys, Policies, Sessions, Alerts, Connectors
  - **Auth**: JWT (HS256) + bcrypt password login, RBAC (platform_admin, tenant_admin, approver, viewer)
  - **Acceptance**: docker-compose up → login (admin@openclause.dev / admin123) → create tenant → see analytics → browse audit trail

- [x] **(2) Self-service tenant/agent onboarding**
  - **Scope**: Admin CRUD APIs for tenants, agents, API keys with DB-backed auth
  - **Files**: `pkg/console/store.go`, `migrations/001_initial.sql`, `pkg/auth/dbkeys.go`, `pkg/auth/middleware.go`
  - **Schema**: api_keys (SHA-256 hashed + prefix), users (bcrypt), user_roles tables
  - **Endpoints**: All admin endpoints implemented in console-api
  - **Gateway update**: CompositeKeyStore (env + DB) for seamless key validation
  - **Acceptance**: Create tenant → register agent → mint key → use key in gateway → revoke → rejected

- [x] **(3) Python/TypeScript/Java SDKs**
  - **Scope**: Three SDK packages with OpenClauseClient, typed models, quickstarts
  - **Files**: `sdk/python/`, `sdk/typescript/`, `sdk/java/`
  - **Features**: submitToolCall, waitForApproval, execute, trace_id propagation, idempotency helper
  - **Tests**: Python (10 tests), TypeScript (Jest tests), Java (JUnit 5 tests)
  - **Acceptance**: Each SDK can submit a tool call, poll approval, execute

- [x] **(4) Connector marketplace / breadth + discovery**
  - **Scope**: Registry endpoint, 6 new built-in connectors, connector SDK docs
  - **Files**: `pkg/connectors/builtins/`, `pkg/connectors/registry.go`, `docs/CONNECTORS.md`, `CONTRIBUTING.md`
  - **Connectors**: GitHub, AWS, ServiceNow, Email, Postgres (read-only), Webhook (with SSRF protection)
  - **Discovery**: GET /v1/connectors on both gateway and console-api
  - **Acceptance**: GET /v1/connectors lists 8 connectors (2 remote + 6 builtin) with actions

## Tier 2 — Retention Gaps (minimum viable)

- [x] **(5) Policy editor / playground**
  - **Scope**: Policy versioning in DB, simulator endpoint via OPA, UI for simulation
  - **Files**: `cmd/console-api/main.go`, `pkg/console/store.go`, `web/console/src/pages/Policies.tsx`
  - **Endpoints**: POST /admin/policy/simulate, GET/POST /admin/policy/versions
  - **Acceptance**: Submit simulation → see policy decision + matched rules

- [x] **(6) Reporting / compliance exports**
  - **Scope**: CSV export, evidence bundle JSON, verify CLI
  - **Files**: `cmd/console-api/main.go`, `cmd/verify/main.go`, `pkg/console/store.go`
  - **Endpoints**: GET /admin/events/export/csv, GET /admin/reports/export/bundle
  - **CLI**: `go run ./cmd/verify --bundle <file>` verifies bundle structure
  - **Acceptance**: Generate tenant activity report → download CSV + verified bundle

- [x] **(7) Alerting / anomaly detection**
  - **Scope**: Alert rules (4 types) + alert events stored in DB, API + UI
  - **Files**: `migrations/001_initial.sql`, `pkg/console/store.go`, `cmd/console-api/main.go`, `web/console/src/pages/Alerts.tsx`
  - **Rule types**: deny_spike, approve_backlog, unusual_tool, volume_spike
  - **Acceptance**: Create alert rule → see in UI → (worker evaluates and triggers — scaffold ready)

- [x] **(8) Session / conversation context**
  - **Scope**: Sessions table, timeline API, session-scoped event listing, UI
  - **Files**: `migrations/001_initial.sql`, `pkg/console/store.go`, `web/console/src/pages/Sessions.tsx`, `web/console/src/pages/SessionTimeline.tsx`
  - **Endpoints**: GET /admin/sessions, GET /admin/sessions/{id}/timeline
  - **Acceptance**: Click session → see ordered tool calls with decisions end-to-end

## Tier 3 — Differentiators (scaffold + thin vertical)

- [x] **(LLM summaries)** — Audited + refactored + HF service scaffolded
  - **Scope**: SummaryProvider interface, TemplateSummaryProvider (default), LLMSummaryProvider (optional)
  - **Files**: `pkg/approvals/summary.go`, `cmd/llm-summarizer/`
  - **Status**: Existing Summarizer interface is clean. Enhanced with SummaryProvider that supports both template and LLM modes. FastAPI service with optional HF model + hash-based caching.

- [x] **(9) LLM-powered risk scoring** (scaffold)
  - **Scope**: RiskScorer interface, PassthroughScorer, RulesBasedScorer
  - **Files**: `pkg/risk/scorer.go`

- [x] **(10) Multi-framework integrations** (scaffold)
  - **Scope**: LangChain wrapper (Python), MCP server stub (TypeScript)
  - **Files**: `sdk/python/openclause/langchain.py`, `sdk/typescript/src/mcp.ts`

- [x] **(11) Cost / budget controls** (scaffold)
  - **Scope**: UsageCounter type, IncrementUsageCounter, GetUsageCounters in store
  - **Files**: `pkg/console/store.go`, `migrations/001_initial.sql` (usage_counters table)

- [x] **(12) Sandbox / dry-run per policy** (scaffold)
  - **Scope**: ShadowEvaluator for running enforced + shadow policies in parallel
  - **Files**: `pkg/policy/shadow.go`

---

## Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| UI stack | Vite + React + TypeScript | Fast dev, single-page app, broad ecosystem |
| Console API | Separate `cmd/console-api` on :8090 | Separation of concerns, different auth model |
| Console auth | JWT HS256 + bcrypt passwords | Simple, secure, no external IdP needed for MVP |
| API key storage | SHA-256 hash + 8-char prefix for lookup | Constant-time safe, prefix enables indexed DB lookups |
| RBAC model | platform_admin / tenant_admin / approver / viewer | Minimal viable roles covering all use cases |
| SDK languages | Python, TypeScript, Java | Top 3 agent framework languages |
| Built-in connectors | In-process Go functions | Zero latency, no deployment needed for demos |
| Connector discovery | GET /v1/connectors | Single endpoint for remote + builtin connectors |
| Gateway key auth | CompositeKeyStore (env + DB) | Backward-compatible, allows migration to DB-only |
| LLM summary | SummaryProvider interface | Template default, LLM opt-in behind flag |
| Risk scoring | RiskScorer interface | Passthrough default, rules-based or LLM opt-in |
| Shadow mode | ShadowEvaluator | Parallel policy eval, divergence logging |

## Implementation Log

| Date | Item | Notes |
|------|------|-------|
| 2026-03-02 | Phase 0: tracker + branch | Branch: feat/product-gap-v1 |
| 2026-03-02 | Schema update | Added api_keys, users, user_roles, sessions, alert_rules, alert_events, usage_counters, policy_versions (enhanced) |
| 2026-03-02 | Console API | cmd/console-api with JWT auth, RBAC, full CRUD |
| 2026-03-02 | DB-backed auth | pkg/auth/dbkeys.go, CompositeKeyStore, gateway updated |
| 2026-03-02 | Console UI | web/console React SPA with 12 pages |
| 2026-03-02 | SDKs | Python, TypeScript, Java with tests and quickstarts |
| 2026-03-02 | Connectors | 6 built-in connectors + registry endpoint + docs |
| 2026-03-02 | Tier 2 | Policy simulator, CSV/bundle export, verify CLI, alerts, sessions |
| 2026-03-02 | Tier 3 | LLM summary refactor, risk scoring, framework integrations, cost controls, shadow mode |
| 2026-03-02 | Tests | go test -race ./... passes, JWT tests, store tests, composite auth tests |

## Demo Steps (docker-compose)

```bash
cd deploy
docker-compose up -d

# 1. Login to console
curl -X POST http://localhost:8090/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@openclause.dev","password":"admin123"}'
# → {"token":"eyJ...","user":{...}}

# 2. Create tenant + agent + key (use token from step 1)
TOKEN="eyJ..."
curl -X POST http://localhost:8090/admin/tenants \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Demo Corp"}'

curl -X POST http://localhost:8090/admin/tenants/<tenant_id>/agents \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Demo Agent"}'

curl -X POST http://localhost:8090/admin/tenants/<tenant_id>/apikeys \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Demo Key"}'
# → {"raw_key":"sk-oc-..."}  (save this!)

# 3. Submit a tool call via gateway
curl -X POST http://localhost:8080/v1/toolcalls \
  -H "X-API-Key: sk-oc-..." \
  -H 'Content-Type: application/json' \
  -d '{
    "tenant_id":"<tenant_id>","agent_id":"<agent_id>",
    "tool":"github","action":"issue.create",
    "params":{"title":"Test issue","body":"Hello"},
    "risk_score":8,"idempotency_key":"demo-1"
  }'
# → decision: "approve" (risk >= 7)

# 4. Approve from console
curl -X POST http://localhost:8090/admin/approvals/<id>/approve \
  -H "Authorization: Bearer $TOKEN"

# 5. Execute
curl -X POST http://localhost:8080/v1/toolcalls/<event_id>/execute \
  -H "X-API-Key: sk-oc-..."

# 6. See audit trail
curl http://localhost:8090/admin/events \
  -H "Authorization: Bearer $TOKEN"

# 7. Export CSV
curl http://localhost:8090/admin/events/export/csv \
  -H "Authorization: Bearer $TOKEN" -o events.csv

# 8. Export bundle
curl http://localhost:8090/admin/reports/export/bundle?tenant_id=<id> \
  -H "Authorization: Bearer $TOKEN" -o bundle.json

# 9. Verify bundle
go run ./cmd/verify --bundle bundle.json

# Or use the web console at http://localhost:3000
```
