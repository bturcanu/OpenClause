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
  - Commit links: `TBD`
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
- [ ] Extend invites to tenant_admin signup + API key generation by tenant_admin
  - Scope: invite acceptance rules + key issuance permissions
  - Files/services: console-api + console UI
  - API changes: maybe new endpoints or extended invite behavior
  - UI changes: onboarding updates
  - Acceptance criteria: tenant_admin can self-join + generate API keys for own tenant
  - Commit links: ``

### (7) Notification configuration UI persistence
- [ ] Persist per-tenant notification config + surface in console UI
  - Scope: DB storage + minimal UI + allow OPA inputs to read config
  - Files/services: console store + alerts/notifier-related code
  - API changes: new CRUD for config if needed
  - UI changes: notification config screen
  - Acceptance criteria: tenant can update config; OPA can read it in policy input
  - Commit links: ``

### (8) Complete alert system
- [ ] Ensure alert rules persisted + evaluation worker skeleton exists
  - Scope: minimal worker + deny spike rule hook (if easy)
  - Files/services: alert persistence + worker/cron
  - API changes: None required
  - UI changes: alert rule editor/preview improvements if needed
  - Acceptance criteria: rules persist; worker skeleton runs and emits alert events (even if minimal)
  - Commit links: ``

## Tier 3 — Polish (9 → 12)

### (9) Dashboard & analytics improvements
- [ ] Trend charts, risk heatmap, per-agent breakdown, onboarding checklist
  - Scope: UI charts + API fields as needed
  - Acceptance criteria: dashboard renders without regressions; data matches backend
  - Commit links: ``

### (10) API key lifecycle UX
- [ ] Rotation workflow + last-used + expiry warnings
  - Scope: UI + backend endpoints if needed
  - Acceptance criteria: users can rotate and see last-used/expiry info
  - Commit links: ``

### (11) Policy authoring UX
- [ ] Rule builder with diffs + rollback UI
  - Scope: simulator and policy version management UI updates
  - Acceptance criteria: create/edit/rollback works end-to-end
  - Commit links: ``

### (12) Helm charts for console services
- [ ] Add helm charts for console-api and console-ui
  - Scope: chart templates, values
  - Acceptance criteria: chart deploys locally and runs UI/API
  - Commit links: ``

