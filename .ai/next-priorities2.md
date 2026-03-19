# Approver Management + UX Roadmap

This document tracks implementation work for OpenClause “Approver Management + UX Roadmap”.

## Tier 1 — Adoption-critical (1 → 4)

### (1) Database-backed approver management with Console UI
- [x] Implement approvers using existing `users` + `user_roles` (role=`approver`) with tenant scoping
  - Scope: DB model + approvals authorization + console API + Tenant Detail UI tab
  - Files/services: `migrations/001_initial.sql`, `pkg/approvals/authorizer.go`, `pkg/console/store.go`, `cmd/console-api/main.go`, `web/console/src/pages/TenantDetail.tsx`, approvals Slack handler/identity mapping
  - API changes:
    - `GET /admin/tenants/{tenant_id}/approvers`
    - `POST /admin/tenants/{tenant_id}/approvers`
    - `DELETE /admin/tenants/{tenant_id}/approvers/{user_id}`
  - UI changes:
    - Add “Approvers” tab: list (email/name/slack_user_id), add approver (email + optional slack id), remove approver
    - Show allowlist source warning banner (DB-only by default)
  - Acceptance criteria:
    - Allowed DB approver can approve/deny immediately (no restart)
    - Non-allowed gets 403
    - Tenant A approver cannot approve tenant B
    - Invalid email/slack formats return 400; unknown tenant returns 404; duplicates return 409
  - Commit links: `TBD (not committed in this session)`

### (2) User management API & UI
- [x] Implement user CRUD + role assignment + invite + password reset + RBAC enforcement
  - Scope: console-api endpoints + Users UI + minimal security correctness (tenant isolation)
  - Files/services: `pkg/console/store.go`, `cmd/console-api/main.go`, `web/console` pages/components, auth/jwt usage
  - API changes:
    - `GET /admin/users`
    - `POST /admin/users`
    - `POST /admin/users/{id}/roles`
    - `DELETE /admin/users/{id}/roles/{role_id}`
    - `POST /admin/invites`
    - `GET /admin/invites`
    - `POST /auth/invite/accept`
    - `POST /auth/reset/request`
    - `POST /auth/reset/confirm`
  - UI changes:
    - Users page (list, create/invite, assign/remove roles)
    - Invite-accept flow UI (token-based)
    - Basic password reset UI
  - Acceptance criteria:
    - tenant_admin cannot assign roles in other tenants
    - approver cannot create tenants/users
    - invite accept creates user + assigns role + tenant scope
    - reset request/confirm works and tokens expire (or are invalid)
  - Commit links: `TBD (not committed in this session)`
  - Notes:
    - Fix: `ConsumeInviteAccept` populates `User.CreatedAt` from DB `RETURNING created_at` when creating new users.
    - Fix: `handleCreateInvite` now persists optional invite `name` from client requests (previously silently ignored).

### (3) First-run / onboarding wizard
- [x] Add setup endpoints + frontend onboarding wizard + strong secret helper scripts
  - Scope: console initialization bootstrap, disabled-after-init, onboarding flow
  - Files/services: `cmd/console-api/main.go`, `pkg/console/store.go` (or new setup store), `web/console` routes/components, `scripts/*`
  - API changes:
    - `GET /setup/status`
    - `POST /setup/initialize`
  - UI changes:
    - On console-ui startup call `/setup/status`; show wizard if not initialized; else route to login
  - Acceptance criteria:
    - `/setup/initialize` only works when DB has zero users
    - after initialization, endpoint returns 409/403 and cannot run again
    - created records correct: platform_admin user + first tenant + role assignment
  - Commit links: `TBD (not committed in this session)`

### (4) Cross-platform developer experience (Windows + Mac)
- [x] Add scripts and update docs/Makefile for Windows-friendly developer workflow
  - Scope: scripts + docs only; preserve existing dev commands where possible
  - Files/services: `scripts/dev.*`, `scripts/migrate.*`, `scripts/seed-dev.*`, `Makefile`, `docs/LOCAL_TESTING.md`, README
  - API changes: None
  - UI changes: None
  - Acceptance criteria:
    - Windows quickstart works using PowerShell commands
    - onboarding wizard replaces manual seed SQL for first run
  - Commit links: `TBD (not committed in this session)`
  - Notes:
    - `scripts/dev.*` now ensures `CONSOLE_JWT_SECRET` is set/seeded into `.env` (explicit warning) and passes `--env-file` to `docker compose` so compose interpolation uses the correct env file.

## Tier 2 — Production readiness scaffolding (5 → 8)

### (5) SSO/OIDC abstraction
- [x] Introduce auth provider abstraction layer
  - Scope: add `AuthProvider` seam + email/password provider; wire `/auth/login` to dispatch via provider (future OIDC-ready wiring)
  - Files/services: `cmd/console-api/main.go`, `cmd/console-api/auth_provider.go`, `cmd/console-api/auth_provider_test.go`
  - API changes: None (`POST /auth/login` response contract preserved: `{token, user{ id,email,name,roles }}`)
  - UI changes: None
  - Acceptance criteria:
    - Current `/auth/login` flow works unchanged (including tenant-scope fail-closed behavior)
    - Codebase has clean seams for adding OIDC later (provider interface + factory wiring; login handler no longer coupled to credential verification/token generation)
  - Commit links: `7c25c8f`
  - Verification notes:
    - `go test ./... -count=1` (PASS)
    - `go test -race ./... -count=1` (PASS)
    - `PATH="$PWD:$PATH" make policy-test` (PASS; local `opa` binary downloaded since `opa` was not present in PATH)
    - `web/console`: `npm install && npm run build` (PASS)
    - `sdk/typescript`: `npm install && npm run build` (PASS)
    - Curl smoke (localhost):
      - `allow decision=allow event_id=b652041b-dae8-4249-b031-0df2c8316fbc`
      - `deny decision=deny event_id=eb101770-7410-4944-bdcf-5229374530dc`
      - `approve decision=approve event_id=28d80561-7692-411e-a500-6f1a0f62b2c3`
      - Audit trail: verified returned JSON contains matching `event_id`

### (6) Self-service tenant onboarding via invites
- [x] Extend invites to tenant_admin signup + API key generation by tenant_admin
  - Scope: enrich `/auth/invite/accept` response with tenant context; UI onboarding next-steps + deep-link to tenant API Keys tab; add RBAC unit tests for tenant-scoped API key issuance
  - Files/services:
    - `pkg/console/store.go` (invite accept result payload)
    - `cmd/console-api/main.go` (invite accept response JSON)
    - `cmd/console-api/apikeys_rbac_test.go` (tenant_admin RBAC boundary tests)
    - `web/console/src/pages/InviteAccept.tsx` (show tenant context/next steps for tenant_admin)
    - `web/console/src/pages/TenantDetail.tsx` (support `?tab=api_keys` deep link)
  - API changes: `/auth/invite/accept` now returns `{ status, user, tenant_id, role }` (existing `status` + `user` preserved)
  - UI changes: tenant_admin invite acceptance shows next steps + link to `/tenants/:id?tab=api_keys`
  - Acceptance criteria:
    - tenant_admin can accept an invite for `tenant_id` and immediately create API keys for that tenant
    - tenant_admin cannot manage other tenants' API keys (verified via RBAC tests)
  - Commit links: `021a79d`
  - Verification notes:
    - `go test ./... -count=1` (PASS)
    - `go test -race ./... -count=1` (PASS)
    - `PATH="$PWD:$PATH" make policy-test` (PASS)
    - `web/console`: `npm install && npm run build` (PASS)
    - `sdk/typescript`: `npm install && npm run build` (PASS)
    - Curl smoke against updated servers (`:18090/:18080`) verified:
      - invite accept payload: `role=tenant_admin`, `tenant_id=tenant1`
      - allow decision: `allow_event_id=3b88f5e2-9178-4fb8-89dc-38c093313c06`
      - deny decision: `deny_event_id=9e85b2c1-6272-404b-a120-bea7e38649c2`
      - approve+execute: `approve_event_id=031bf987-ae15-44c6-b28e-15e9de6ba7c2`

### (7) Notification configuration UI persistence
- [x] Persist per-tenant notification config + surface in console UI
  - Scope: console-api CRUD + notifier routing override in gateway + Tenant Detail “Notification Routing” form
  - Files/services: `pkg/console/store.go`, `cmd/console-api/notification_config.go`, `cmd/console-api/main.go`, `cmd/gateway/main.go`, `web/console/src/pages/TenantDetail.tsx`, `web/console/src/api.ts`, plus new tests in `cmd/console-api/notification_config_test.go` and `cmd/console-api/notification_config_handler_test.go`
  - API changes: `GET/PUT /admin/tenants/{tenant_id}/notification-config` (tenant_admin)
  - UI changes: API Keys tab shows notification routing form and saves via `PUT`
  - Acceptance criteria: tenant_admin updates config; SSRF-protected webhook validation; approvals created afterwards use updated routing (verified via approve+execute after PUT; outbox recipients not directly asserted)
  - Commit links: `5286d3f`
  - Verification notes:
    - `go test ./... -count=1` (PASS)
    - `go test -race ./... -count=1` (PASS)
    - `make policy-test` (PASS; local `opa` symlink)
    - `web/console`: `npm install && npm run build` (PASS)
    - `sdk/typescript`: `npm install && npm run build` (PASS)
    - Curl smoke (alternate fresh ports to avoid stale binaries): `PUT /admin/tenants/tenant1/notification-config` code=200; `GET` returns saved `notify[0].channel`
    - Curl smoke (approve+execute): toolcall decision=`approve` event_id=`9c09cfa3-caf9-4e77-9e51-5f9df8bc46bf`; approval_id=`033a3c46-5047-43b5-bb8f-c6e580109977`; gateway execute_status=200; audit `GET /admin/events/{event_id}` event_id match=true

### (8) Complete alert system
- [x] Ensure alert rules persisted + evaluation worker runs
  - Scope: end-to-end tenant `deny_spike` alert system (CRUD rules + emitted events + notification dispatch) with UI visibility
  - Files/services: `cmd/alert-worker/*`, `cmd/console-api/alerts_handlers.go` + tests, `pkg/alerts/*`, `pkg/console/store.go`, `cmd/console-api/main.go`, `migrations/001_initial.sql`, `web/console/src/pages/TenantDetail.tsx`, `deploy/docker-compose.yml`, plus docs (`README.md`, `docs/LOCAL_TESTING.md`)
  - API changes: tenant-scoped CRUD + event listing:
    - `GET/POST/PUT/DELETE /admin/tenants/{tenant_id}/alerts/rules`
    - `GET /admin/tenants/{tenant_id}/alerts/events`
    - Legacy global routes preserved (`/admin/alerts/*`)
  - UI changes: Tenant Detail “Alerts” tab (create/edit/enable/disable deny_spike rules; list recent alert events)
  - Acceptance criteria: rules persist; worker evaluates denies spike per tenant window; alert events are created and notifications are dispatched via per-tenant routing
  - Commit links: `f429ac6`
  - Verification notes:
    - Go tests: `go test ./... -count=1` (PASS)
    - Go tests (race): `go test -race ./... -count=1` (PASS)
    - Policy tests: `docker run --rm -v \"$PWD/policy\":/policy openpolicyagent/opa:0.62.0 test /policy/bundles/v0 /policy/tests -v` (PASS, 17/17)
    - Web build: `web/console: npm install && npm run build` (PASS)
    - TS SDK build: `sdk/typescript: npm install && npm run build` (PASS)
    - Smoke test (tenant UUID based on `sk-test-key-1`):
      - tenant UUID: `338e57f1-0114-45b6-8c7f-34871c04601c`
      - notification-config PUT: `{"notify":[{"kind":"slack","channel":"alerts"}]}`
      - created rule:
        - `rule_id=3272d815-08e8-46e7-9964-bb9d54442b8f` name=`deny-spike-smoke-20260319192000` config=`n=3,m_minutes=5`
      - generated 3 deny toolcalls (decision=`deny`):
        - tool event_ids: `5ae4df8f-4fa6-482c-a607-73fdfb685ebe`, `a955e30f-049f-4721-a64c-9ab3b52eee18`, `143c9b03-3b50-4ecc-af82-73aea1c22499`
      - waited ~45s, then verified via console-api:
        - `GET /admin/tenants/$TENANT_ID/alerts/events?limit=50`
        - found alert event `alert_event_id=49ee32c4-9806-4559-a003-253cb759f1fd`
        - `status=sent`, `delivered_at=2026-03-19T19:20:05.713147Z`, message includes `deny spike: 3 denies`

## Tier 3 — Polish (9 → 12)

### (9) Dashboard & analytics improvements
- [x] Trend charts, risk heatmap, per-agent breakdown, onboarding checklist
  - Scope: add tenant-scoped analytics summary endpoint + render charts in Tenant Detail “Analytics” tab
  - Files/services:
    - Backend: `pkg/console/store.go`, `cmd/console-api/tenant_analytics_handlers.go`, `cmd/console-api/main.go` (+ tests)
    - Frontend: `web/console/src/pages/TenantDetail.tsx`
    - Docs: `docs/LOCAL_TESTING.md`, `README.md`
  - API changes:
    - `GET /admin/tenants/{tenant_id}/analytics/summary?range=24h&bucket_minutes=60&top_agents=5`
  - UI changes:
    - Tenant Detail -> `Analytics` tab with range selector and:
      - Allow/Deny/Approve trend buckets
      - Risk heatmap (risk_score 0..10)
      - Per-agent breakdown (top agents)
      - Onboarding checklist widget (has_api_key/has_approver/has_toolcall/has_approval/has_execution)
  - Acceptance criteria:
    - Tenant selection drives analytics queries (tenant id comes from `/tenants/:id` route + explicit smoke-selected tenant id)
    - Charts populate without regressions and totals match DB counts for the same 24h window
  - Commit links: `e0d5ac1`
  - Verification notes:
    - `go test ./... -count=1` (PASS)
    - `go test -race ./... -count=1` (PASS)
    - `docker run --rm -v \"$PWD/policy\":/policy openpolicyagent/opa:0.62.0 test /policy/bundles/v0 /policy/tests -v` (PASS; 17/17)
    - `web/console`: `npm install && npm run build` (PASS)
    - `sdk/typescript`: `npm install && npm run build` (PASS)
    - Smoke (curl) for tenant `338e57f1-0114-45b6-8c7f-34871c04601c`:
      - toolcalls:
        - allow `event_id=1ff6e3f8-b5a7-47f0-896e-5d1794840bb7`
        - deny `event_id=751be1e3-6320-433c-8ebb-f2bc85b0337e`
        - approve `event_id=b4427ea9-95af-40c7-8718-09923ed078bb` (approval_id=`fa8149f5-49e5-4120-9d85-845a85c45fe0`)
      - approve+execute:
        - execution `event_id=4546a60f-ce4f-465d-8ff4-7832bc049df1` with `result.status=success` (audit trail verified via `GET /admin/events/{execution_event_id}`)
      - analytics:
        - `analytics/summary` totals: `total_events=37 allow=10 deny=17 approve=10`
        - `onboarding_checklist`: all `true`
        - DB totals (24h) matched: `37|10|17|10`

### (10) API key lifecycle UX
- [x] Rotation workflow + last-used + expiry warnings
  - Scope: extend API key schema + tenant-scoped rotate endpoint + UI workflow/metadata display
  - Files/services:
    - Backend: `migrations/001_initial.sql`, `pkg/console/store.go`, `pkg/auth/dbkeys.go`, `cmd/console-api/main.go`
    - Frontend: `web/console/src/pages/TenantDetail.tsx`
    - Docs: `README.md`, `docs/LOCAL_TESTING.md`
  - API changes:
    - `POST /admin/tenants/{tenant_id}/apikeys/rotate`
    - `POST /admin/tenants/{tenant_id}/apikeys` now accepts optional `expires_at`
    - `GET /admin/tenants/{tenant_id}/apikeys` now includes `expires_at`, `last_used_at`, `is_primary`
  - Acceptance criteria:
    - tenant_admin/platform_admin can rotate key: create new -> mark primary -> revoke old primary
    - key metadata (`last_used_at`, `expires_at`, `is_primary`) is visible in UI and API
    - gateway rejects expired DB-backed keys (`expires_at <= NOW()`)
  - Commit links: `5f00d7c`
  - Verification notes:
    - `go test ./... -count=1` (PASS)
    - policy tests: `docker run --rm -v \"$PWD/policy\":/policy openpolicyagent/opa:0.62.0 test /policy/bundles/v0 /policy/tests -v` (PASS; 17/17)
    - `web/console`: `npm install && npm run build` (PASS)
    - `sdk/typescript`: `npm install && npm run build` (PASS)
    - Docker restart: `docker compose -f deploy/docker-compose.yml up --build -d gateway console-api console-ui` (PASS)
    - Smoke (explicit tenant UUID from `.env` API key mapping):
      - login: token issued for `admin@openclause.dev`
      - selected tenant: `338e57f1-0114-45b6-8c7f-34871c04601c`
      - rotation:
        - old primary key id: `e3cf65cf-1f6e-492e-89e7-69ef7cf7f664`
        - new primary key id: `43326f08-8f6b-4afd-8d49-bae7cc386c99`
        - new key `expires_at=2030-01-01T23:59:59.999999Z`, `is_primary=true`
        - old key revoked (`status=revoked`, `is_primary=false`)
      - toolcalls with rotated key:
        - allow `event_id=796322a3-115f-4823-bfc7-5858d53b4d03`
        - deny `event_id=ccd1c1ef-f019-4816-bad7-adc7d14bff7e`
        - approve `event_id=4516ef5b-dd12-43d9-b88f-1694454d7ae5` (`approval_id=dcb61ee5-dd81-40a0-9985-9705f6da9c08`)
      - approve + execute:
        - execute response `decision=allow`, `result_status=success`
        - execution audit event `event_id=4b06ff2c-7c3d-4008-b70f-249988f72870`, `result_status=success`
      - API key metadata post-use:
        - rotated key `last_used_at=2026-03-19T20:45:23.735883Z` persisted

### (11) Policy authoring UX
- [x] Rule builder with diffs + rollback UI
  - Scope: tenant-scoped policy authoring UX and enforcement wiring:
    - Rule builder knobs (`max_risk_auto_approve`, read/write/destructive allowlists, `require_destructive_approval`)
    - Policy simulator preview using builder config
    - Policy versions list + diff preview + rollback endpoint
    - Gateway enforcement uses saved tenant policy config for subsequent toolcalls
  - Files/services:
    - Backend/API: `cmd/console-api/policy_authoring_handlers.go`, `cmd/console-api/main.go`, `pkg/console/store.go`, `pkg/policy/rulebuilder.go`, `cmd/gateway/main.go`
    - Policy bundle: `policy/bundles/v0/main.rego`
    - Frontend: `web/console/src/pages/Policies.tsx`
    - Docs: `README.md`, `docs/LOCAL_TESTING.md`
  - API changes:
    - `GET/PUT /admin/tenants/{tenant_id}/policy/config`
    - `GET/POST /admin/tenants/{tenant_id}/policy/versions`
    - `POST /admin/tenants/{tenant_id}/policy/versions/{version_id}/rollback`
    - `POST /admin/tenants/{tenant_id}/policy/simulate`
  - UI changes:
    - Policies page now requires explicit tenant selection (platform_admin safe flow)
    - Rule builder form + simulator preview + versions table + JSON diff + rollback action
  - Acceptance criteria:
    - edit -> preview -> save affects subsequent gateway decisions for the selected tenant
    - rollback restores previous behavior and creates a rollback version record
  - Commit links: `6991445`
  - Verification notes:
    - `go test ./... -count=1` (PASS)
    - `docker run --rm -v "$PWD/policy":/policy openpolicyagent/opa:0.62.0 test /policy/bundles/v0 /policy/tests -v` (PASS; 17/17)
    - `web/console`: `npm install && npm run build` (PASS)
    - `sdk/typescript`: `npm install && npm run build` (PASS)
    - Docker restart: `docker compose -f deploy/docker-compose.yml up --build -d opa gateway console-api console-ui` (PASS)
    - Smoke (explicit tenant UUID from `.env` API key mapping):
      - login token issued for `admin@openclause.dev`
      - selected tenant: `338e57f1-0114-45b6-8c7f-34871c04601c`
      - policy behavior change:
        - tighter config (`max_risk_auto_approve=2`) simulate -> `decision=deny`
        - gateway toolcall under tight config: `event_id=04168e89-5cbb-401a-ad79-e3a2f1587c78` `decision=deny`
        - rollback to baseline version, same request: `event_id=4b9d8db3-787e-4bd4-9563-620b8705a7f6` `decision=allow`
      - happy-path toolcalls:
        - allow `event_id=d48e253d-d6c4-49c4-8aa6-9a94a9eec842`
        - deny `event_id=9b19ebba-33cc-4d59-b712-19bfc492cf25`
        - approve `event_id=a11d9e3a-763d-4206-836b-e58650a7c0b5` (`approval_id=e32cb93a-bddf-4494-b66e-84907a43305a`)
      - approve + execute:
        - execution audit event `event_id=58c44b22-d9d7-4d23-842e-f93a35ab8f98`
        - audit trail verified via `GET /admin/events/{execution_event_id}`

### (12) Helm charts for console services
- [ ] Add helm charts for console-api and console-ui
  - Scope: chart templates, values
  - Acceptance criteria: chart deploys locally and runs UI/API
  - Commit links: ``

