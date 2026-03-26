# cURL Test Matrix

This document is the exhaustive cURL test checklist for the current HTTP surface in this repo.

It is exhaustive by shipped endpoint family and bug-prone scenario class. It is not an infinite combinatorial matrix of every possible payload permutation.

Use it to turn manual bug sweeps into repeatable smoke and regression checks.

Repo smoke helpers that now cover the highest-value live checks:

- `bash ./scripts/curl-smoke-onboarding.sh`
  - self-discovers the preview tenant
  - creates a fresh smoke tenant + agent
  - validates preview, create, bundle archive, live toolcall, saved integration, revisions, saved bundle, regenerate, regenerate-defaults, events, sessions, and analytics
- `bash ./scripts/curl-smoke-console-backend.sh`
  - self-discovers or bootstraps a usable tenant + onboarded agent
  - validates events, analytics, saved integration, and revisions from the host
- `bash ./scripts/backend-smoke-incontainer.sh`
  - resolves the same usable tenant + onboarded agent from the host, then verifies console-api from inside the compose network

## P0 Smoke Set

Run these first on every stack:

1. `GET /healthz` and `GET /readyz` for gateway and console-api
2. `POST /auth/login`
3. `GET /admin/tenants`
4. `POST /admin/onboarding/integrations`
5. `POST /v1/toolcalls` allow path
6. `POST /v1/toolcalls` approve path
7. `POST /admin/approvals/{id}/approve`
8. `POST /v1/toolcalls/{event_id}/execute`
9. `GET /admin/events`
10. `GET /admin/sessions/{session_id}/timeline`

For the current repo, the fastest repeatable way to cover most of that set is:

1. `bash ./scripts/curl-smoke-onboarding.sh`
2. `bash ./scripts/curl-smoke-console-backend.sh`
3. `bash ./scripts/backend-smoke-incontainer.sh`

## 1. Service Health And Discovery

### Gateway

- `GET /healthz`
  - healthy process
- `GET /readyz`
  - database reachable
  - database unavailable
- `GET /v1/connectors`
  - connector catalog visible before any onboarding

### Console API

- `GET /healthz`
  - healthy process
- `GET /readyz`
  - database reachable
  - database unavailable

### Approvals

- `GET /healthz`
  - healthy process

### Remote connectors

- `GET /healthz` on connector-slack
- `GET /healthz` on connector-jira
- `POST /exec`
  - valid internal token + supported action
  - missing internal token
  - bad internal token
  - unsupported action

### Metrics

- `GET /metrics` on the internal metrics listener
  - scrape works

## 2. First-Run Setup And Public Auth

- `GET /setup/status`
  - uninitialized system
  - initialized system
- `POST /setup/initialize`
  - happy path on empty DB
  - repeat call after setup already complete
  - invalid payload
- `POST /auth/login`
  - happy path
  - bad password
  - unknown user
  - disabled user if applicable
- `POST /auth/logout`
  - happy path with JWT
  - missing JWT
- `POST /auth/invite/accept`
  - happy path
  - expired token
  - invalid token
  - reused token
- `POST /auth/reset/request`
  - existing email
  - unknown email
  - malformed payload
- `POST /auth/reset/confirm`
  - happy path
  - expired token
  - invalid token
  - weak password / invalid payload

## 3. Gateway Tool-Call Lifecycle

- `POST /v1/toolcalls`
  - allow path
  - deny path
  - approve path
  - missing API key
  - invalid API key
  - expired API key
  - disabled tenant
  - missing `tenant_id`
  - missing `agent_id`
  - missing `tool`
  - missing `action`
  - missing `idempotency_key`
  - invalid `risk_score`
  - invalid `schema_version`
  - invalid tool/action format
  - oversized `params`
  - oversized `resource`
  - oversized labels map
  - idempotent replay of allow path
  - idempotent replay of approve path
  - tenant/key mismatch if enforced
  - connector execution failure on allow
  - connector unavailable on allow
- `GET /v1/toolcalls/{event_id}`
  - existing event
  - missing event
  - unauthorized
- `POST /v1/toolcalls/{event_id}/execute`
  - happy path after approval
  - execute before approval granted
  - execute for deny event
  - execute for allow event that does not need approval
  - missing event
  - unauthorized

## 4. Internal Approvals Service

- `POST /v1/approvals/requests`
  - happy path
  - missing internal token
  - bad internal token
  - malformed payload
- `GET /v1/approvals/requests/{id}`
  - existing request
  - missing request
  - unauthorized
- `POST /v1/approvals/requests/{id}/approve`
  - happy path
  - already resolved request
  - missing request
  - unauthorized
- `POST /v1/approvals/requests/{id}/deny`
  - happy path
  - already resolved request
  - missing request
  - unauthorized
- `GET /v1/approvals/pending`
  - default listing
  - `tenant_id` filter
  - pagination with `limit` / `offset`
  - unauthorized
- `GET /ui/pending`
  - internal HTML renders
  - `tenant_id` filter
  - unauthorized
- `POST /v1/integrations/slack/interactions`
  - valid Slack signature
  - invalid signature
  - malformed action payload

## 5. Console Onboarding Lifecycle

- `POST /admin/onboarding/integrations`
  - create against existing tenant
  - create with inline new tenant
  - missing runtime
  - missing agent name
  - missing tools
  - invalid tool/action pair
  - unsupported runtime
  - tenant-scoped permission denial
  - platform-admin happy path
- `POST /admin/onboarding/bundles/preview`
  - happy path existing tenant
  - missing tenant
  - inline new tenant rejected
  - invalid tools
  - unsupported runtime
- `POST /admin/onboarding/bundles/regenerate`
  - happy path with active key reference
  - happy path with no active key
  - missing tenant
  - missing agent
  - unsupported runtime
  - invalid tools
- `POST /admin/onboarding/bundles/regenerate-defaults`
  - happy path using saved onboarding metadata
  - happy path using curated fallback defaults
  - no curated defaults available
  - missing tenant
  - missing agent
  - no active key present
- `POST /admin/onboarding/bundles/archive`
  - happy path with writable artifacts
  - invalid bundle payload
  - empty artifact set
  - auth failure
- `GET /admin/tenants/{tenant_id}/agents/{agent_id}/integration`
  - happy path saved integration snapshot
  - missing tenant
  - missing agent
  - forbidden tenant
  - no saved integration for that agent
- `GET /admin/tenants/{tenant_id}/agents/{agent_id}/integration/revisions`
  - happy path recent revision list
  - limit query handling
  - forbidden tenant
  - no revisions yet
- `GET /admin/tenants/{tenant_id}/agents/{agent_id}/integration/bundle`
  - happy path saved bundle rebuild
  - happy path defaults bundle rebuild
  - happy path archive zip
  - no active key omission behavior
  - saved integration missing
  - saved tool/runtime no longer valid

## 6. Tenants, Agents, And API Keys

- `POST /admin/tenants`
  - happy path
  - duplicate name if constrained
  - invalid payload
  - tenant-admin forbidden
- `GET /admin/tenants`
  - platform-admin list
  - tenant-admin scoped list
- `GET /admin/tenants/{tenant_id}`
  - existing tenant
  - missing tenant
  - forbidden tenant
- `POST /admin/tenants/{tenant_id}/status`
  - disable tenant
  - re-enable tenant
  - invalid status
  - forbidden caller
- `POST /admin/tenants/{tenant_id}/agents`
  - happy path
  - duplicate name/id behavior
  - malformed payload
  - forbidden tenant
- `GET /admin/tenants/{tenant_id}/agents`
  - active-only list
  - include disabled list
  - forbidden tenant
- `POST /admin/tenants/{tenant_id}/apikeys`
  - happy path
  - invalid name/payload
  - forbidden tenant
- `GET /admin/tenants/{tenant_id}/apikeys`
  - existing keys
  - empty list
  - forbidden tenant
- `POST /admin/tenants/{tenant_id}/apikeys/{key_id}/revoke`
  - happy path
  - missing key
  - already revoked key
- `POST /admin/tenants/{tenant_id}/apikeys/rotate`
  - happy path make primary
  - happy path revoke old primary
  - invalid expiry
  - malformed payload

## 6a. Local Bridge Alpha

Run these against the locally started bridge from `openclause bridge start --config ./openclause-bridge.yaml`.

- `GET /healthz`
  - healthy bridge process
- `GET /v1/bridge/profiles`
  - default profile visible
  - multi-profile config visible
  - profile metadata does not leak raw API keys
- `GET /v1/bridge/tools`
  - configured tool list is visible
  - selected tenant / agent context is visible
  - profile override via `X-OpenClause-Profile`
- `GET /v1/models`
  - proxied model list works when `openai` bridge config is present
- `POST /v1/chat/completions`
  - happy path with a model-generated governed tool call
  - happy path with no tool call required
  - happy path with `stream=true` returning governed SSE chunks through the tool loop
  - reject client-provided `tools`
  - reject when openai config is missing
- `POST /mcp`
  - `tools/list` JSON-RPC over HTTP
  - `tools/call` happy path for a configured governed tool
  - `tools/call` unknown tool rejection
  - `initialize` returns `Mcp-Session-Id`
  - later requests reuse `Mcp-Session-Id`
- `DELETE /mcp`
  - closes an existing MCP HTTP session
- `openclause bridge mcp --config ./openclause-bridge.yaml`
  - `initialize`
  - `tools/list`
  - `tools/call`
- `POST /v1/toolcalls`
  - happy path with omitted `tenant_id` / `agent_id`
  - happy path with omitted `session_id` / `trace_id` / `idempotency_key`
  - configured tool allowlist rejection
  - tenant mismatch rejection
  - agent mismatch rejection
  - configured risk-score injection
- `GET /v1/toolcalls/{event_id}`
  - upstream event replay through the bridge
- `POST /v1/toolcalls/{event_id}/execute`
  - happy execute after approval
  - upstream missing-event behavior

## 7. Notification Routing And Approvers

- `GET /admin/tenants/{tenant_id}/notification-config`
  - existing config
  - empty/default config
  - forbidden tenant
- `PUT /admin/tenants/{tenant_id}/notification-config`
  - slack-only route
  - webhook-only route
  - mixed routes
  - invalid webhook URL
  - missing webhook secret
  - missing slack channel
  - forbidden tenant
- `GET /admin/tenants/{tenant_id}/approvers`
  - list existing approvers
  - empty list
  - forbidden tenant
- `POST /admin/tenants/{tenant_id}/approvers`
  - happy path create/upsert
  - invalid payload
  - forbidden tenant
- `DELETE /admin/tenants/{tenant_id}/approvers/{user_id}`
  - happy path
  - missing approver
  - forbidden tenant

## 8. Admin Approval Queue

- `GET /admin/approvals/pending`
  - full queue
  - `tenant_id` filter
  - empty queue
  - forbidden tenant
- `POST /admin/approvals/{id}/approve`
  - happy path
  - already approved request
  - already denied request
  - missing request
  - non-approver forbidden
- `POST /admin/approvals/{id}/deny`
  - happy path
  - already resolved request
  - missing request
  - non-approver forbidden

## 9. Events, Reports, And Sessions

- `GET /admin/events`
  - unfiltered list
  - `tenant_id` filter
  - `agent_id` filter
  - `session_id` filter
  - `trace_id` filter
  - decision filter
  - tool/action filters
  - risk range filters
  - malformed time filters
- `GET /admin/events/{event_id}`
  - existing event
  - missing event
  - forbidden tenant
- `GET /admin/events/export/csv`
  - happy path
  - missing tenant for platform-admin
  - invalid range
- `GET /admin/reports/export/bundle`
  - happy path
  - missing tenant for platform-admin
  - invalid range
  - too-large range rejection
- `GET /admin/reports/activity`
  - alias behavior matches `/admin/events`
- `GET /admin/reports/export/csv`
  - alias behavior matches `/admin/events/export/csv`
- `GET /admin/sessions`
  - unfiltered list
  - `tenant_id` filter
  - `agent_id` filter
  - `session_id` filter
  - empty result
- `GET /admin/sessions/{session_id}`
  - happy path
  - missing session
  - ambiguous platform-admin lookup
- `GET /admin/sessions/{session_id}/timeline`
  - happy path
  - missing session
  - ambiguous platform-admin lookup
- `GET /admin/sessions/{session_id}/export/csv`
  - happy path
  - missing session
- `GET /admin/sessions/{session_id}/export/json`
  - happy path
  - missing session

## 10. Analytics

- `GET /admin/analytics/overview`
  - happy path
  - tenant-scoped role
  - unauthorized
- `GET /admin/analytics/timeseries`
  - happy path
  - custom range/bucket
  - malformed params
- `GET /admin/tenants/{tenant_id}/analytics/summary`
  - happy path
  - empty tenant with stable JSON shape
  - custom range/bucket/top-agents
  - malformed params
  - forbidden tenant

## 11. Policies

- `GET /admin/policy/versions`
  - list versions
  - tenant-scoped view
- `POST /admin/policy/versions`
  - happy path
  - malformed payload
  - forbidden caller
- `POST /admin/policy/simulate`
  - allow simulation
  - deny simulation
  - approve simulation
  - invalid payload
- `GET /admin/tenants/{tenant_id}/policy/config`
  - existing config
  - default config
  - forbidden tenant
- `PUT /admin/tenants/{tenant_id}/policy/config`
  - happy path
  - invalid thresholds
  - malformed allowlists
  - forbidden tenant
- `GET /admin/tenants/{tenant_id}/policy/versions`
  - list snapshots
  - empty list
- `POST /admin/tenants/{tenant_id}/policy/versions`
  - create snapshot
  - malformed payload
- `POST /admin/tenants/{tenant_id}/policy/versions/{version_id}/rollback`
  - happy path
  - missing version
  - forbidden tenant
- `POST /admin/tenants/{tenant_id}/policy/simulate`
  - allow/deny/approve simulation
  - malformed payload

## 12. Alerts

- `GET /admin/tenants/{tenant_id}/alerts/rules`
  - list rules
  - empty list
- `POST /admin/tenants/{tenant_id}/alerts/rules`
  - happy path
  - invalid config
  - malformed payload
- `PUT /admin/tenants/{tenant_id}/alerts/rules/{rule_id}`
  - happy path
  - missing rule
  - invalid config
- `DELETE /admin/tenants/{tenant_id}/alerts/rules/{rule_id}`
  - happy path
  - missing rule
- `GET /admin/tenants/{tenant_id}/alerts/events`
  - list events
  - `limit` behavior
  - empty list
- `GET /admin/alerts/rules`
  - global/scoped list
- `POST /admin/alerts/rules`
  - global/scoped create
- `GET /admin/alerts/events`
  - global/scoped events list

## 13. Users, Roles, Invites, And Auth Sessions

- `GET /admin/users`
  - platform-admin list
  - tenant-admin scoped list
- `POST /admin/users`
  - happy path
  - duplicate email
  - invalid payload
  - tenant-admin scope restrictions
- `POST /admin/users/{id}/roles`
  - happy path
  - duplicate role
  - invalid role
  - forbidden scope
- `DELETE /admin/users/{id}/roles/{role_id}`
  - happy path
  - missing role assignment
  - forbidden scope
- `POST /admin/invites`
  - happy path
  - invalid role/tenant combination
  - duplicate invite behavior
  - SMTP configured and delivery status
- `GET /admin/invites`
  - list pending invites
  - tenant-scoped list
- `GET /admin/auth-sessions`
  - list sessions
  - `user_id` filter
  - tenant-scoped view
- `POST /admin/auth-sessions/{session_id}/revoke`
  - happy path
  - missing session
  - forbidden scope

## 14. Connectors Catalog

- `GET /admin/connectors`
  - happy path
  - empty registry behavior
  - malformed upstream connector payload handling if proxied
- `GET /v1/connectors` on console-api
  - legacy alias behavior

## 15. High-Value Negative Tests

These are worth automating early because they catch subtle regressions fast:

- regenerate bundle with no active API key must omit `api_key`
- preview bundle must emit synthetic `agent.created_at`
- defaulted regenerate must fail clearly when no curated defaults exist
- archive endpoint must return only writable artifacts
- event export bundle must reject overly large ranges
- session detail must return ambiguity hints for platform-admin cross-tenant collisions
- approvals filtering must not leak stale cross-tenant state in the UI after the API filter changes
- onboarding create must persist metadata so regenerate starts from the saved runtime/tool/posture later

## Suggested Next Step

The first repo-owned live smoke script now exists:

- `scripts/curl-smoke-onboarding.sh`
  - logs in as the platform admin
  - runs preview and create
  - downloads a direct bundle archive
  - submits a real governed `postgres.query.readonly` call through the gateway
  - verifies saved integration, revision history, saved bundle rebuild, archive rebuild, regenerate, regenerate-defaults, events, sessions, and analytics
  - asserts host-reachable onboarding bundles use `http://localhost:8080` in the local Docker stack

Keep expanding the repo-owned smoke script set:

- `scripts/curl-smoke-gateway.sh`
- `scripts/curl-smoke-console-auth.sh`
- `scripts/curl-smoke-operator.sh`

That gives you a stable manual + CI-ready layer without inventing new product architecture.
