# OpenClause — Next Steps (Verified)

**Branch:** `product/next-steps`
**Date:** 2026-03-20
**Method:** Full rebuild, fresh DB, automated demo script, verbatim output capture

---

## Status Refresh — Current Repo State

This file started as the verified backlog for the `product/next-steps` branch. Later branches have already completed several items that were still open in this snapshot:

- Completed since the original draft:
  - NS-01 connector registry endpoint via `GET /admin/connectors`
  - NS-02 Java Gradle wrapper and `./gradlew test`
  - NS-10 console login-session visibility and revocation, implemented from the Users surface via `GET /admin/auth-sessions` and `POST /admin/auth-sessions/{session_id}/revoke`
  - invite email delivery with `accept_url` + `email_status`
  - operator-grade Sessions explorer and broader console UI polish
- Still genuinely open/high-value:
  - NS-03 Python 3.9 support
  - NS-04 Windows `demo.ps1`
  - NS-05/NS-06 CI release gates + e2e demo job
  - NS-07 docs golden path consolidation
  - NS-08 broader mock connector coverage
  - NS-09 alerts folded into the main demo flow
- Watch items from the latest hardening sweeps:
  - Large-tenant evidence bundle export performance after the new count-before-export guard
  - Keeping future auth entry points on the shared tolerant bearer-parsing pattern
- Historical note:
  - the “Known gaps”, “Top 10 Next Steps”, and “PR-Sized Chunks” sections below preserve the branch-time planning context and are superseded wherever they conflict with the current repo state.

---

## 1. Reality Check — What Is Confirmed Working

### Quality Gates (all pass)

| Gate | Result |
|------|--------|
| `go test ./... -count=1` | ✅ 13 packages PASS |
| `go test -race ./... -count=1` | ✅ All PASS, no races |
| OPA policy tests (Docker, 0.62.0) | ✅ 19/19 PASS |
| `web/console npm run build` | ✅ 58 modules, clean |
| `sdk/typescript npm run build` | ✅ Clean compile |
| Java SDK `gradle test` (Docker) | ✅ 3 tasks, BUILD SUCCESSFUL |
| Python SDK `pip install -e .` | ⚠️ Requires Python ≥3.10 (system has 3.9.6) |

### Happy Path Demo (scripts/demo.sh — all 14 steps pass)

| Step | Result | Evidence |
|------|--------|----------|
| 1. Setup / Initialize | ✅ | `initialized: true` |
| 2. Login | ✅ | JWT token issued |
| 3. Create tenant/agent/key | ✅ | All IDs generated |
| 4. Allow toolcall (slack/channel.list, risk=1) | ✅ | `decision=allow` |
| 5. Deny toolcall (unknown/danger) | ✅ | `decision=deny` |
| 6. Approve toolcall (slack/msg.post, risk=8) | ✅ | `decision=approve` |
| 7. Platform admin approves (BUG-016) | ✅ | `status=approved`, no approver registration needed |
| 8. Execute approved call | ✅ | `result.status=success` (mock Slack connector) |
| 9. Audit trail | ✅ | Event detail returned with tool/action/decision |
| 10. Exports (CSV + bundle) | ✅ | CSV 200, bundle event_count=4 |
| 11. Sessions list + detail export | ✅ | Session visible plus approval/execution chain in JSON export |
| 12. Exports (CSV + bundle) | ✅ | CSV 200, bundle event_count=4 |
| 13. Connectors list (gateway) | ✅ | 8 connectors with actions (2 remote + 6 builtin) |
| 14. Tenant analytics | ✅ | Totals, trend, risk heatmap, per-agent breakdown |

### Additional Verifications

| Check | Result |
|-------|--------|
| Disabled tenant → 403 "tenant disabled" | ✅ (must use POST, not PUT) |
| Invite accept without JWT (BUG-001) | ✅ |
| Password reset without JWT (BUG-002) | ✅ |
| TenantDetail 404 → structured error (BUG-022) | ✅ |
| Alert rule creation (BUG-018) | ✅ |
| Tenants list for Policies page (BUG-019) | ✅ |

### Bug Sweep Tracker

`.ai/bug-sweep.md` contains per-bug evidence for all 43 bugs (BUG-001 through BUG-043) with:
- Status, summary, files changed, verification command
- Organized by severity: Blockers → High → Medium → Low
- Commit SHA: `30b1ce1` (main bulk), `cbc22bc` (OPA/Helm), `7fc444b` (deny body), `d5564ff` (binary cleanup)

### Unmerged Feature Branches

Both `fix/usability-correctness-sweep` and `fix/usability-deep-dive` are **already merged to main**.
All TB-001..TB-010 and U-001..U-012 fixes are merged (tracked in `.ai/usability-deep-dive-fixes.md`).

---

## 2. Issues Found in This Verification

### Fixed in this branch

| # | Issue | Fix |
|---|-------|-----|
| F-001 | Java SDK test uses `riskScore(8.5)` — compile error after BUG-003 Integer change | Changed test to `riskScore(8)` (type safety makes the non-integer test obsolete) |
| F-002 | Connectors.tsx used `tool` field but gateway returns `name` — field mismatch | Normalized UI to handle both `name` and `tool` fields |
| F-003 | Overview page calls `/admin/analytics/overview` + `/admin/analytics/timeseries` but routes were not registered | Routes already existed as handlers; added route registration |
| F-004 | Demo script missing | Created `scripts/demo.sh` — now a 14-step automated happy-path |

### Known gaps at the time of original verification (historical snapshot)

| # | Gap | Impact | Effort |
|---|-----|--------|--------|
| G-001 | Console-API `/v1/connectors` returned event-based data (observed tools), not the connector registry | Fixed later: console-api now proxies the full gateway connector registry via `/admin/connectors` | M |
| G-002 | Overview page endpoints (`/admin/analytics/overview`, `/admin/analytics/timeseries`) existed but weren't routed — now fixed | Overview showed zeros before fix | Fixed ✅ |
| G-003 | Python SDK requires Python ≥3.10; system macOS has 3.9.6 | Developers with older Python can't install | S |
| G-004 | No Gradle wrapper in Java SDK | Fixed later: repo now includes `sdk/java/gradlew` and wrapper assets | S |
| G-005 | No `scripts/demo.ps1` for Windows PowerShell | Windows developers can't run demo script | S |

---

## 3. Demo Script — 5-Minute "Wow" Sequence

**File:** `scripts/demo.sh`
**Prerequisites:** `./scripts/dev.sh` (starts Docker stack)

```bash
# Full automated demo:
./scripts/demo.sh

# Manual demo walkthrough:
# 1. Open http://localhost:3000 in browser
# 2. Complete setup wizard (or login if already initialized)
# 3. Navigate: Tenants → Create tenant → Create agent → Create API key
# 4. Open terminal, use API key to submit toolcalls:
#    - Low risk → auto-allowed
#    - Unknown tool → auto-denied
#    - High risk → requires approval
# 5. Go to Approvals page → Approve the pending request
# 6. Execute via API → show result
# 7. Audit Trail → show tamper-evident event chain
# 8. Exports → download CSV/bundle
# 9. Tenant Detail → Analytics tab → show charts
# 10. Tenant Detail → Alerts tab → create deny_spike rule
```

---

## 4. Backlog — Prioritized Next Steps

### Demo-ready v0.3 Checklist (must-haves)

- [x] First-run wizard works end-to-end
- [x] Create tenant/agent/key via UI and API
- [x] High-risk approval → execute → success result
- [x] Audit trail + exports (CSV/bundle)
- [x] Alerts + analytics visible
- [x] Platform admin can approve without approver registration
- [x] Zero broken pages in happy path
- [x] Automated demo script (`scripts/demo.sh`)

### Top 10 Next Steps

#### NS-01: Connector registry endpoint for console-api — Completed later
- **Status:** Completed on later branches via `GET /admin/connectors`
- **Impact:** Connectors page shows real connector catalog (not just observed events)
- **Acceptance:** `GET /admin/connectors` returns all 8 connectors with names, types, actions — even before any toolcalls
- **Sketch:** Add console-api env `GATEWAY_URL`, proxy `GET /v1/connectors` from gateway, or embed a static registry; update Connectors.tsx to call `/admin/connectors`
- **Effort:** S
- **Test:** `curl /admin/connectors | jq 'length'` returns 8
- **Docs:** Update LOCAL_TESTING.md connectors section
- **Demo:** "Here are all 8 connectors OpenClause can govern — Slack, Jira, GitHub, AWS, ServiceNow, email, Postgres, webhook"

#### NS-02: Add Gradle wrapper to Java SDK — Completed later
- **Status:** Completed on later branches via committed wrapper assets and `./gradlew test`
- **Impact:** Java SDK builds reproducibly without system Gradle
- **Acceptance:** `cd sdk/java && ./gradlew test` works on fresh checkout
- **Sketch:** `cd sdk/java && gradle wrapper --gradle-version 8.5`
- **Effort:** S
- **Test:** `./gradlew test` in CI and locally
- **Docs:** Update sdk/java/README.md

#### NS-03: Relax Python SDK to Python ≥3.9
- **Impact:** macOS system Python (3.9) can install the SDK
- **Acceptance:** `pip install -e .` works on Python 3.9.6
- **Sketch:** Change `requires-python = ">=3.9"` in pyproject.toml; verify no 3.10+ syntax (match/case, `X | Y` unions)
- **Effort:** S
- **Test:** Clean venv on 3.9 + `import openclause`
- **Docs:** Update sdk/python/README.md

#### NS-04: Windows demo script (demo.ps1)
- **Impact:** Windows developers can run the demo
- **Acceptance:** `.\scripts\demo.ps1` runs same 14 steps on PowerShell
- **Sketch:** Port demo.sh to PowerShell (Invoke-RestMethod instead of curl)
- **Effort:** S
- **Test:** Manual on Windows or PowerShell on macOS
- **Docs:** Add to README quick start

#### NS-05: CI release gate workflow
- **Impact:** PRs blocked if quality gates fail; images auto-built on merge
- **Acceptance:** GitHub Actions runs go test, OPA tests, web build, SDK builds, helm template on every PR
- **Sketch:** Extend `.github/workflows/ci.yml` with jobs for each gate; add `helm template` validation
- **Effort:** M
- **Test:** Open PR → checks run → all green
- **Docs:** Add CI badge to README

#### NS-06: End-to-end integration test in CI
- **Impact:** Catches regressions in the full stack
- **Acceptance:** CI spins up docker-compose, runs demo.sh, asserts exit 0
- **Sketch:** New `e2e` job in ci.yml: `docker compose up -d`, wait for health, `./scripts/demo.sh`
- **Effort:** M
- **Test:** CI green with demo.sh pass
- **Docs:** Document in CONTRIBUTING.md

#### NS-07: Consolidate docs into golden path
- **Impact:** Single clear onboarding path for developers
- **Acceptance:** README Quick Start is < 10 steps, works on macOS/Linux/Windows; LOCAL_TESTING.md is the deep reference
- **Sketch:** Deduplicate README and LOCAL_TESTING.md; add "Known defaults and ports" section; ensure all examples use real tenant IDs
- **Effort:** M
- **Test:** Fresh reader can copy-paste and succeed
- **Docs:** README + LOCAL_TESTING.md rewrite

#### NS-08: Improve mock connector coverage
- **Impact:** Demo never shows connector errors
- **Acceptance:** All common demo actions (slack msg.post, jira issue.create, github issue.create) return mock success
- **Sketch:** Update `pkg/connectors/builtins` handlers; ensure Slack/Jira mock connectors handle common actions gracefully
- **Effort:** S
- **Test:** Demo script uses each connector → all return success
- **Docs:** List supported mock actions in LOCAL_TESTING.md

#### NS-09: Alert worker demo observability
- **Impact:** Demo can show alert triggering live
- **Acceptance:** After 3 deny events in 5 minutes, alert event appears in Tenant Detail alerts tab within 30s
- **Sketch:** Reduce alert-worker poll interval for dev mode; add alert event polling in TenantDetail
- **Effort:** M
- **Test:** Demo script generates 3 denies, waits, verifies alert event
- **Docs:** Add "Alerts demo" section to LOCAL_TESTING.md

#### NS-10: Session management UI — Completed later
- **Status:** Completed on later branches; active console login sessions now live under Users via `/admin/auth-sessions`, while the Sessions page focuses on agent/tool-call runs
- **Impact:** Admins can see active user sessions and revoke access
- **Acceptance:** Sessions page lists active JWT sessions; revoke action invalidates token
- **Sketch:** Already have `/admin/sessions` endpoint; Sessions.tsx page exists; add revocation
- **Effort:** M
- **Test:** Login → see session → revoke → re-auth required
- **Docs:** Update README API reference

---

## 5. Release Checklist (merge to main)

Before merging `product/next-steps` to main:

- [x] `go test ./... -count=1` passes
- [x] `go test -race ./... -count=1` passes
- [x] OPA policy tests pass (19/19)
- [x] `web/console npm run build` passes
- [x] `sdk/typescript npm run build` passes
- [x] Java SDK `gradle test` passes (via Docker)
- [x] `scripts/demo.sh` passes all 14 steps on fresh DB
- [x] No stray binaries committed
- [x] `.ai/bug-sweep.md` has per-bug evidence for all 43 bugs
- [ ] Python SDK tested on Python 3.10+ (blocked by system Python version)

---

## 6. PR-Sized Chunks

### PR1: `fix/java-sdk-test-connectors-overview` (this branch)
**Scope:** Fix Java SDK test compile error, normalize Connectors.tsx field handling, register Overview analytics routes, add `scripts/demo.sh`
**Files:** `sdk/java/src/test/.../OpenClauseClientTest.java`, `web/console/src/pages/Connectors.tsx`, `cmd/console-api/main.go`, `scripts/demo.sh`
**Tests:** Java SDK gradle test passes; demo.sh runs end-to-end

### PR2: `fix/connector-registry-proxy`
**Scope:** NS-01 — Add `/admin/connectors` endpoint to console-api that returns the full connector registry (not event-based)
**Files:** `cmd/console-api/main.go`, `pkg/console/store.go` or new proxy, `web/console/src/pages/Connectors.tsx`
**Tests:** Connectors page shows 8 connectors on fresh install

### PR3: `fix/sdk-build-improvements`
**Scope:** NS-02 + NS-03 — Add Gradle wrapper, relax Python version to ≥3.9
**Files:** `sdk/java/gradlew`, `sdk/java/gradle/`, `sdk/python/pyproject.toml`
**Tests:** Both SDKs build in CI without pre-installed tools

### PR4: `feat/ci-release-gates`
**Scope:** NS-05 + NS-06 — CI quality gates + e2e demo test
**Files:** `.github/workflows/ci.yml`, potentially `scripts/ci-e2e.sh`
**Tests:** CI runs all gates on PR; e2e test spins up compose and runs demo.sh

### PR5: `docs/golden-path-consolidation`
**Scope:** NS-04 + NS-07 — Windows demo script + docs consolidation
**Files:** `scripts/demo.ps1`, `README.md`, `docs/LOCAL_TESTING.md`
**Tests:** README quick start verified on macOS + documented for Windows
