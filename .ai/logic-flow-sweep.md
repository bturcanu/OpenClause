# Logic Flow Sweep

Date: 2026-03-20
Branch: `fix/logic-flow-sweep`
Status: In progress

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

## Fix Plan

- [x] Build initial flow inventory and tracker.
- [x] Review Flow A end-to-end and fix confirmed issues.
- [ ] Review Flow B.
- [ ] Review Flow C.
- [ ] Review Flow D.
- [ ] Review Flow E.
- [ ] Review Flow F.
- [ ] Review Flow G.
- [ ] Review Flow H.
- [ ] Review Flow I.
- [ ] Review Flow J.
- [ ] Run full smoke flow on fresh stack and capture outputs.
- [ ] Final docs/README contract pass.

## Per-Bug Checklist

### LF-001
- [x] Reproduce by code inspection.
- [x] Fix route helpers.
- [x] Add tests.
- [ ] Smoke via console logs in fresh stack.

### LF-002
- [x] Reproduce by code inspection.
- [x] Fix setup input normalization.
- [x] Add tests.
- [ ] Smoke via setup wizard request.

### LF-003
- [x] Reproduce by code inspection.
- [x] Fix canonicalization / case-insensitive auth path.
- [x] Add tests.
- [ ] Smoke via mixed-case login.

## Files Changed

- `cmd/console-api/main.go`
- `cmd/console-api/setup_config_test.go`
- `cmd/console-api/auth_provider_test.go`
- `pkg/console/store.go`
- `pkg/console/store_test.go`
- `.ai/logic-flow-sweep.md`

## Verification Evidence

- `go test ./... -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/console-api`, `ok github.com/bturcanu/OpenClause/pkg/console`
- `go test -race ./... -count=1`
  - Pass
  - Key output: `ok github.com/bturcanu/OpenClause/cmd/gateway`, `ok github.com/bturcanu/OpenClause/pkg/console`
- `opa test policy/bundles/v0/ policy/tests/ -v`
  - Pass
  - Key output: `PASS: 19/19`
- `npm --prefix web/console install`
  - Pass
  - Key output: `up to date in 316ms`
- `npm --prefix web/console run build`
  - Pass
  - Key output: `✓ built in 567ms`
- `npm --prefix sdk/typescript install`
  - Pass
  - Key output: `up to date in 395ms`
- `npm --prefix sdk/typescript run build`
  - Pass
  - Key output: `tsc`
- `PYTHONPATH=sdk/python python3 -m unittest discover -s sdk/python/tests -v`
  - Pass
  - Key output: `Ran 16 tests in 0.014s`, `OK`
- `gradle -p sdk/java test`
  - Pass
  - Key output: `BUILD SUCCESSFUL in 620ms`

## Smoke Evidence

- Pending.
