# OpenClause Bug Sweep — Fix Tracker

**Branch:** `fix/bug-sweep-2026-03-19`  
**Commit:** `30b1ce1`  
**Date:** 2026-03-19

## Historical Note

This file is a verified point-in-time bug-fix snapshot from `fix/bug-sweep-2026-03-19`.

- Active bug-minimization/source-of-truth trackers now live in:
  - `.ai/bug-minimization-roadmap.md`
  - `.ai/console-ui-test-tracker.md`
- Keep this document for historical evidence. If it conflicts with the current repo state, prefer the roadmap and current trackers.

## Quality Gates

| Gate | Status |
|------|--------|
| `go test ./... -count=1` | ✅ All 13 packages PASS |
| `go test -race ./... -count=1` | ✅ All PASS |
| OPA policy tests (Docker) | ✅ 17/17 PASS |
| `web/console npm run build` | ✅ 58 modules, clean |
| `sdk/typescript npm run build` | ✅ Clean compile |
| Python SDK installable | ✅ `setup.py` shim added |

## Smoke Test Results

| Step | Result |
|------|--------|
| Setup + Login | ✅ token returned |
| Create tenant/agent/key | ✅ all created |
| Allow toolcall (slack/channel.list, risk=1) | ✅ `decision=allow` |
| Deny toolcall (unknown/do.stuff) | ✅ `decision=deny` |
| Approve toolcall (jira/issue.delete, risk=8) | ✅ `decision=approve` |
| **Platform admin approves (BUG-016)** | ✅ `status=approved` |
| Execute | ✅ `decision=allow`, execution recorded |
| Audit trail | ✅ event detail returned |
| CSV export | ✅ headers + data |
| Bundle export | ✅ event_count correct |
| **Connectors show actions (BUG-029)** | ✅ slack: `msg.post,channel.list,approval.request`; jira: `issue.create,issue.list,issue.delete` |
| **Disabled tenant (BUG-021)** | ✅ returns `"tenant disabled"` (FORBIDDEN), not "invalid API key" |
| Alert rule create (deny_spike) | ✅ rule created |

## Bug Fix Checklist

### Blockers — ALL FIXED

| ID | Status | Summary | Files Changed | Verification |
|----|--------|---------|---------------|-------------|
| BUG-001 | ✅ | InviteAccept uses unauthPost (no stale JWT redirect) | `web/console/src/api.ts`, `web/console/src/pages/InviteAccept.tsx` | Navigate to `/invite/accept` with stale token in localStorage |
| BUG-002 | ✅ | PasswordReset uses unauthPost, adds loading states | `web/console/src/pages/PasswordReset.tsx` | Navigate to `/reset` with stale token |
| BUG-003 | ✅ | Java SDK riskScore: Double → Integer | `sdk/java/.../ToolCallRequest.java` | `riskScore(8)` serializes as `8` not `8.0` |
| BUG-004 | ✅ | Connectors page uses `/admin/connectors` | `web/console/src/pages/Connectors.tsx` | Load `/connectors` page |
| BUG-005 | ✅ | Console-UI Helm relaxed security context for nginx | `deploy/helm/console-ui/templates/deployment.yaml`, `values.yaml` | `helm template` renders valid deployment |
| BUG-006 | ✅ | Console-API Helm includes CONSOLE_JWT_SECRET + CORS | `deploy/helm/console-api/values.yaml` | Check values.yaml env section |

### High — ALL FIXED

| ID | Status | Summary | Files Changed | Verification |
|----|--------|---------|---------------|-------------|
| BUG-007 | ✅ | Rulebuilder `>=` → `>` for MaxRiskAutoApprove | `pkg/policy/rulebuilder.go`, `rulebuilder_test.go` | `go test ./pkg/policy/ -v` — new test `TestEvaluateWithRuleBuilder_AllowsAtExactThreshold` passes |
| BUG-008 | ✅ | `api.delete()` handles 204 No Content | `web/console/src/api.ts` | Delete operations no longer crash |
| BUG-009 | ✅ | CI builds console-api, console-ui, alert-worker | `.github/workflows/ci.yml` | Inspect workflow file |
| BUG-010 | ✅ | Overview falls back to empty state, has loading var | `web/console/src/pages/Overview.tsx` | Load Overview when API unavailable |
| BUG-011 | ✅ | RotateAPIKeys INSERT includes `is_primary=true` | `pkg/console/store.go` | RotateAPIKeys → ListAPIKeys shows primary |
| BUG-012 | ✅ | README curl uses double-quoted `-d` for variable expansion | `readme.md` | Copy-paste README examples with `$TENANT_ID` set |
| BUG-013 | ✅ | INTERNAL_AUTH_TOKEN aligned to `dev-internal-token-change-me` | `.env.example` | Compare `.env.example` with `docs/LOCAL_TESTING.md` |
| BUG-014 | ✅ | Java + TypeScript getEvent() returns ToolCallEvent model | `sdk/java/.../ToolCallEvent.java` (new), `sdk/typescript/src/models.ts`, `client.ts`, `index.ts` | Compile both SDKs |
| BUG-015 | ✅ | OPA risk_overrides plumbed end-to-end | `pkg/policy/client.go` | Code review confirms field mapping |
| BUG-016 | ✅ | Platform admin bypasses approver check for approve/deny | `cmd/console-api/main.go` | Smoke test: admin approves for new tenant without registering as approver |
| BUG-017 | ✅ | docker-compose `S3_ENDPOINT` → `EVIDENCE_S3_ENDPOINT` | `deploy/docker-compose.yml` | Archiver connects to MinIO |

### Medium — ALL FIXED

| ID | Status | Summary | Files Changed | Verification |
|----|--------|---------|---------------|-------------|
| BUG-018 | ✅ | Alerts create form sends correct payload | `web/console/src/pages/Alerts.tsx` | Create alert rule from Alerts page |
| BUG-019 | ✅ | Policies fetchTenants wrapped in try/catch | `web/console/src/pages/Policies.tsx` | Load Policies page when API slow |
| BUG-020 | ✅ | InviteAccept token field editable | `web/console/src/pages/InviteAccept.tsx` | Navigate to `/invite/accept` without `?token=` |
| BUG-021 | ✅ | Disabled tenant returns "tenant disabled" | `pkg/auth/dbkeys.go` | Smoke test confirms `FORBIDDEN: "tenant disabled"` |
| BUG-022 | ✅ | TenantDetail error shows back link | `web/console/src/pages/TenantDetail.tsx` | Load `/tenants/nonexistent-id` |
| BUG-023 | ✅ | Python LangChain _arun uses asyncio.to_thread | `sdk/python/openclause/langchain.py` | `python3 -m py_compile` passes |
| BUG-024 | ✅ | Go SDK poll loop uses exponential backoff | `pkg/sdk/client/client.go` | Code review confirms backoff logic |
| BUG-025 | ✅ | Go SDK isRetryable handles non-JSON 409 | `pkg/sdk/client/client.go` | Code review confirms string check |
| BUG-026 | ✅ | Approvals Helm includes CONNECTOR_SLACK_URL | `deploy/helm/approvals/values.yaml` | Check values.yaml |
| BUG-027 | ✅ | Gateway Helm includes PUBLIC_APPROVALS_URL | `deploy/helm/gateway/values.yaml` | Check values.yaml |
| BUG-028 | ✅ | Alert-worker Helm chart created | `deploy/helm/alert-worker/` (new) | `helm template oc-alert-worker ./deploy/helm/alert-worker` |
| BUG-029 | ✅ | Remote connectors registered with known actions | `pkg/connectors/registry.go`, `cmd/gateway/main.go` | Smoke test: `GET /v1/connectors` shows actions for slack/jira |
| BUG-030 | ✅ | API keys table colSpan 7 → 8 | `web/console/src/pages/TenantDetail.tsx` | View tenant with no API keys |
| BUG-031 | ✅ | MinIO env vars aligned to EVIDENCE_S3_* | `deploy/docker-compose.yml` | MinIO starts with correct creds |
| BUG-032 | ✅ | Python SDK from_dict uses `is not None` | `sdk/python/openclause/models.py` | `python3 -m py_compile` passes |
| BUG-033 | ✅ | Dockerfile EXPOSE uses SERVICE_PORT arg | `Dockerfile` | `docker inspect` shows correct port |
| BUG-034 | ✅ | Python SDK setup.py shim for editable installs | `sdk/python/setup.py` (new) | `pip install -e .` succeeds |
| BUG-035 | ✅ | Python SDK installs as openclause (not UNKNOWN) | `sdk/python/setup.py` | `pip show openclause` shows correct name |
| BUG-036 | ✅ | Users page Create button has disabled/loading state | `web/console/src/pages/Users.tsx` | Double-click Create doesn't duplicate |

### Low — ALL FIXED

| ID | Status | Summary | Files Changed | Verification |
|----|--------|---------|---------------|-------------|
| BUG-037 | ✅ | Login page has "Forgot password?" link | `web/console/src/pages/Login.tsx` | Visual check |
| BUG-038 | ✅ | Console-UI Dockerfile copies lockfile, uses npm ci | `web/console/Dockerfile` | Build reproducibility |
| BUG-039 | ✅ | Events JWT decoding wrapped in useMemo | `web/console/src/pages/Events.tsx` | No per-render parsing |
| BUG-040 | ✅ | TypeScript SDK checks actual response body size | `sdk/typescript/src/client.ts` | Chunked responses properly sized |
| BUG-041 | ✅ | ListAlertEvents includes next_attempt_at column | `pkg/console/store.go` | `GET /admin/tenants/{id}/alerts/events` returns field |
| BUG-042 | ✅ | Approvals/connector Helm readiness probes use /readyz | `deploy/helm/approvals/templates/deployment.yaml`, connector helm templates | Check deployment YAML |
| BUG-043 | ✅ | TypeScript AuthenticationError includes status code | `sdk/typescript/src/errors.ts`, `client.ts` | Compile check |

## Files Changed (49 total)

### Backend (Go)
- `cmd/console-api/main.go` — BUG-016 (platform_admin bypass approver check)
- `cmd/gateway/main.go` — BUG-029 (register remote connectors with actions)
- `pkg/auth/dbkeys.go` — BUG-021 (remove tenant status JOIN from key lookup)
- `pkg/connectors/registry.go` — BUG-029 (Registry stores actions for remote connectors)
- `pkg/console/store.go` — BUG-011 (RotateAPIKeys is_primary), BUG-041 (next_attempt_at)
- `pkg/policy/client.go` — BUG-015 (risk_overrides field)
- `pkg/policy/rulebuilder.go` — BUG-007 (>= → >)
- `pkg/policy/rulebuilder_test.go` — Updated + new test for threshold boundary
- `pkg/sdk/client/client.go` — BUG-024 (exponential backoff), BUG-025 (retryable 409)

### Web Console (React/TypeScript)
- `web/console/src/api.ts` — BUG-008 (delete 204), unauthPost helper
- `web/console/src/pages/Alerts.tsx` — BUG-018 (correct payload)
- `web/console/src/pages/Connectors.tsx` — BUG-004 (correct API path)
- `web/console/src/pages/Events.tsx` — BUG-039 (useMemo)
- `web/console/src/pages/InviteAccept.tsx` — BUG-001 (unauthPost), BUG-020 (editable token)
- `web/console/src/pages/Login.tsx` — BUG-037 (forgot password link)
- `web/console/src/pages/Overview.tsx` — BUG-010 (error handling)
- `web/console/src/pages/PasswordReset.tsx` — BUG-002 (unauthPost + loading states)
- `web/console/src/pages/Policies.tsx` — BUG-019 (error handling)
- `web/console/src/pages/TenantDetail.tsx` — BUG-022 (back link), BUG-030 (colSpan)
- `web/console/src/pages/Users.tsx` — BUG-036 (loading state)
- `web/console/Dockerfile` — BUG-038 (lockfile + npm ci)

### SDKs
- `sdk/java/.../ToolCallRequest.java` — BUG-003 (Integer riskScore)
- `sdk/java/.../ToolCallEvent.java` — BUG-014 (new model)
- `sdk/java/.../OpenClauseClient.java` — BUG-014 (getEvent returns ToolCallEvent)
- `sdk/python/setup.py` — BUG-034/035 (new shim)
- `sdk/python/openclause/models.py` — BUG-032 (is not None)
- `sdk/python/openclause/langchain.py` — BUG-023 (asyncio.to_thread)
- `sdk/typescript/src/client.ts` — BUG-014 (ToolCallEvent), BUG-040 (body size), BUG-043 (auth error)
- `sdk/typescript/src/models.ts` — BUG-014 (ToolCallEvent interface)
- `sdk/typescript/src/errors.ts` — BUG-043 (statusCode + body)
- `sdk/typescript/src/index.ts` — BUG-014 (export ToolCallEvent)

### Infrastructure
- `.env.example` — BUG-013 (INTERNAL_AUTH_TOKEN aligned)
- `.github/workflows/ci.yml` — BUG-009 (console-api/ui/alert-worker builds)
- `Dockerfile` — BUG-033 (SERVICE_PORT arg)
- `deploy/docker-compose.yml` — BUG-017/031 (EVIDENCE_S3_* env vars)
- `deploy/helm/alert-worker/` — BUG-028 (new chart)
- `deploy/helm/approvals/values.yaml` — BUG-026 (CONNECTOR_SLACK_URL)
- `deploy/helm/approvals/templates/deployment.yaml` — BUG-042 (/readyz)
- `deploy/helm/connector-jira/templates/deployment.yaml` — BUG-042 (/readyz)
- `deploy/helm/connector-slack/templates/deployment.yaml` — BUG-042 (/readyz)
- `deploy/helm/console-api/values.yaml` — BUG-006 (JWT_SECRET + CORS)
- `deploy/helm/console-ui/templates/deployment.yaml` — BUG-005 (security context)
- `deploy/helm/console-ui/values.yaml` — BUG-005 (port)
- `deploy/helm/gateway/values.yaml` — BUG-027 (PUBLIC_APPROVALS_URL)

### Docs
- `readme.md` — BUG-012 (curl quoting), approval happy path added
