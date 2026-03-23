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
- Known recent UI bugs found by tests:
  - Auth/setup/reset/invite labels were not properly bound to inputs.
  - Tenant search/create labels were not properly bound to inputs.
  - Overview showed a misleading "no activity" empty state on timeseries partial failure.

## Workstreams

| Workstream | Objective | Priority | Status |
| --- | --- | --- | --- |
| Backend invariants | Lock down auth, scope, approval, evidence, export, analytics invariants | P0 | In progress |
| Console UI actions | Cover operator actions and error/partial states | P0 | In progress |
| API/UI contracts | Detect drift between console expectations and API payloads | P0 | Planned |
| Critical e2e smokes | Prove a few full cross-service flows in CI | P1 | Partial |
| Property/fuzz tests | Find edge classes in date, filter, analytics, export, and parsing logic | P1 | Planned |
| Observability | Catch and triage escaped bugs fast in dev/prod | P1 | Planned |

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

- Add API/UI contract fixtures for console analytics, sessions, approvals, and users.
- Finish remaining thin console action coverage in the most stateful branches of tenant detail and session detail.
- Add regression tests whenever local/manual console verification finds a mismatch.

### P1

- Add a tiny Playwright smoke pack for the 3-4 most valuable flows.
- Add property/fuzz coverage for date filters, query builders, and analytics buckets.
- Add CI reporting that highlights which critical workstreams failed.

### P2

- Expand observability and local reproduction tooling for UI/API contract failures.
- Add richer failure-triage docs once the core coverage stabilizes.

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
