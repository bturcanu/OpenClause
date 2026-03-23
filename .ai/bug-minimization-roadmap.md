# Bug-Minimization Roadmap

Branch: `codex/bug-minimization-roadmap`

## Goal

Drive the repo toward "as few escaped bugs as realistically possible" by combining:

1. Stronger invariants and contracts
2. Better test layering
3. Targeted regression capture
4. Runtime observability for anything tests miss

This is not a claim of "zero bugs." It is a tracker for systematically shrinking the unknown-risk surface.

## Operating Principles

- Test invariants, not just happy paths.
- Every real bug gets a reproducer test before or with the fix.
- Prefer deterministic unit/store/handler/page tests over flaky end-to-end coverage.
- Use end-to-end tests sparingly for only the highest-value cross-service flows.
- Treat API contracts and UI/API boundary behavior as first-class test targets.
- Add observability for anything too expensive or too stateful to prove exhaustively in tests.

## Current Baseline

- Strong backend coverage exists across console/auth/store/evidence/approvals/alerts/gateway/SDKs.
- Console UI now has a `Vitest + React Testing Library` harness with focused integration coverage.
- `web/console` currently passes `17` test files / `90` tests locally on this branch.
- A tiny Playwright smoke pack now covers the 4 highest-value browser flows in CI (`login -> overview`, `tenant create/agent/key`, `audit trail -> event detail`, `sessions -> execution linkage`).
- Known recent UI bugs found by tests:
  - Auth/setup/reset/invite labels were not properly bound to inputs.
  - Tenant search/create labels were not properly bound to inputs.
  - Overview showed a misleading "no activity" empty state on timeseries partial failure.
  - Users page dropped valid array-form `/admin/users` and `/admin/auth-sessions` payloads because it only trusted wrapped objects.
  - Tenant detail had brittle contract handling for approvers and tenant alert subresources when payloads arrived in array-vs-wrapped variants.
  - Alerts, Policies, Connectors, and Session detail still had visible labels that were not actually bound to their controls, weakening keyboard and screen-reader flows until the latest sweep.
  - Tenant detail could keep showing stale notification-routing state after a later refetch failed, instead of honestly dropping back to the unavailable state.
  - Events, Sessions, Users, Approvals, and Policies still had visible high-traffic filter/form labels that were not actually bound to their controls until the newest operator-label sweep.

## Workstreams

| Workstream | Objective | Priority | Status |
| --- | --- | --- | --- |
| Backend invariants | Lock down auth, scope, approval, evidence, export, analytics invariants | P0 | In progress |
| Console UI actions | Cover operator actions and error/partial states | P0 | In progress |
| API/UI contracts | Detect drift between console expectations and API payloads | P0 | In progress |
| Critical e2e smokes | Prove a few full cross-service flows in CI | P1 | In progress |
| Property/fuzz tests | Find edge classes in date, filter, analytics, export, and parsing logic | P1 | In progress |
| Observability | Catch and triage escaped bugs fast in dev/prod | P1 | In progress |

## Completed In This Branch

- Added a `Vitest + React Testing Library` harness for `web/console` and expanded it to `90` deterministic tests.
- Converted high-value console pages from mostly smoke coverage to action-level integration coverage.
- Hardened multiple UI/API boundaries against array-vs-wrapped payload drift for users, sessions, analytics, and tenant detail subresources.
- Added export-contract, query-builder, copy-helper, and datetime edge tests in the console layer.
- Surfaced request/correlation IDs in `APIClientError` messages so failing console requests are easier to triage from the UI.
- Added repeated API-failure telemetry in [`web/console/src/api.ts`](../web/console/src/api.ts) so the console now emits structured warnings after the third consecutive failure for the same method/path and resets that counter after a success.
- Added a tiny Playwright browser smoke pack plus CI jobs for `console-ui`, `browser-smoke`, and explicit Python 3.9 SDK install/import verification.
- Added deterministic and fuzz-backed analytics parser coverage for `range`, `bucket_minutes`, and `top_agents`.
- Bound the remaining high-traffic console form labels to their controls and added label-driven tests so those accessibility regressions now break CI.
- Fixed stale notification-config state in tenant detail so failed refetches no longer leave misleading routing values on screen.
- Fixed session-detail and tenant-detail race/contract edges:
  - malformed wrapped session summaries now fail closed instead of rendering a broken run context
  - stale alert/analytics responses are ignored when the operator quickly changes tabs or ranges

## Findings

- Fixed: [`web/console/src/pages/Users.tsx`](../web/console/src/pages/Users.tsx) incorrectly treated valid array-form `/admin/users` and `/admin/auth-sessions` payloads as empty data.
- Fixed: [`web/console/src/pages/TenantDetail.tsx`](../web/console/src/pages/TenantDetail.tsx) was too strict about approver and tenant alert payload shapes at the API boundary.
- Fixed: [`web/console/src/api.ts`](../web/console/src/api.ts) did not preserve `x-request-id` / `x-correlation-id` values from failed responses, which made console-side bug reports harder to trace.
- Fixed: [`web/console/src/pages/Alerts.tsx`](../web/console/src/pages/Alerts.tsx), [`web/console/src/pages/Policies.tsx`](../web/console/src/pages/Policies.tsx), [`web/console/src/pages/Connectors.tsx`](../web/console/src/pages/Connectors.tsx), and [`web/console/src/pages/SessionTimeline.tsx`](../web/console/src/pages/SessionTimeline.tsx) exposed labels visually without binding them to their controls, which made keyboard-first operator flows weaker and masked regressions in tests.
- Fixed: [`web/console/src/pages/Policies.tsx`](../web/console/src/pages/Policies.tsx) previously dropped wrapped `{ versions: [...] }` payloads, and [`web/console/src/pages/Connectors.tsx`](../web/console/src/pages/Connectors.tsx) trusted malformed connector `actions` payloads too much.
- Fixed: [`web/console/src/pages/TenantDetail.tsx`](../web/console/src/pages/TenantDetail.tsx) could preserve stale notification-config UI after a later refetch lost that subresource, which made the page look healthier than the current data really was.
- Fixed: [`web/console/src/pages/Events.tsx`](../web/console/src/pages/Events.tsx), [`web/console/src/pages/Sessions.tsx`](../web/console/src/pages/Sessions.tsx), [`web/console/src/pages/Users.tsx`](../web/console/src/pages/Users.tsx), [`web/console/src/pages/Approvals.tsx`](../web/console/src/pages/Approvals.tsx), and [`web/console/src/pages/Policies.tsx`](../web/console/src/pages/Policies.tsx) still showed visible operator labels without real control bindings, which weakened keyboard navigation and let tests rely on DOM-neighbor helpers instead of true labels.
- Corrected tracker drift: the approvals notifier malformed-webhook-row fail-closed coverage already exists, and Python SDK metadata now declares `requires-python >=3.9`.
- Fixed: [`web/console/src/pages/SessionTimeline.tsx`](../web/console/src/pages/SessionTimeline.tsx) previously trusted “successful” summary payloads too much, so malformed wrapped responses like `{ "session": null }` could still render a broken session-detail shell instead of failing closed.
- Fixed: [`cmd/console-api/tenant_analytics_handlers.go`](../cmd/console-api/tenant_analytics_handlers.go) let huge raw `range` hour values overflow `time.Duration` negative before the handler clamp, which fuzzing reproduced with `range=2700000`.
- Fixed: [`web/console/vite.config.ts`](../web/console/vite.config.ts) needed an explicit `src/**/*.test.{ts,tsx}` include so Vitest would not try to execute the Playwright smoke spec as a unit suite.
- Local limitation, not product bug: Playwright browser smokes still cannot launch bundled Chromium on this macOS host because the browser process is blocked by a MachPort permission error. The smoke pack is intended to verify on CI/Linux, and the local failure is documented rather than masked.

## Phase Plan

### Phase 1: Lock Critical Invariants

- Auth/session invariants:
  - token/session persistence
  - logout/revoke behavior
  - tenant ambiguity handling
  - role-gated flows
- Tenant scope invariants:
  - platform vs tenant-scoped actions
  - wrong-tenant fail-closed behavior
- Approval invariants:
  - approve/deny/execute linkage
  - expiry and replay boundaries
- Export invariants:
  - tenant requirement
  - time-window correctness
  - large-range fail-closed behavior
- Analytics invariants:
  - stable empty-state shape
  - deterministic seeded-data totals and ordering

Definition of done:

- Every critical backend flow has at least one deterministic invariant test.
- Every escaped bug in these areas is converted into a permanent regression test.

### Phase 2: Finish Console Operator Coverage

- Expand UI integration tests so every meaningful operator action is exercised:
  - form submit
  - toggle/state transitions
  - URL/query sync
  - retry and partial-failure behavior
  - copy/export interactions
  - empty/error/loading states
- Focus first on:
  - Audit Trail
  - Sessions / Session detail
  - Tenant detail
  - Approvals
  - Alerts
  - Users

Definition of done:

- All major operator pages have action-level coverage, not just smoke render tests.
- The console test tracker can point to a focused test for each high-value action.

### Phase 3: Add Contract Tests at Boundaries

- Freeze representative API payloads for:
  - analytics
  - events
  - sessions
  - approvals
  - users
  - tenant detail subresources
- Add tests that validate the console still handles:
  - array vs wrapped payloads where applicable
  - missing optional fields
  - partial success / partial failure
  - structured API errors with metadata

Definition of done:

- API shape drift breaks CI before it breaks the UI.

### Phase 4: Add Small Critical E2E Smokes

- Keep this intentionally small:
  - login -> overview
  - toolcall -> approval -> execute -> session/event visibility
  - tenant detail key/agent lifecycle smoke
  - audit filtering/export smoke

Definition of done:

- CI proves the most important cross-service paths, not every path.

### Phase 5: Property/Fuzz + Edge Exploration

- Candidate targets:
  - datetime conversion helpers
  - filter/query builders
  - analytics bucketing
  - export range validation
  - JWT / bearer / API-key parsing
  - risk-score thresholds

Definition of done:

- Edge-class bugs become measurably harder to introduce.

### Phase 6: Observability and Escape Hatches

- Add or improve:
  - request IDs
  - structured error logs
  - UI-visible correlation IDs where useful
  - alerting on repeated client/API failures
  - dashboards for failing paths

Definition of done:

- Escaped bugs are fast to detect, isolate, and reproduce.

## Immediate Backlog

### P0

- Finish the remaining thin contract fixtures for session detail partial-failure branches beyond the now-covered request-id, policies/connectors, and tenant-notification cases.
- Deepen the most stateful tenant-detail and session-detail branches that still rely on broad smoke-plus-one-action coverage after the latest stale-response, malformed-summary, and operator-label sweeps.
- Add regression tests whenever local/manual console verification finds a mismatch.

### P1

- Keep the new Playwright smoke pack green in CI and add one follow-up pass once the first Linux run proves the selectors against a real stack.
- Expand the current deterministic edge matrices and analytics-parser fuzzing into broader property/fuzz coverage for date filters, query builders, and analytics buckets.
- Add CI reporting that highlights which critical workstreams failed.

### P2

- Expand observability and local reproduction tooling beyond request IDs and repeated API-failure logging, especially around failing export/session paths and repeated browser-side errors.
- Add richer failure-triage docs once the core coverage stabilizes.

## Source Of Truth

- Active backlog/source of truth: this roadmap plus [`.ai/console-ui-test-tracker.md`](.ai/console-ui-test-tracker.md)
- Historical snapshots/reference only: [`.ai/bug-sweep.md`](.ai/bug-sweep.md) and [`.ai/next-steps.md`](.ai/next-steps.md)

## Bug Intake Rule

For every new bug:

1. Record repro steps
2. Identify the missing invariant/contract/action test
3. Add the failing test
4. Apply the smallest fix
5. Link the fix back to this roadmap and the relevant tracker

## Verification Expectations

- `go test ./... -count=1`
- `go test -race ./... -count=1`
- `npm --prefix web/console run test`
- `npm --prefix web/console run build`
- `npm --prefix sdk/typescript run build`
- `npm --prefix sdk/typescript run test`
- `PYTHONPATH=sdk/python python3 -m unittest discover -s sdk/python/tests -v`
- `(cd sdk/java && ./gradlew test)`
- `opa test policy/bundles/v0/ policy/tests/ -v`

## Related Trackers

- [`.ai/test-coverage-sweep.md`](.ai/test-coverage-sweep.md)
- [`.ai/console-ui-test-tracker.md`](.ai/console-ui-test-tracker.md)
