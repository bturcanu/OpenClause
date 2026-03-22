# Logic Flow Sweep

Date: 2026-03-20
Branch: `fix/logic-flow-sweep`
Status: Complete

## Assumptions / Contracts (2026-03-21)

- `web/console/src/api.ts`
  - `apiFetch(...)` returns a raw `Response`
  - `api.get/post/put/delete` parse JSON through `readJSONResponse(...)`
  - `api.delete(...)` returns `{}` for `204 No Content`
- Invite/reset URLs
  - `PUBLIC_BASE_URL` is the canonical base for absolute invite/reset links
  - invalid or blank `PUBLIC_BASE_URL` falls back to `http://localhost:3000`
- Invite/reset tokens
  - invite and password-reset tokens are stored as keyed HMAC hashes at rest
  - only `POST /admin/invites` returns the raw invite token once
  - `GET /admin/invites` omits the raw token and shows delivery status only
- Login tenant scoping
  - non-platform users without a single tenant assignment are rejected
  - multi-tenant non-platform users are rejected
  - `platform_admin` ignores tenant-scoped roles and gets an empty tenant claim
- Workspace note
  - this sweep started with pre-existing local edits in `pkg/auth/middleware.go` and `pkg/auth/middleware_test.go`; they were treated as in-progress changes and left untouched

## Follow-up: 2026-03-21 Contracts + Auth / Invite Sweep

Date: 2026-03-21
Branch: `main`
Status: In Progress

### New Findings

| ID | Sev | Flow | Symptom | Root cause | Fix | Files | Status |
|---|---|---|---|---|---|---|---|
| LF-037 | High | Auth / invite accept / password reset confirm | Public token flows could return `400 invalid or expired token` even when the real problem was an internal store/DB failure, which masked server issues as user mistakes | `handleInviteAccept` and `handleResetConfirm` flattened unexpected errors into the same bad-token response used for genuine invalid/expired tokens | Added explicit invalid-token sentinels in `pkg/console/store.go`, returned `500` for unexpected internal failures, kept `400` for real invalid-token cases, and added handler/unit regressions plus invite-sender misconfig tests | `pkg/console/store.go`, `cmd/console-api/main.go`, `cmd/console-api/invite_handlers_test.go`, `cmd/console-api/invite_email_test.go` | Fixed |

### Verification

- Targeted tests
  - `go test ./cmd/console-api ./pkg/console -count=1`
    - Pass
    - Key output: `ok github.com/bturcanu/OpenClause/cmd/console-api`, `ok github.com/bturcanu/OpenClause/pkg/console`
  - `go test ./cmd/console-api -run 'TestHandleInviteAccept|TestResetConfirmErrorStatus|TestNewInviteEmailSenderFromEnv' -count=1`
    - Pass
    - Key output: `ok github.com/bturcanu/OpenClause/cmd/console-api`
- Live auth/invite checks
  - Invite create + list
    - `POST /admin/invites` returned `email_status=logged`, raw `token`, and non-empty `accept_url`
    - `GET /admin/invites` returned the invite metadata with `email_status=logged` and omitted `token`
  - Invite accept + login
    - `POST /auth/invite/accept` returned `status=accepted`, tenant scope, and role for `accept-1774145581@example.com`
    - follow-up `POST /auth/login` for that user returned a JWT and `session_id`
  - Password reset from emailed URL
    - `POST /auth/reset/request` returned `status=ok`
    - dev log emitted `confirm_url=http://localhost:3000/reset?token=...`
    - `POST /auth/reset/confirm` with that token returned `status=reset`
    - follow-up `POST /auth/login` with the new password succeeded
- Contract checks
  - `PUBLIC_BASE_URL` fallback remains covered by `Test_buildConsolePageURL_FallsBackToDefaultBaseURL`
  - SMTP misconfig safe-failure behavior is now covered by `TestNewInviteEmailSenderFromEnv_MisconfiguredSenderReturnsSafeFailure`

## Follow-up: Invite Delivery Hardening

Date: 2026-03-20
Branch: `feature/console-sessions-polish`
Status: Complete

### New Findings

| ID | Sev | Flow | Symptom | Root cause | Fix | Files | Status |
|---|---|---|---|---|---|---|---|
| LF-029 | High | Auth / invites | `POST /admin/invites` created a token but never attempted real email delivery, so operators had no delivery signal beyond dev logs and manual copy/paste | Invite creation stopped after persisting the invite row; there was no sender abstraction, SMTP path, or delivery-status contract | Added a pluggable invite email sender (dev logger + SMTP), absolute `accept_url`, short-timeout delivery attempt, persisted `email_status` / `email_sent_at` / `email_error`, and handler regressions for sent/failed branches | `cmd/console-api/main.go`, `cmd/console-api/invite_email.go`, `cmd/console-api/invite_handlers_test.go`, `pkg/console/store.go`, `migrations/001_initial.sql`, `web/console/src/pages/Users.tsx` | Fixed |
| LF-030 | Medium | Auth / invites | Invite links were only server-relative unless the UI reconstructed them client-side, which made API-created invites awkward to share and harder to email from the backend | Invite/reset URL helpers returned route-only paths and the backend had no public console base URL config | Switched backend invite/reset URL builders to absolute URLs using `PUBLIC_BASE_URL`, surfaced `accept_url` in create-invite responses, and updated docs/config | `cmd/console-api/main.go`, `cmd/console-api/setup_config_test.go`, `.env.example`, `deploy/helm/console-api/values.yaml`, `readme.md`, `docs/LOCAL_TESTING.md` | Fixed |
| LF-031 | Medium | Auth / invites | `GET /admin/invites` looked like it could return a reusable `token`, but invite tokens are HMAC-hashed at rest so list consumers could only ever get a non-usable value | The store still selected the persisted `token` column even after hashing-at-rest was added, so the list shape implied a secret that no longer existed | Stopped exposing raw token fields from list responses and replaced that operator surface with durable delivery status metadata | `pkg/console/store.go`, `cmd/console-api/invite_handlers_test.go`, `readme.md`, `docs/LOCAL_TESTING.md` | Fixed |

### Minimal Repros

- LF-029 before fix:
  - Create an invite with `POST /admin/invites`
  - Result: response only contained `token` + `expires_at`; there was no backend email attempt and no delivery status
  - After: response includes `accept_url`, `email_status`, and `email_error` when needed; SMTP-backed send is attempted with a short timeout
- LF-030 before fix:
  - Call `POST /admin/invites` from the API or inspect dev logs
  - Result: invite accept URLs were route-only (`/invite/accept?...`) unless a browser rebuilt them
  - After: backend returns/logs absolute URLs such as `https://console.example.com/invite/accept?...`
- LF-031 before fix:
  - Call `GET /admin/invites` after creating an invite
  - Result: the `token` field reflected the hashed-at-rest DB value, not a usable invite token
  - After: list responses omit the raw token and instead expose delivery status fields operators can trust

### Files Changed

- `cmd/console-api/main.go`
- `cmd/console-api/invite_email.go`
- `cmd/console-api/invite_handlers_test.go`
- `cmd/console-api/setup_config_test.go`
- `pkg/console/store.go`
- `migrations/001_initial.sql`
- `web/console/src/pages/Users.tsx`
- `.env.example`
- `deploy/helm/console-api/values.yaml`
- `readme.md`
- `docs/LOCAL_TESTING.md`
- `scripts/demo.sh`
- `scripts/e2e-curl-happy-path.sh`

### Verification

- `go test ./cmd/console-api -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/console-api`
- `npm --prefix web/console run build`
  - Pass
  - Key output: `✓ built in 617ms`
- `go test ./... -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/console-api`, `ok github.com/bturcanu/OpenClause/pkg/console`
- `go test -race ./... -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/console-api`, `ok github.com/bturcanu/OpenClause/pkg/console`
- `docker run --rm -v "$PWD/policy:/policy" openpolicyagent/opa:0.62.0 test /policy/bundles/v0 /policy/tests -v`
  - Pass
  - Key output: `PASS: 19/19`
- `npm --prefix sdk/typescript run build`
  - Pass
  - Key output: `tsc`
- `./scripts/dev.sh`
  - Pass
  - Key output: migrations added `email_status`, `email_sent_at`, and `email_last_error` via `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`
- `./scripts/demo.sh`
  - Pass
  - Key output: `Invite link logged for dev/test (SMTP not configured)`

### Follow-up Notes

- This machine did not have SMTP configured, so the live demo exercised the no-op logging sender and returned `email_status=logged`. The SMTP branch is covered by handler tests and environment-driven configuration.

## Follow-up: Sessions Hardening

Date: 2026-03-20
Branch: `feature/console-sessions-polish`
Status: Complete

### New Findings

| ID | Sev | Flow | Symptom | Root cause | Fix | Files | Status |
|---|---|---|---|---|---|---|---|
| LF-015 | High | Sessions timeline | Session detail could show ghost duplicate rows if `tool_results` or `approval_requests` ever contained more than one row for a single `event_id` | `GetSessionTimeline` used direct left joins without limiting related rows | Replaced direct joins with `LEFT JOIN LATERAL (...) ORDER BY ... LIMIT 1` and added a de-duplicating timeline builder regression test | `pkg/console/store.go`, `pkg/console/store_test.go` | Fixed |
| LF-016 | Medium | Sessions platform-admin UX | Pasting a bare `session_id` that existed in multiple tenants produced an opaque `400` with no operator recovery path | `resolveSessionTenant` only returned a sentinel error, and handlers/UI flattened it to a string message | Added a typed ambiguity error carrying tenant candidates, structured JSON handler responses, and a Session detail tenant picker that retries with `tenant_id` | `pkg/console/store.go`, `cmd/console-api/main.go`, `cmd/console-api/session_handlers_test.go`, `web/console/src/api.ts`, `web/console/src/pages/SessionTimeline.tsx`, `web/console/src/pages/Sessions.tsx` | Fixed |
| LF-017 | Low | Sessions/UI polish | The new console theme still had a few readability hazards on sidebar links, badges over striped tables, and code blocks on smaller screens | Palette/spacing polish landed faster than a targeted contrast/overflow pass | Applied minimal CSS-only contrast and overflow fixes | `web/console/src/index.css` | Fixed |
| LF-018 | Low | Demo/docs | The demo proved approvals and exports, but not that Sessions and user attribution worked on a fresh run | `scripts/demo.sh` did not send session/user/trace attribution or check `/admin/sessions` | Added attributed toolcalls, a Sessions confirmation step, and matching README/local-testing guidance | `scripts/demo.sh`, `readme.md`, `docs/LOCAL_TESTING.md` | Fixed |

### Follow-up Verification

- `go test ./pkg/console ./cmd/console-api -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/pkg/console`, `ok github.com/bturcanu/OpenClause/cmd/console-api`
- `npm --prefix web/console run build`
  - Pass
  - Key output: `✓ built in 595ms`
- `go test ./... -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/pkg/console`, `ok github.com/bturcanu/OpenClause/cmd/console-api`
- `go test -race ./... -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/pkg/console`, `ok github.com/bturcanu/OpenClause/cmd/console-api`
- `docker run --rm -v "$PWD/policy:/policy" openpolicyagent/opa:0.62.0 test /policy/bundles/v0 /policy/tests -v`
  - Pass
  - Key output: `PASS: 19/19`
- `npm --prefix sdk/typescript run build`
  - Pass
  - Key output: `tsc`
- `./scripts/dev.sh`
  - Pass
  - Key output: stack rebuilt, migrations applied, health URLs printed
- `./scripts/demo.sh`
  - Pass
  - Key output: session confirmation `Session visible in console API: demo-session-1774058548`
- Live ambiguity smoke
  - Pass
  - Key output: `status=400`, `candidates=["4e511724-16f6-4390-8db5-9bdc51e845cb","76a644cf-2540-452b-a09c-6ebf438c76d9"]`, `message=tenant_id required`

### Follow-up Notes

- Initial direct localhost curl smoke failed inside the sandbox with `curl: (7) Failed to connect to localhost...`; this was an environment restriction, not a repo bug. Rerunning the same smoke outside the sandbox succeeded and produced the expected ambiguity payload.
- Follow-up implementation commit: `d2c80f3` (`fix(console): harden sessions timeline and tenant recovery`)

## Follow-up: Merge-ready Sweep

Date: 2026-03-20
Branch: `feature/console-sessions-polish`
Status: Complete

### New Findings

| ID | Sev | Flow | Symptom | Root cause | Fix | Files | Status |
|---|---|---|---|---|---|---|---|
| LF-019 | Medium | C9/C13/F19 | Replay, event detail, and chain/export queries could still multiply or pick unstable related rows if `tool_results` or `approval_requests` ever drifted from 1:1 per event | Several evidence + console detail queries still used direct joins after the session timeline hardening pass | Replaced the remaining joins with defensive `LEFT JOIN LATERAL (...) ORDER BY ... LIMIT 1` lookups so related rows stay stable and unique | `pkg/evidence/store.go`, `pkg/console/store.go` | Fixed |
| LF-020 | Medium | G21/G23 | Updating or deleting a missing alert rule returned a generic `500`, which looked like a server failure instead of a not-found case | Alert rule store methods did not surface a sentinel not-found error when zero rows were affected | Added `ErrAlertRuleNotFound`, mapped it to `404`, and added handler regressions for update/delete | `pkg/console/store.go`, `cmd/console-api/alerts_handlers.go`, `cmd/console-api/alerts_handlers_test.go` | Fixed |
| LF-021 | Medium | G22/G23 | Pending alert deliveries lost retry timing in the tenant-scoped flow, so the UI could not explain when another delivery attempt would happen | `ListAlertEventsSince` selected `next_attempt_at` in SQL but did not scan it into the response model, and the alerts UIs did not render retry metadata | Restored `next_attempt_at` scanning, surfaced retry/delivery details in Alerts and Tenant Detail, and documented the contract | `pkg/console/store.go`, `web/console/src/pages/Alerts.tsx`, `web/console/src/pages/TenantDetail.tsx`, `readme.md`, `docs/LOCAL_TESTING.md` | Fixed |
| LF-022 | Medium | A1 | A transient `/setup/status` failure dropped operators into the setup wizard, implying the instance was uninitialized when the real problem was connectivity or an upstream error | `App.tsx` treated any setup-status exception as `not_initialized` | Added an explicit setup-check error state with retry so first-run setup only appears when the API confirms it is needed | `web/console/src/App.tsx` | Fixed |
| LF-023 | Low | H24/G23/B6/I27/Journey UX | Several console pages degraded into blank/zero states on partial API failures, hiding the actual problem from operators | `Overview`, `Alerts`, and `TenantDetail` used `.catch(() => [])`-style fallbacks, and Policies had an empty tenant dead-end | Switched to partial-load handling with actionable inline errors, retry affordances, and a friendlier empty state for Policies | `web/console/src/pages/Overview.tsx`, `web/console/src/pages/Alerts.tsx`, `web/console/src/pages/TenantDetail.tsx`, `web/console/src/pages/Policies.tsx`, `web/console/src/index.css` | Fixed |

### Files Changed

- `cmd/console-api/alerts_handlers.go`
- `cmd/console-api/alerts_handlers_test.go`
- `pkg/console/store.go`
- `pkg/evidence/store.go`
- `web/console/src/App.tsx`
- `web/console/src/pages/Overview.tsx`
- `web/console/src/pages/Alerts.tsx`
- `web/console/src/pages/TenantDetail.tsx`
- `web/console/src/pages/Policies.tsx`
- `web/console/src/index.css`
- `readme.md`
- `docs/LOCAL_TESTING.md`

### Verification

- `go test ./cmd/console-api ./pkg/console ./pkg/evidence -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/console-api`, `ok github.com/bturcanu/OpenClause/pkg/console`, `ok github.com/bturcanu/OpenClause/pkg/evidence`
- `npm --prefix web/console run build`
  - Pass
  - Key output: `✓ built in 1.00s`
- `go test ./... -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/console-api`, `ok github.com/bturcanu/OpenClause/pkg/console`, `ok github.com/bturcanu/OpenClause/pkg/evidence`
- `go test -race ./... -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/gateway`, `ok github.com/bturcanu/OpenClause/pkg/console`, `ok github.com/bturcanu/OpenClause/pkg/evidence`
- `docker run --rm -v "$PWD/policy:/policy" openpolicyagent/opa:0.62.0 test /policy/bundles/v0 /policy/tests -v`
  - Pass
  - Key output: `PASS: 19/19`
- `npm --prefix sdk/typescript run build`
  - Pass
  - Key output: `tsc`
- `./scripts/dev.sh`
  - Pass
  - Key output: stack rebuilt, services restarted cleanly, migrations reapplied, health URLs printed
- `./scripts/demo.sh`
  - Pass
  - Key output: session confirmation `Session visible in console API: demo-session-1774060201`, connectors `8 registered`, analytics `4 events in last 24h`

### Follow-up Notes

- No new environment limitations surfaced in this pass. The approved Docker-based OPA run, full Go matrix, web build, dev stack, and live demo all completed successfully on this machine.

## Follow-up: Targeted Bug Sweep

Date: 2026-03-20
Branch: `feature/console-sessions-polish`
Status: Complete

### Candidate Issues

| ID | Sev | Repro | Likely root cause | Priority |
|---|---|---|---|---|
| LF-024 | High | `GET /admin/sessions?...&since=2099-01-01T00:00` still returned the session when using the same `datetime-local` shape the UI emits | Sessions UI sent `datetime-local` strings without timezone, while the API only parsed RFC3339 timestamps, so time filters were silently ignored | Fixed |
| LF-025 | High | Unit repro: login a non-platform user with roles in `tenant-1` and `tenant-2`; JWT scope depended on whichever tenant role was seen last | Auth provider collapsed tenant scope by iterating role rows without enforcing a single non-platform tenant | Fixed |
| LF-026 | Medium | Unit repro: platform admin with an extra tenant-scoped role got a tenant claim/session tenant, which could prefill UI forms unexpectedly | Auth provider did not clear tenant scope for platform admins when tenant roles were also present | Fixed |
| LF-027 | Medium | UI repro: go to page 2+ in Sessions, click `Clear filters`, and pagination stayed offset so “cleared” results could still look empty or partial | Clear/reset actions only reset filter fields, not pagination state | Fixed |
| LF-028 | Low | UI repro: copy an invite link from Users and paste it into chat/email; it was a relative path, not a usable standalone URL | Users page copied `/invite/accept?...` instead of an absolute URL with origin | Fixed |

### Files Changed

- `cmd/console-api/auth_provider.go`
- `cmd/console-api/auth_provider_test.go`
- `cmd/console-api/main.go`
- `cmd/console-api/session_handlers_test.go`
- `web/console/src/api.ts`
- `web/console/src/pages/Sessions.tsx`
- `web/console/src/pages/Users.tsx`
- `readme.md`

### Minimal Repros

- LF-024 before fix:
  - `TOKEN=$(curl -sS -X POST http://localhost:8090/auth/login -H 'Content-Type: application/json' -d '{"email":"admin@openclause.dev","password":"Admin123!"}' | jq -r '.token')`
  - `curl -sS "http://localhost:8090/admin/sessions?tenant_id=74084408-873e-40a7-af0d-ae4e79c88a7f&session_id=demo-session-1774060201&since=2099-01-01T00:00" -H "Authorization: Bearer $TOKEN" | jq 'length'`
  - Before: `1`
  - After: `0`
- LF-025 before fix:
  - Unit repro in `Test_EmailPasswordAuthProvider_Login_rejectsMultipleTenantAssignments`
  - Before: login succeeded with an arbitrary tenant claim
  - After: login returns `409` with `user has multiple tenant assignments`
- LF-026 before fix:
  - Unit repro in `Test_EmailPasswordAuthProvider_Login_platformAdminIgnoresTenantScopedRoles`
  - Before: platform-admin token/session inherited a tenant claim from unrelated tenant-scoped roles
  - After: platform-admin token/session keeps empty tenant scope
- LF-027 before fix:
  - UI steps: open Sessions, paginate forward, click `Clear filters`
  - Before: page offset stayed unchanged
  - After: filters and page reset together
- LF-028 before fix:
  - UI steps: create invite from Users, click `Copy link`, paste outside the browser context
  - Before: copied value was a relative path
  - After: copied value is a full absolute URL

### Verification

- `go test ./cmd/console-api -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/console-api`
- `npm --prefix web/console run build`
  - Pass
  - Key output: `✓ built in 585ms`
- `go test ./... -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/console-api`, `ok github.com/bturcanu/OpenClause/pkg/console`
- `go test -race ./... -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/console-api`, `ok github.com/bturcanu/OpenClause/pkg/console`
- `docker run --rm -v "$PWD/policy:/policy" openpolicyagent/opa:0.62.0 test /policy/bundles/v0 /policy/tests -v`
  - Pass
  - Key output: `PASS: 19/19`
- `npm --prefix sdk/typescript run build`
  - Pass
  - Key output: `tsc`
- `./scripts/dev.sh`
  - Pass
  - Key output: stack rebuilt and migrations reapplied cleanly
- `./scripts/demo.sh`
  - Pass
  - Key output: session confirmation `Session visible in console API: demo-session-1774061260`
- Live LF-024 curl repro after rebuild
  - Pass
  - Key output: future `since=2099-01-01T00:00` query returned `0` sessions instead of `1`

## Flow Map

### A. Console bootstrap and auth flows
- 1. Setup wizard: [cmd/console-api/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/main.go), [cmd/console-api/setup_config_test.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/setup_config_test.go), [web/console/src/App.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/App.tsx), [web/console/src/pages/SetupWizard.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/SetupWizard.tsx), [pkg/console/store.go](/Users/bogdan/dev/personal/OpenClause/pkg/console/store.go)
- 2. Login/JWT issuance: [cmd/console-api/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/main.go), [cmd/console-api/auth_provider.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/auth_provider.go), [cmd/console-api/auth_provider_test.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/auth_provider_test.go), [pkg/console/jwt.go](/Users/bogdan/dev/personal/OpenClause/pkg/console/jwt.go), [pkg/console/store.go](/Users/bogdan/dev/personal/OpenClause/pkg/console/store.go), [web/console/src/pages/Login.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Login.tsx)
- 3. Invite accept: [cmd/console-api/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/main.go), [pkg/console/store.go](/Users/bogdan/dev/personal/OpenClause/pkg/console/store.go), [web/console/src/pages/InviteAccept.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/InviteAccept.tsx), [web/console/src/pages/Users.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Users.tsx)
- 4. Password reset: [cmd/console-api/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/main.go), [pkg/console/store.go](/Users/bogdan/dev/personal/OpenClause/pkg/console/store.go), [web/console/src/pages/PasswordReset.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/PasswordReset.tsx)
- 5. RBAC/tenant scope: [cmd/console-api/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/main.go), [pkg/console/jwt.go](/Users/bogdan/dev/personal/OpenClause/pkg/console/jwt.go), [pkg/console/store.go](/Users/bogdan/dev/personal/OpenClause/pkg/console/store.go)

### B. Tenant management flows
- 6. Tenant create/list/status: [cmd/console-api/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/main.go), [pkg/console/store.go](/Users/bogdan/dev/personal/OpenClause/pkg/console/store.go), [web/console/src/pages/Tenants.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Tenants.tsx), [web/console/src/pages/TenantDetail.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/TenantDetail.tsx), [pkg/auth/dbkeys.go](/Users/bogdan/dev/personal/OpenClause/pkg/auth/dbkeys.go)
- 7. Agents CRUD: [cmd/console-api/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/main.go), [pkg/console/store.go](/Users/bogdan/dev/personal/OpenClause/pkg/console/store.go), [web/console/src/pages/TenantDetail.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/TenantDetail.tsx)
- 8. API keys create/list/revoke/rotate: [cmd/console-api/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/main.go), [pkg/console/store.go](/Users/bogdan/dev/personal/OpenClause/pkg/console/store.go), [pkg/auth/apikey.go](/Users/bogdan/dev/personal/OpenClause/pkg/auth/apikey.go), [pkg/auth/dbkeys.go](/Users/bogdan/dev/personal/OpenClause/pkg/auth/dbkeys.go), [web/console/src/pages/TenantDetail.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/TenantDetail.tsx)

### C. Toolcall governance flows
- 9-13. Toolcall submit/eval/allow/deny/approve/execute: [cmd/gateway/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/gateway/main.go), [cmd/gateway/main_test.go](/Users/bogdan/dev/personal/OpenClause/cmd/gateway/main_test.go), [pkg/types/toolcall.go](/Users/bogdan/dev/personal/OpenClause/pkg/types/toolcall.go), [pkg/evidence/store.go](/Users/bogdan/dev/personal/OpenClause/pkg/evidence/store.go), [pkg/policy/client.go](/Users/bogdan/dev/personal/OpenClause/pkg/policy/client.go), [policy/bundles/v0/main.rego](/Users/bogdan/dev/personal/OpenClause/policy/bundles/v0/main.rego), [pkg/connectors/registry.go](/Users/bogdan/dev/personal/OpenClause/pkg/connectors/registry.go)

### D. Approvals flows
- 14-16. Pending queue/internal endpoints/Slack interactions: [cmd/approvals/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/approvals/main.go), [pkg/approvals/handlers.go](/Users/bogdan/dev/personal/OpenClause/pkg/approvals/handlers.go), [pkg/approvals/store.go](/Users/bogdan/dev/personal/OpenClause/pkg/approvals/store.go), [pkg/approvals/handlers_slack_test.go](/Users/bogdan/dev/personal/OpenClause/pkg/approvals/handlers_slack_test.go), [web/console/src/pages/Approvals.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Approvals.tsx)

### E. Notification routing flows
- 17-18. Tenant notification config and webhook/slack delivery: [cmd/console-api/notification_config.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/notification_config.go), [cmd/console-api/notification_config_handler_test.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/notification_config_handler_test.go), [pkg/approvals/notifier.go](/Users/bogdan/dev/personal/OpenClause/pkg/approvals/notifier.go), [cmd/alert-worker/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/alert-worker/main.go), [web/console/src/pages/TenantDetail.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/TenantDetail.tsx)

### F. Audit / exports flows
- 19-20. Audit list/detail and exports: [cmd/console-api/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/main.go), [cmd/console-api/export_handlers_test.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/export_handlers_test.go), [pkg/console/store.go](/Users/bogdan/dev/personal/OpenClause/pkg/console/store.go), [web/console/src/pages/Events.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Events.tsx), [web/console/src/pages/EventDetail.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/EventDetail.tsx)

### G. Alerts flows
- 21-23. Rules, worker loop, UI: [cmd/console-api/alerts_handlers.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/alerts_handlers.go), [cmd/console-api/alerts_handlers_test.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/alerts_handlers_test.go), [cmd/alert-worker/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/alert-worker/main.go), [cmd/alert-worker/main_test.go](/Users/bogdan/dev/personal/OpenClause/cmd/alert-worker/main_test.go), [pkg/alerts](/Users/bogdan/dev/personal/OpenClause/pkg/alerts), [web/console/src/pages/Alerts.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Alerts.tsx), [web/console/src/pages/TenantDetail.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/TenantDetail.tsx)

### H. Analytics flows
- 24-26. Overview, timeseries, tenant summary, UI rendering: [cmd/console-api/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/main.go), [cmd/console-api/tenant_analytics_handlers.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/tenant_analytics_handlers.go), [cmd/console-api/tenant_analytics_handlers_test.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/tenant_analytics_handlers_test.go), [pkg/console/store.go](/Users/bogdan/dev/personal/OpenClause/pkg/console/store.go), [web/console/src/pages/Overview.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Overview.tsx), [web/console/src/pages/TenantDetail.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/TenantDetail.tsx), [web/console/src/api.ts](/Users/bogdan/dev/personal/OpenClause/web/console/src/api.ts)

### I. Connectors catalog flows
- 27-28. Gateway catalog and console catalog UI: [cmd/gateway/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/gateway/main.go), [pkg/connectors/registry.go](/Users/bogdan/dev/personal/OpenClause/pkg/connectors/registry.go), [pkg/connectors/builtins/registry.go](/Users/bogdan/dev/personal/OpenClause/pkg/connectors/builtins/registry.go), [cmd/console-api/main.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/main.go), [cmd/console-api/connectors_handler_test.go](/Users/bogdan/dev/personal/OpenClause/cmd/console-api/connectors_handler_test.go), [web/console/src/pages/Connectors.tsx](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Connectors.tsx)

### J. SDK flows
- 29. TypeScript SDK: [sdk/typescript/src/client.ts](/Users/bogdan/dev/personal/OpenClause/sdk/typescript/src/client.ts), [sdk/typescript/src/errors.ts](/Users/bogdan/dev/personal/OpenClause/sdk/typescript/src/errors.ts), [sdk/typescript/src/models.ts](/Users/bogdan/dev/personal/OpenClause/sdk/typescript/src/models.ts), [sdk/typescript/tests/client.test.ts](/Users/bogdan/dev/personal/OpenClause/sdk/typescript/tests/client.test.ts), [sdk/typescript/tests/mcp.test.ts](/Users/bogdan/dev/personal/OpenClause/sdk/typescript/tests/mcp.test.ts)
- 30. Java SDK: [sdk/java/src/main/java/dev/openclause/sdk/OpenClauseClient.java](/Users/bogdan/dev/personal/OpenClause/sdk/java/src/main/java/dev/openclause/sdk/OpenClauseClient.java), [sdk/java/src/test/java/dev/openclause/sdk/OpenClauseClientTest.java](/Users/bogdan/dev/personal/OpenClause/sdk/java/src/test/java/dev/openclause/sdk/OpenClauseClientTest.java), [sdk/java/build.gradle](/Users/bogdan/dev/personal/OpenClause/sdk/java/build.gradle)
- 31. Python SDK: [sdk/python/openclause/client.py](/Users/bogdan/dev/personal/OpenClause/sdk/python/openclause/client.py), [sdk/python/openclause/langchain.py](/Users/bogdan/dev/personal/OpenClause/sdk/python/openclause/langchain.py), [sdk/python/tests/test_client.py](/Users/bogdan/dev/personal/OpenClause/sdk/python/tests/test_client.py), [sdk/python/pyproject.toml](/Users/bogdan/dev/personal/OpenClause/sdk/python/pyproject.toml)
- 32. Go SDK: [pkg/sdk/client/client.go](/Users/bogdan/dev/personal/OpenClause/pkg/sdk/client/client.go)

## Findings

| ID | Sev | Flow | Symptom | Root cause | Plan | Files | Status |
|---|---|---|---|---|---|---|---|
| LF-001 | Medium | A3/A4 | Dev-mode invite/reset copy-paste flow points operators to non-existent console routes | console-api logs `/auth/invite/accept?...` and `/reset/confirm?...` instead of the actual UI pages | Route logs through page-URL helpers and test them | `cmd/console-api/main.go`, `cmd/console-api/setup_config_test.go` | Fixed |
| LF-002 | Medium | A1 | Setup wizard accepts whitespace-only `first_tenant_name`, creating invalid tenant names | handler validates before trimming | Normalize and validate first tenant name after trim | `cmd/console-api/main.go`, `cmd/console-api/setup_config_test.go` | Fixed |
| LF-003 | Medium | A2 | Login can fail for case-variant email input even though invite/reset flows already treat emails case-insensitively | `AuthenticateUser` used exact-case lookup and login forwarded raw email | Canonicalize emails and authenticate case-insensitively; add tests for trimmed dispatch + helper | `pkg/console/store.go`, `pkg/console/store_test.go`, `cmd/console-api/main.go`, `cmd/console-api/auth_provider_test.go` | Fixed |
| LF-004 | Medium | B6 | Platform admins had no console control to disable/enable a tenant even though the backend and docs supported it | tenant status endpoint existed, but the tenant detail page had no UI action wired to it | Add tenant status control to the tenant detail screen | `web/console/src/pages/TenantDetail.tsx` | Fixed |
| LF-005 | Medium | B6/B7/B8 | Tenant/agent/API-key mutations accepted whitespace-only names and surfaced missing-resource or already-revoked states as generic 500s | handlers validated before trim and store methods did not distinguish not-found/conflict cases | Normalize names and map tenant/key state errors to 404/409 contracts | `cmd/console-api/main.go`, `pkg/console/store.go`, `cmd/console-api/setup_config_test.go` | Fixed |
| LF-006 | High | C9/C13 | Idempotent `POST /v1/toolcalls` replays lost execution results or approval URLs, and `/execute` could miss a recorded replay until the parent link existed | evidence idempotency lookup only returned `event_id` + `decision` | Return full replay payloads from evidence and use deterministic exec-idempotency fallback in the gateway | `pkg/evidence/store.go`, `pkg/evidence/logger.go`, `cmd/gateway/main.go`, `cmd/gateway/main_test.go` | Fixed |
| LF-007 | High | C9/C12 | Tenant policy config could silently replace the OPA result at runtime, dropping OPA reasons, notify routes, approver groups, and any future policy metadata | gateway re-ran local rule-builder logic after already sending tenant config to OPA | Keep tenant config as policy input only; stop overriding OPA decisions in production flow | `cmd/gateway/main.go`, `cmd/gateway/main_test.go` | Fixed |
| LF-008 | High | C10/C12 | Gateway returned apparent success on approve/deny paths even when evidence persistence or approval-request creation failed | critical errors were logged but the handler still encoded success responses | Fail closed on evidence/approval creation failures | `cmd/gateway/main.go`, `cmd/gateway/main_test.go` | Fixed |
| LF-009 | Medium | D14/D16 | Approval resolve races/expired requests returned inconsistent status codes, and deny could still resolve expired requests | approvals store used generic errors and `DenyRequest` ignored `expires_at` | Add a shared sentinel error, enforce expiry on deny, and map conflicts consistently in approvals + console-api handlers | `pkg/approvals/store.go`, `pkg/approvals/handlers.go`, `pkg/approvals/handlers_slack_test.go`, `cmd/console-api/main.go` | Fixed |
| LF-010 | Medium | G23 | Global Alerts page could not create rules reliably | UI posted the tenant-scoped payload shape to a different global handler contract and had no tenant input for platform admins | Unify alert-create payloads, add global create tests, and add tenant input in the UI | `cmd/console-api/alerts_handlers.go`, `cmd/console-api/alerts_handlers_test.go`, `cmd/console-api/main.go`, `web/console/src/pages/Alerts.tsx` | Fixed |
| LF-011 | Medium | H24 | Overview analytics chart rendered from the wrong field and could collapse into broken/NaN bar heights | UI expected `count`, but the API returns `total` buckets | Align the chart with the real `total` field | `web/console/src/pages/Overview.tsx` | Fixed |
| LF-012 | Medium | J32 | Go SDK approval wait loop retried permanent `409` conflicts instead of only `awaiting approval` conflicts | retry helper treated any HTTP 409 as retryable | Restrict retries to awaiting-approval conflicts and add retry-loop regression tests | `pkg/sdk/client/client.go`, `pkg/sdk/client/client_test.go` | Fixed |
| LF-013 | Medium | J30 | Java SDK verification was not reproducible from a fresh checkout | repo lacked a committed Gradle wrapper, and `./gradlew` failed on Homebrew Java installs where `java` was not on `PATH` | Add wrapper artifacts and Homebrew JDK fallback in the wrapper script; document `./gradlew test` | `sdk/java/gradlew`, `sdk/java/gradlew.bat`, `sdk/java/gradle/wrapper/*`, `sdk/java/README.md` | Fixed |
| LF-014 | Low | Docs/Smoke | Local testing/demo scripts and docs had stale or misleading commands | broken shell interpolation in JSON examples, stale CSV export endpoints in scripts, and outdated SDK verification/install commands | Update `README`, `LOCAL_TESTING`, SDK READMEs, and smoke scripts to match verified behavior | `readme.md`, `docs/LOCAL_TESTING.md`, `sdk/python/README.md`, `sdk/java/README.md`, `scripts/demo.sh`, `scripts/e2e-curl-happy-path.sh` | Fixed |

## Fix Plan

- [x] Build initial flow inventory and tracker.
- [x] Review Flow A end-to-end and fix confirmed issues.
- [x] Review Flow B.
- [x] Review Flow C.
- [x] Review Flow D.
- [x] Review Flow E.
- [x] Review Flow F.
- [x] Review Flow G.
- [x] Review Flow H.
- [x] Review Flow I.
- [x] Review Flow J.
- [x] Run full smoke flow on fresh stack and capture outputs.
- [x] Final docs/README contract pass.

## Per-Bug Checklist

### LF-001
- [x] Reproduce and fix.
- [x] Add tests.
- [ ] Smoke via console logs in fresh stack.

### LF-002
- [x] Reproduce and fix.
- [x] Add tests.
- [x] Smoke via setup wizard request.

### LF-003
- [x] Reproduce and fix.
- [x] Add tests.
- [x] Smoke via mixed-case login.

### LF-004
- [x] Reproduce and fix.
- [x] Verify in web console build.
- [x] Smoke via tenant status toggle against fresh stack.

### LF-005
- [x] Reproduce and fix.
- [x] Add tests/helpers.
- [x] Smoke via tenant/agent/API-key mutations on fresh stack.

### LF-006
- [x] Reproduce and fix.
- [x] Add gateway replay tests.
- [x] Smoke via repeated toolcall/execute requests on fresh stack.

### LF-007
- [x] Reproduce and fix.
- [x] Add gateway regression test for preserved OPA result metadata.
- [ ] Smoke via policy-config-backed approval flow.

### LF-008
- [x] Reproduce and fix.
- [x] Add gateway failure-path tests.
- [x] Smoke via approve/deny flow on fresh stack.

### LF-009
- [x] Reproduce and fix.
- [x] Add approvals handler tests.
- [x] Smoke via resolved/expired approval actions.

### LF-010
- [x] Reproduce and fix.
- [x] Add alert create handler tests.
- [x] Verify in web console build.
- [x] Smoke via global Alerts page or tenant alerts flow.

### LF-011
- [x] Reproduce and fix.
- [x] Verify in web console build.
- [ ] Smoke via overview analytics page.

### LF-012
- [x] Reproduce and fix.
- [x] Add Go SDK retry-loop tests.
- [ ] Smoke via Go SDK client against a live approval flow.

### LF-013
- [x] Reproduce and fix.
- [x] Add wrapper artifacts.
- [x] Verify `./gradlew test`.

### LF-014
- [x] Reproduce and fix.
- [x] Update README / LOCAL_TESTING / smoke scripts.
- [x] Smoke the updated scripts on a fresh stack.

## Files Changed

- `cmd/console-api/main.go`
- `cmd/console-api/alerts_handlers.go`
- `cmd/console-api/alerts_handlers_test.go`
- `cmd/console-api/setup_config_test.go`
- `cmd/console-api/auth_provider_test.go`
- `cmd/gateway/main.go`
- `cmd/gateway/main_test.go`
- `pkg/console/store.go`
- `pkg/console/store_test.go`
- `pkg/approvals/store.go`
- `pkg/approvals/handlers.go`
- `pkg/approvals/handlers_slack_test.go`
- `pkg/evidence/store.go`
- `pkg/evidence/logger.go`
- `pkg/sdk/client/client.go`
- `pkg/sdk/client/client_test.go`
- `sdk/java/gradlew`
- `sdk/java/gradlew.bat`
- `sdk/java/gradle/wrapper/gradle-wrapper.jar`
- `sdk/java/gradle/wrapper/gradle-wrapper.properties`
- `sdk/java/README.md`
- `sdk/python/README.md`
- `web/console/src/pages/TenantDetail.tsx`
- `web/console/src/pages/Alerts.tsx`
- `web/console/src/pages/Overview.tsx`
- `readme.md`
- `docs/LOCAL_TESTING.md`
- `scripts/demo.sh`
- `scripts/e2e-curl-happy-path.sh`
- `.ai/logic-flow-sweep.md`

## Verification Evidence

- `go test ./cmd/gateway -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/gateway`
- `go test ./pkg/approvals ./cmd/gateway ./cmd/console-api -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/pkg/approvals`, `ok github.com/bturcanu/OpenClause/cmd/console-api`
- `go test ./pkg/sdk/client -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/pkg/sdk/client`
- `go test ./... -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/console-api`, `ok github.com/bturcanu/OpenClause/cmd/gateway`, `ok github.com/bturcanu/OpenClause/pkg/sdk/client`
- `go test -race ./... -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/gateway`, `ok github.com/bturcanu/OpenClause/pkg/console`, `ok github.com/bturcanu/OpenClause/pkg/sdk/client`
- `opa test policy/bundles/v0/ policy/tests/ -v`
  - Pass
  - Key output: `PASS: 19/19`
- `npm --prefix web/console install`
  - Pass
  - Key output: `up to date in 1s`
- `npm --prefix web/console run build`
  - Pass
  - Key output: `✓ built in 560ms`
- `npm --prefix sdk/typescript install`
  - Pass
  - Key output: `up to date in 1s`
- `npm --prefix sdk/typescript test -- --runInBand`
  - Pass
  - Key output: `PASS tests/mcp.test.ts`, `PASS tests/client.test.ts`
- `npm --prefix sdk/typescript run build`
  - Pass
  - Key output: `tsc`
- `PYTHONPATH=sdk/python python3 -m unittest discover -s sdk/python/tests -v`
  - Pass
  - Key output: `Ran 16 tests`, `OK`
- `python3.10 --version`
  - Not available in this environment
  - Key output: `command not found: python3.10`
- `./gradlew test`
  - Pass
  - Key output: `BUILD SUCCESSFUL`
- `python3 -m pip install -e .`
  - Not rerun after docs update
  - Rationale: local environment does not provide `python3.10`, and current docs now state the editable-install gate explicitly.

## Smoke Evidence

- `docker compose --env-file .env -f deploy/docker-compose.yml down -v`
  - Pass
  - Key output: local stack reset completed before smoke run.
- `./scripts/dev.sh`
  - Pass
  - Key output: images built, services started, migration `001_initial` applied, health checks reported gateway, approvals, slack, jira, opa, and minio ready.
- `./scripts/demo.sh`
  - Pass
  - Key output:
    - setup initialized fresh instance
    - login succeeded for `admin@openclause.dev`
    - tenant `48d8d4cf-8415-4862-80c0-f6ccf2b1e249`
    - agent `014a805d-640a-4647-ac60-1fe1ac221b3b`
    - allow event `990fbdd2-7e8a-478f-bc02-05fc428f7c1a`
    - deny event `e35ae6f0-ce9c-44a4-84c5-990855b3b1df`
    - approve event `c5b04eea-2ff0-4547-8bf6-4b560ddc17c6`
    - execute event `44fa01c4-fcb5-4acb-852f-fded66db2052`
    - CSV export `HTTP 200`
    - bundle export `4 events`
    - gateway connectors `3 registered`
    - console connectors `3 registered`
    - tenant analytics `4 events in last 24h`
- Alerts smoke via curl
  - Pass
  - Key output:
    - tenant `105d9e3c-34e1-4f62-9199-37f234463e3c`
    - agent `19412da2-b4a9-4b31-8bba-7c97658c6475`
    - rule `1bc94059-e3f9-4a74-bd30-5a280051d802`
    - notification config `{"notify":[{"kind":"slack","channel":"#team-alerts"}]}`
    - three deny toolcalls returned decision `deny`
    - alert event `8c540354-6a1a-46ea-b92d-f16c81dbbe05`
    - event status `sent`
    - message `deny spike: 3 denies in last 5 minutes (threshold 3)`
- Targeted live regressions via curl
  - Pass
  - Key output:
    - mixed-case login succeeded with email `"  ADMIN@OPENCLAUSE.DEV  "`
    - disabled tenant returned `403` with body `{"code":"FORBIDDEN","message":"tenant disabled","retryable":false}`
    - allow replay preserved event/result for `2a98f497-bf8c-457d-8659-c761bc37978a`
    - approval replay preserved `approval_url` for `76722b1e-1398-439f-a854-f49663724740`
    - post-approval deny returned `409 request already resolved or expired`
    - execute replay preserved event/result for `592d50bb-47cd-4a79-9259-4a3a82608c8d`

## Notes

- Live smoke directly covered LF-002, LF-003, LF-004, LF-005, LF-006, LF-008, LF-009, LF-010, LF-011 API reachability, and LF-014.
- LF-001, LF-007, and LF-012 remain primarily verified by focused automated regression tests because they require log-link inspection, policy-engine failure/override conditions, or SDK harness behavior that is more reliable to validate in tests than raw curl.
- Historical note: the early `gateway connectors 3 registered` / `console connectors 3 registered` smoke output above came from a pre-registry-proxy demo run and is superseded by later follow-up sections and the current README/LOCAL_TESTING guidance, which both reflect the current 8-connector catalog.

## UI Issues Fixed

- Global polish
  - Improved sidebar contrast and active/hover affordance in `web/console/src/index.css`.
  - Strengthened badge borders/contrast across zebra and hover table states in `web/console/src/index.css`.
  - Contained `pre`/code blocks on smaller widths and kept single horizontal scroll behavior in `web/console/src/index.css`.
  - Removed connector-page silent failure behavior by surfacing actionable inline errors with retry in `web/console/src/pages/Connectors.tsx`.
- Sessions
  - Reworked the Sessions banner copy, clarified local-time filters, and enforced numeric risk inputs in `web/console/src/pages/Sessions.tsx`.
  - Simplified Session detail header actions into an export menu, added structured run context with copy actions, removed the stray approval punctuation, and made “Why OpenClause decided this way” more actionable with a direct policy link in `web/console/src/pages/SessionTimeline.tsx`.
  - Added tenant preselection support for policy review links in `web/console/src/pages/Policies.tsx`.
- Users
  - Tightened the invite form into a two-column layout, improved invite delivery feedback, cleaned up role pill wrapping, and simplified the sessions cell/action layout in `web/console/src/pages/Users.tsx`.
- Tenant detail
  - Realigned Rotate Primary Key controls, clarified expiration date copy, and split Notification Routing into clearer desktop/mobile sections in `web/console/src/pages/TenantDetail.tsx`.
  - Improved approver helper-text contrast, aligned the Add action with the form, and upgraded empty states for approvers and alerts in `web/console/src/pages/TenantDetail.tsx`.
  - Converted alert creation into a clear create card, made enabled/disabled semantics explicit, added trend-chart low-data messaging, improved analytics legend readability, and de-emphasized zero-value heatmap cells in `web/console/src/pages/TenantDetail.tsx`.
- Connectors
  - Added a search box, help panel, and capped action-badge display with `+N more` expansion in `web/console/src/pages/Connectors.tsx`.

### UI Verification

- `npm --prefix web/console run build`
  - Pass
  - Key output: `✓ built in 575ms`
- No Go tests were required for this pass because the changes were limited to console UI behavior and styling.

## Final Sweep: API Client + Console Consistency

Date: 2026-03-20
Branch: `feature/console-sessions-polish`
Status: Complete

### New Findings

| ID | Sev | Flow | Symptom | Root cause | Fix | Files | Status |
|---|---|---|---|---|---|---|---|
| LF-032 | Medium | Policies UI | Policy Builder still loaded config + version history as all-or-nothing, so a partial outage looked like a full-page failure and hid whichever half still worked | `fetchPolicyState` used `Promise.all(...)` and a single catch path instead of explicit partial-load handling | Switched the load to `Promise.allSettled(...)`, surfaced a single actionable error banner naming the failed sections, reset only the failed slices, and upgraded the empty version-history row to a real `EmptyState` | `web/console/src/pages/Policies.tsx` | Fixed |
| LF-033 | Low | Overview UI | The dashboard still had older “plain row” empty states, which looked inconsistent next to the newer operator surfaces and made no-data conditions feel accidental | `Overview` had earlier error handling improvements but had not been updated to the shared empty-state pattern | Replaced the remaining empty placeholders with `EmptyState` blocks for both event-volume and recent-event no-data cases | `web/console/src/pages/Overview.tsx` | Fixed |

### API Client Invariant

- Confirmed and kept the existing invariant in `web/console/src/api.ts`:
  - `apiFetch(...)` returns a raw `Response`
  - `api.get/post/put/delete` are responsible for JSON parsing
  - `api.delete(...)` returns `{}` for `204 No Content` and does not attempt JSON parsing
- This avoided a double-parse runtime break after the recent `readJSONResponse(...)` changes, so no further `api.ts` refactor was needed in this pass.

### Files Changed

- `web/console/src/pages/Overview.tsx`
- `web/console/src/pages/Policies.tsx`

### Verification

- `npm --prefix web/console run build`
  - Pass
  - Key output: `✓ built in 784ms`
- Route-shell smoke after final sweep
  - Pass
  - Key output:
    - Overview is mounted at `/` and returned `200`
    - `/policies` and `/connectors` returned `200`
    - `/events` and `/events/20fb65a2-484c-4fcc-9109-da37a9da8689` returned `200`
    - `/approvals`, `/alerts`, `/tenants`, `/tenants/9550e31d-4f23-4da3-b206-55b5ec61ff5e?tab=agents`, `/users`, `/sessions`, `/sessions/demo-session-1774067410`, `/login`, `/invite/accept?token=demo-token`, and `/reset?token=demo-token` all returned `200`
- Operator-path API smoke
  - Pass
  - Key output:
    - login succeeded for `admin@openclause.dev`
    - current tenant `9550e31d-4f23-4da3-b206-55b5ec61ff5e`
    - current event `20fb65a2-484c-4fcc-9109-da37a9da8689`
    - current session `demo-session-1774067410`
    - invite creation returned `{"email_status":"logged","has_accept_url":true}`
- Outage simulation
  - Pass
  - Key output:
    - while `console-api` was stopped, `http://localhost:3000/` still returned `200`
    - proxied API routes `http://localhost:3000/api/setup/status` and `http://localhost:3000/api/admin/events?limit=1` returned `502`
    - after restart, `http://localhost:8090/healthz` returned `200`

### Notes

- No backend/API contract changes were made in this final sweep.
- The outage simulation evidence aligns with the current page code paths: auth/bootstrap pages and the remaining data-heavy pages now route request failures into explicit `InlineErrorState` handling instead of rendering misleading empty states.

## Final Sweep: Documentation Accuracy

Date: 2026-03-20
Branch: `feature/console-sessions-polish`
Status: Complete

### New Findings

| ID | Sev | Flow | Symptom | Root cause | Fix | Files | Status |
|---|---|---|---|---|---|---|---|
| LF-034 | Low | Internal docs / roadmap | `.ai/next-steps.md` still presented NS-01, NS-02, and NS-10 as open backlog even though later branches had already shipped those capabilities | The planning doc captured a correct point-in-time snapshot, but it never got a status refresh after the connector-registry, Gradle-wrapper, and auth-session work landed | Added a current-state refresh section and marked the completed backlog items explicitly as later-completed historical entries | `.ai/next-steps.md` | Fixed |
| LF-035 | Low | Internal tracker hygiene | `.ai/sessions-and-ui-polish.md` still showed one docs verification item as `pending` even though the README/local-testing updates had already been validated through build + demo runs | Tracker drift after the polish work was finished | Updated the checklist entry with the actual verification commands that were already used | `.ai/sessions-and-ui-polish.md` | Fixed |
| LF-036 | Low | Release/docs summary | The primary README still stopped at `v0.3`, so the top-level project summary missed the connector registry, auth-session revocation, operator-grade Sessions, invite email delivery, Java wrapper, and final console hardening work | The release summary was never refreshed after the later branches landed | Added a concise `v0.4` summary line covering the features and hardening that shipped after the `v0.3` line | `readme.md` | Fixed |

### Files Changed

- `.ai/next-steps.md`
- `.ai/sessions-and-ui-polish.md`
- `.ai/logic-flow-sweep.md`
- `readme.md`

### Verification

- `go test ./... -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/console-api`, `ok github.com/bturcanu/OpenClause/pkg/console`, `ok github.com/bturcanu/OpenClause/pkg/sdk/client`
- `go test -race ./... -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/gateway`, `ok github.com/bturcanu/OpenClause/pkg/console`, `ok github.com/bturcanu/OpenClause/pkg/evidence`
- `docker run --rm -v "$PWD/policy:/policy" openpolicyagent/opa:0.62.0 test /policy/bundles/v0 /policy/tests -v`
  - Pass
  - Key output: `PASS: 19/19`
- `npm --prefix web/console run build`
  - Pass
  - Key output: `✓ built in 4.13s`
- `npm --prefix sdk/typescript run build`
  - Pass
  - Key output: `tsc`
- `PYTHONPATH=sdk/python python3 -m unittest discover -s sdk/python/tests -v`
  - Pass
  - Key output: `Ran 16 tests`, `OK`
- `cd sdk/java && ./gradlew test`
  - Pass
  - Key output: `BUILD SUCCESSFUL`
- `./scripts/dev.sh`
  - Pass
  - Key output: stack rebuilt cleanly, migrations reapplied, and health URLs printed for gateway/approvals/connectors/OPA/MinIO
- `./scripts/demo.sh`
  - Pass
  - Key output: 14/14 steps passed, including invite email status, session visibility, connectors `8 registered`, and tenant analytics `4 events in last 24h`

### Notes

- No new runtime regressions were reproduced in this final pass. The actionable issues were documentation drift and release-summary gaps rather than code-path failures.

## Follow-up: 2026-03-21 Approval / Export / Docs Hardening

### New Findings

| ID | Sev | Flow | Symptom | Root cause | Fix | Files | Status |
|---|---|---|---|---|---|---|---|
| LF-038 | Medium | Evidence / exports | `GET /admin/reports/export/bundle` ignored the caller-supplied `until` timestamp, so bundle exports could include newer events than requested | `handleExportBundle` parsed `since` but always used `time.Now()` for `until` | Parse `until` exactly like the CSV export path and cover it with a handler test | `cmd/console-api/main.go`, `cmd/console-api/export_handlers_test.go` | Fixed |
| LF-039 | Medium | Sessions / exports | `GET /admin/sessions/{session_id}/export/csv` could return `200` with a header-only CSV even when the session did not exist or the tenant hint was wrong | CSV export skipped the existence check that JSON export already performed | Validate the session first, return `404 session not found` when appropriate, and keep the existing ambiguity contract intact | `cmd/console-api/main.go`, `cmd/console-api/session_handlers_test.go` | Fixed |
| LF-040 | Medium | Evidence / exports | Large evidence-bundle requests could silently truncate at the store’s 10,000-row cap | Bundle export relied on `ListEventsInRange(..., 10000)` without detecting overflow | Added an explicit count pre-check and fail-closed `400` when the requested window exceeds 10,000 events; updated docs to describe the current limit | `cmd/console-api/main.go`, `pkg/console/store.go`, `cmd/console-api/export_handlers_test.go`, `readme.md`, `docs/LOCAL_TESTING.md` | Fixed |
| LF-041 | Low | Docs / config | The Helm console-api values and current-state notes did not fully reflect invite-email/env support and the expanded 14-step demo | Docs/config drift after invite email delivery and session proof were added | Added SMTP + `CONSOLE_DEV_LOG_RAW_TOKENS` placeholders to Helm values, refreshed the README/local testing notes, and updated `next-steps.md` to describe the current 14-step demo | `deploy/helm/console-api/values.yaml`, `readme.md`, `docs/LOCAL_TESTING.md`, `.ai/next-steps.md` | Fixed |

### Verification

- `go test ./cmd/console-api -run 'TestHandleExportBundle_|TestHandleExportSessionCSV' -count=1`
  - Pass
- `go test ./... -count=1`
  - Pass
- `go test -race ./... -count=1`
  - Pass
- `npm --prefix web/console run build`
  - Pass
- `npm --prefix sdk/typescript run build`
  - Pass
- `docker run --rm -v "$PWD/policy:/policy" openpolicyagent/opa:0.62.0 test /policy/bundles/v0 /policy/tests -v`
  - Pass
- `PYTHONPATH=sdk/python python3 -m unittest discover -s sdk/python/tests -v`
  - Pass
- `cd sdk/java && ./gradlew test`
  - Pass
- `./scripts/demo.sh`
  - Pass
  - Key output:
    - invite creation returned `email_status=logged`
    - session step confirmed both list visibility and session detail export approval/execution linkage
    - bundle export returned `4 events`
    - connectors returned `8 registered` in both gateway and console
