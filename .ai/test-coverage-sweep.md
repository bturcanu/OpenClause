# Test Coverage Sweep

Updated: 2026-03-21

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
| 6. Agents CRUD | `PARTIAL` | `cmd/console-api/store_backed_handlers_test.go`; `pkg/console/store_integration_test.go` | Covers create/list ordering and missing-tenant `404` path. Delete coverage remains backlog because the product currently exposes create/list plus status updates, not a delete handler. |
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
| 11. Slack + webhook notifier | `PARTIAL` | `pkg/approvals/notifier_test.go`; `pkg/approvals/handlers_slack_test.go` | Covers CloudEvent payload, HMAC signature header, retry-then-success flow, slack delivery, and backoff schedule behavior. Remaining gap: no fail-closed runtime test for malformed/legacy outbox rows with missing `secret_ref`. |

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
| 18. Overview + tenant analytics | `PARTIAL` | `cmd/console-api/tenant_analytics_handlers_test.go`; `pkg/console/store_integration_test.go` | Covers range/bucket/top-agent parsing, nil-summary stable JSON shape, and empty-state store behavior (11 risk buckets + zero totals). Remaining gap: no seeded-data integration test yet for timeseries bucket totals/per-agent ordering across multiple events. |

## I. Connectors Catalog

| Item | Status | Tests | Notes |
| --- | --- | --- | --- |
| 19. Registry invariants + console proxy parity | `PARTIAL` | `pkg/connectors/registry_test.go`; `cmd/console-api/connectors_handler_test.go` | Covers execution, remote-over-builtin dedupe, sorted/deduped catalog ordering, concurrent exec access, and console proxy stripping `base_url`. Remaining gap: no exhaustive assertion that every builtin connector action list exactly matches expected product contract. |

## J. SDKs

| Item | Status | Tests | Notes |
| --- | --- | --- | --- |
| 20. TypeScript SDK | `COVERED` | `sdk/typescript/tests/client.test.ts`; `sdk/typescript/tests/mcp.test.ts` | Covers request construction, auth/timeout error mapping, wait-for-approval polling, execution-result mapping, and MCP output mapping. |
| 21. Python SDK | `PARTIAL` | `sdk/python/tests/test_client.py` | Covers request serialization, risk validation, request building with trace header, timeout/auth/API error mapping, body-size guard, and wait-for-approval polling. Remaining backlog: no automated packaging/installability matrix beyond current `requires-python >=3.10` declaration, so Python 3.9 compatibility remains an explicit backlog item. |
| 22. Java SDK | `COVERED` | `sdk/java/src/test/java/dev/openclause/sdk/OpenClauseClientTest.java` | Covers model serialization/deserialization, request construction against a local HTTP server, and `401`/`500` error mapping to `APIException`. |
| 23. Go SDK | `COVERED` | `pkg/sdk/client/client_test.go` | Covers submit request construction + auto-generated identifiers, execute structured error mapping, and wait-for-approval retry vs permanent-conflict behavior. |

## E2E Smoke

| Flow | Status | Coverage | Notes |
| --- | --- | --- | --- |
| Minimal end-to-end smoke | `PARTIAL` | `scripts/demo.sh`; `scripts/e2e-curl-happy-path.sh` | The repo already has shell-based smoke coverage for live-stack flows. I did not add a new Go e2e test because the sweep prioritized deterministic handler/store coverage first; a true approval-roundtrip integration test is still valid backlog if the team wants a CI-grade harness around the live stack. |

## True Backlog

- Agent delete lifecycle: product/API surface still lacks a delete handler, so the checklist item remains only partially coverable without changing behavior.
- Analytics seeded-data summary/timeseries verification: empty-state and handler parsing are covered, but a richer multi-event analytics fixture would improve confidence in bucket math and per-agent ranking.
- Connector catalog completeness audit: ordering/deduping is now covered, but there is no “golden list” test that locks every builtin connector action set.
- Python 3.9 installability: `sdk/python/pyproject.toml` still declares `requires-python >=3.10`; that compatibility gap remains product backlog, not a missing unit test.
- CI-grade golden-path approval e2e: shell smokes exist, but there is still no dedicated automated test for `toolcall -> approval -> execute -> console session/event detail` across the full local stack.
