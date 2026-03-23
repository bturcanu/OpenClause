# Test Coverage Sweep

Updated: 2026-03-23

Legend:
- `COVERED` = concrete invariants exercised by existing/new automated tests
- `PARTIAL` = meaningful coverage exists, but an important branch/contract still lacks direct tests
- `MISSING` = no meaningful automated coverage

## A. Console Bootstrap + Auth

| Item | Status | Tests | Notes |
| --- | --- | --- | --- |
| 1. Setup wizard | `COVERED` | `cmd/console-api/setup_config_test.go`; `cmd/console-api/store_backed_handlers_test.go` | Covers URL/config helpers, `/setup/status` initialized vs uninitialized vs store failure, `/setup/initialize` invalid input, successful bootstrap, persisted admin user/password, and idempotent `409 already initialized`. |
| 2. Login / JWT issuance | `COVERED` | `cmd/console-api/auth_provider_test.go`; `cmd/console-api/auth_sessions_test.go`; `pkg/console/jwt_test.go` | Covers platform-admin no-tenant scope, single-tenant positive scope, non-platform `403` with 0 tenants, `409` with multiple tenants, case-insensitive/trimmed bearer parsing, revoked-session middleware, and `now >= exp` rejection boundary. |
| 3. Invite accept + password reset | `COVERED` | `cmd/console-api/invite_handlers_test.go`; `cmd/console-api/store_backed_handlers_test.go`; `pkg/console/store_integration_test.go`; `pkg/console/store_test.go` | Covers invalid-token `400` vs internal `500`, invite list omitting raw token, invite token/reset token hashed at rest, invite accept creating user + tenant role + password, reset confirm success, and reset missing-user mapping to `400`. |
| 4. `RequireEnv` / config robustness | `COVERED` | `pkg/config/postgres_test.go` | Covers successful lookup, typed `ErrRequiredEnvNotSet`, and explicit no-panic behavior on missing env vars. |

## B. Tenant Management

| Item | Status | Tests | Notes |
| --- | --- | --- | --- |
| 5. Tenant create/list/status | `COVERED` | `cmd/console-api/store_backed_handlers_test.go`; `pkg/console/store_integration_test.go` | Covers name validation, platform-admin create/list, tenant-scoped list behavior, newest-first paging assumptions, and active/disabled status transitions. |
| 6. Agents CRUD | `COVERED` | `cmd/console-api/store_backed_handlers_test.go`; `cmd/console-api/agents_status_handler_test.go`; `pkg/console/store_integration_test.go`; `pkg/console/agent_status_integration_test.go` | Covers create/list ordering, missing-tenant `404`, tenant-scoped active/disabled status toggles, wrong-tenant `404`, persisted `status` field, and optional `include_disabled=false` filtering. Product behavior now intentionally preserves audit history with status toggles instead of hard-delete. |
| 7. API keys | `COVERED` | `cmd/console-api/store_backed_handlers_test.go`; `pkg/console/store_integration_test.go`; `pkg/auth/middleware_test.go`; `pkg/auth/apikey_test.go` | Covers create/list/revoke/rotate-primary workflow, lookup of rotated key, revoked key rejection, bearer/X-API-Key whitespace tolerance, and tenant-disabled middleware behavior. |

## C. Toolcall Governance

| Item | Status | Tests | Notes |
| --- | --- | --- | --- |
| 8. Gateway main path invariants | `COVERED` | `cmd/gateway/main_test.go`; `cmd/console-api/store_backed_handlers_test.go` | Covers allow/deny/approve paths, nil-policy fail-closed deny, duplicate-idempotency replay without second execution, execute replay/idempotency, awaiting-approval `409`, wrong-tenant `404`, and session/trace identifiers persisting into evidence-backed console retrieval. |
| 9. Evidence store | `COVERED` | `pkg/evidence/hashchain_test.go`; `pkg/evidence/canonical_test.go`; `pkg/evidence/store_integration_test.go` | Covers advisory execution lock serialization, append-only hash-chain invariants, `CheckIdempotency` replay for approval + execution flows, `LinkExecutionToParent` idempotency, request field round-trip, and “latest result wins” selection without row multiplication. |

## D. Approvals

| Item | Status | Tests | Notes |
| --- | --- | --- | --- |
| 10. Approvals store | `COVERED` | `pkg/approvals/store_test.go`; `pkg/approvals/store_integration_test.go`; `pkg/approvals/handlers_slack_test.go` | Covers create request, list pending ordering, approve/deny transitions, one-time grant consumption, approve/approve race, approve/deny race, expiry exclusion from pending, and concurrent outbox claiming. |
| 11. Slack + webhook notifier | `COVERED` | `pkg/approvals/notifier_test.go`; `pkg/approvals/handlers_slack_test.go` | Covers CloudEvent payload, HMAC signature header, retry-then-success flow, slack delivery, backoff schedule behavior, and fail-closed runtime handling for malformed webhook outbox rows with empty or unknown `secret_ref`. |

## E. Notification Routing

| Item | Status | Tests | Notes |
| --- | --- | --- | --- |
| 12. Tenant notification config handlers | `COVERED` | `cmd/console-api/notification_config_test.go`; `cmd/console-api/notification_config_handler_test.go` | Covers slack-only, webhook-only, mixed, and empty config normalization; webhook URL/secret validation; GET stored config; PUT normalization/persistence; and store-failure `500` handling. |
| 13. Alert-worker loop | `COVERED` | `cmd/alert-worker/main_test.go`; `cmd/alert-worker/run_tick_integration_test.go`; `pkg/console/store_integration_test.go`; `cmd/console-api/alerts_handlers_test.go` | Covers partial-sink success semantics, all-sinks-fail errors, retry scheduling (`attempt_count`, `last_error`, `next_attempt_at`), pending-to-sent transitions, and handler/UI-facing retry metadata fields. |

## F. Audit + Exports

| Item | Status | Tests | Notes |
| --- | --- | --- | --- |
| 14. Audit list + event detail | `COVERED` | `cmd/console-api/store_backed_handlers_test.go`; `cmd/console-api/session_handlers_test.go`; `pkg/console/store_test.go` | Covers handler-level since/until local-time parsing, tenant scoping, trace/session filters, and event detail returning `policy_result`, `hash`, and `prev_hash`. |
| 15. Exports | `COVERED` | `cmd/console-api/export_handlers_test.go`; `cmd/console-api/session_handlers_test.go` | Covers CSV export tenant scoping + since/until propagation, bundle since/until propagation, bundle fail-closed `>10,000` contract with `details.reason=range_too_large`, missing-tenant structured errors, and session CSV `404` on missing session. |

## G. Alerts

| Item | Status | Tests | Notes |
| --- | --- | --- | --- |
| 16. Alert rules | `COVERED` | `cmd/console-api/alerts_handlers_test.go` | Covers create/update/delete, canonicalized config, tenant/body scoping, and wrapped `ErrAlertRuleNotFound` -> `404`. |
| 17. Alert events | `COVERED` | `cmd/console-api/alerts_handlers_test.go`; `pkg/console/store_integration_test.go`; `cmd/alert-worker/run_tick_integration_test.go` | Covers retry metadata fields (`attempt_count`, `last_error`, `next_attempt_at`) on tenant-scoped and global endpoints, plus store/worker persistence transitions. |

## H. Analytics

| Item | Status | Tests | Notes |
| --- | --- | --- | --- |
| 18. Overview + tenant analytics | `COVERED` | `cmd/console-api/tenant_analytics_handlers_test.go`; `pkg/console/store_integration_test.go`; `pkg/console/analytics_integration_test.go` | Covers range/bucket/top-agent parsing, nil-summary stable JSON shape, seeded multi-event totals, deterministic trend buckets across 15/30/60/120-minute intervals, per-agent ordering with `agent_id` tie-break, risk-heatmap totals, and non-null JSON arrays for handler responses. |

## I. Connectors Catalog

| Item | Status | Tests | Notes |
| --- | --- | --- | --- |
| 19. Registry invariants + console proxy parity | `COVERED` | `pkg/connectors/registry_test.go`; `pkg/connectors/builtins/registry_contract_test.go`; `cmd/console-api/connectors_handler_test.go` | Covers execution, remote-over-builtin dedupe, sorted/deduped catalog ordering, concurrent exec access, console proxy stripping `base_url`, and the exact builtin connector name/action contract. |

## J. SDKs

| Item | Status | Tests | Notes |
| --- | --- | --- | --- |
| 20. TypeScript SDK | `COVERED` | `sdk/typescript/tests/client.test.ts`; `sdk/typescript/tests/mcp.test.ts` | Covers request construction, auth/timeout error mapping, wait-for-approval polling, execution-result mapping, and MCP output mapping. |
| 21. Python SDK | `COVERED` | `sdk/python/tests/test_client.py` | Covers request serialization, risk validation, request building with trace header, timeout/auth/API error mapping, body-size guard, wait-for-approval polling, core imports without LangChain extras, helpful `openclause[langchain]` import guidance, and the CI matrix now runs an explicit Python 3.9 install/import contract alongside the Python SDK unit suite. |
| 22. Java SDK | `COVERED` | `sdk/java/src/test/java/dev/openclause/sdk/OpenClauseClientTest.java` | Covers model serialization/deserialization, request construction against a local HTTP server, and `401`/`500` error mapping to `APIException`. |
| 23. Go SDK | `COVERED` | `pkg/sdk/client/client_test.go` | Covers submit request construction + auto-generated identifiers, execute structured error mapping, and wait-for-approval retry vs permanent-conflict behavior. |

## E2E Smoke

| Flow | Status | Coverage | Notes |
| --- | --- | --- | --- |
| Minimal end-to-end smoke | `COVERED` | `scripts/demo.sh`; `scripts/e2e-curl-happy-path.sh`; `cmd/gateway/approval_roundtrip_integration_test.go` | Covers shell-based live-stack smokes plus an in-process Go approval roundtrip proving `toolcall -> pending approval -> grant -> execute -> console session timeline/CSV linkage` with real Postgres-backed evidence, approvals, and console stores. |

## True Backlog

- Keep watching future Linux `browser-smoke` CI artifacts and only harden selectors if a later real stack/runtime difference appears; the first uploaded Linux run was already green.
- Deepen the remaining thin session-detail and tenant-detail UI branches beyond the now-covered malformed contract, stale-state, malformed alert/notification payloads, export, diagnostics-clearing, and repeated-failure-triage paths.
- Expand the current auth/date/analytics fuzz smokes and deterministic query/bucketing edge matrices into broader property/fuzz coverage where it buys real signal.
