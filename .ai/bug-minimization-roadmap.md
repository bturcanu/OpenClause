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
- `web/console` currently passes `17` test files / `111` tests locally on this branch.
- A tiny Playwright smoke pack now covers the 4 highest-value browser flows in CI (`login -> overview`, `tenant create/agent/key`, `audit trail -> event detail`, `sessions -> execution linkage`).
- The browser smoke pack now also passes locally on this macOS host when Playwright uses the installed Google Chrome channel outside the sandbox, which removes the old bundled-Chromium blocker for manual repro work.
- GitHub Actions now runs the full repo verification matrix plus a short Go fuzz-smoke layer for analytics, auth-header, and timestamp parsers.
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

- Added a `Vitest + React Testing Library` harness for `web/console` and expanded it to `97` deterministic tests.
- Converted high-value console pages from mostly smoke coverage to action-level integration coverage.
- Hardened multiple UI/API boundaries against array-vs-wrapped payload drift for users, sessions, analytics, and tenant detail subresources.
- Added export-contract, query-builder, copy-helper, and datetime edge tests in the console layer.
- Surfaced request/correlation IDs in `APIClientError` messages so failing console requests are easier to triage from the UI.
- Added repeated API-failure telemetry in [`web/console/src/api.ts`](../web/console/src/api.ts) so the console now emits structured warnings after the third consecutive failure for the same method/path and resets that counter after a success.
- Added a tiny Playwright browser smoke pack plus CI jobs for `console-ui`, `browser-smoke`, and explicit Python 3.9 SDK install/import verification.
- Hardened the browser smoke pack after the first real local run, and added Playwright HTML/test-result artifact upload in CI so the first Linux failures are inspectable instead of opaque.
- Reviewed the first uploaded Linux `browser-smoke` artifact (`3d63373456c090aaea428c0f1067ca740b5e1eff`) and confirmed it was already green (`4/4`), so there was no CI-specific selector drift to harden from that run.
- Added deterministic and fuzz-backed analytics parser coverage for `range`, `bucket_minutes`, and `top_agents`.
- Expanded deterministic edge coverage into query/date helpers and analytics bucketing, including half-hour bucket integration coverage in `pkg/console`.
- Extended the deterministic edge layer again with non-finite query-value filtering in the shared query builder plus two-hour analytics bucket invariants.
- Deepened the session-detail and tenant-detail diagnostics branches so first-failure helper text, repeated-failure banners, and successful-retry clearing are now all covered by focused page tests instead of manual spot checks.
- Expanded analytics bucketing coverage again with a deterministic 15/30/60/120-minute bucket matrix that proves totals stay stable and buckets stay interval-aligned.
- Deepened session detail again so malformed nested approval/execution payloads are now dropped fail-closed while valid timeline rows stay visible, and the UI exposes richer export/session triage with error codes plus occurrence counts.
- Tightened tenant notification contract handling so only operationally valid `slack` and `webhook` routes survive normalization; malformed or incomplete rows are now dropped with explicit diagnostics instead of silently pre-filling misleading config.
- Expanded the deterministic edge layer again with generated query/date helper matrices in `web/console` and a broader analytics bucket-alignment matrix covering `5/10/15/20/30/45/60/90/120/180` minute windows.
- Bound the remaining high-traffic console form labels to their controls and added label-driven tests so those accessibility regressions now break CI.
- Fixed stale notification-config state in tenant detail so failed refetches no longer leave misleading routing values on screen.
- Fixed session-detail and tenant-detail race/contract edges:
  - malformed wrapped session summaries now fail closed instead of rendering a broken run context
  - stale alert/analytics responses are ignored when the operator quickly changes tabs or ranges
- Added structured triage logging for session-detail and tenant-detail partial failures so export/session/tenant fetch problems now emit contextual `console.warn` events with stage, tenant/session ids, and request ids where available.
- Added operator-facing repeated-failure banners in session detail and tenant detail, including copyable diagnostics payloads with the latest stage and request ID when available.
- Expanded CI so the available repo tests are represented in GitHub Actions: Go tests, Go race, Go fuzz-smoke, console unit/build/browser smoke, TypeScript SDK build/tests, Python 3.9 install/import plus Python SDK unit tests, Java SDK tests, OPA tests, and lint.

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
- Fixed: [`web/console/src/pages/SessionTimeline.tsx`](../web/console/src/pages/SessionTimeline.tsx) treated malformed fulfilled timeline payloads as “no events matched your filters,” which was misleading; the page now reports a timeline load failure instead and logs the contract issue.
- Fixed: [`web/console/src/ui.tsx`](../web/console/src/ui.tsx) used to serialize `NaN` / `Infinity` into query strings, which could leak obviously invalid filter values into URLs; `buildQuery()` now drops non-finite numbers.
- Fixed: [`web/console/src/pages/TenantDetail.tsx`](../web/console/src/pages/TenantDetail.tsx) trusted fulfilled notification-config and tenant-alert payloads too much; malformed notification config now fails closed, malformed alert rows are dropped with an honest contract warning, and operators get copyable “Latest diagnostics” on the first visible failure instead of only after repeats.
- Fixed: [`web/console/src/pages/SessionTimeline.tsx`](../web/console/src/pages/SessionTimeline.tsx) now distinguishes between “all timeline rows were malformed” vs “some rows were ignored,” preserving valid rows when possible and failing closed when none are usable.
- Fixed: [`web/console/src/pages/SessionTimeline.tsx`](../web/console/src/pages/SessionTimeline.tsx) and [`web/console/src/pages/TenantDetail.tsx`](../web/console/src/pages/TenantDetail.tsx) only pushed repeated-failure details into the browser console; the UI now exposes copyable diagnostics so operators can paste the latest stage/request ID into bug reports without digging through DevTools.
- Fixed: repeated-failure banners in [`web/console/src/pages/SessionTimeline.tsx`](../web/console/src/pages/SessionTimeline.tsx) and [`web/console/src/pages/TenantDetail.tsx`](../web/console/src/pages/TenantDetail.tsx) could stay stale after the repeated stage recovered if a different one-off issue was still active; triage state now recomputes from the remaining active issues instead of only clearing when *all* issues disappear.
- Fixed: [`web/console/src/pages/SessionTimeline.tsx`](../web/console/src/pages/SessionTimeline.tsx) trusted nested `approval` / `execution` payloads too much, which could render misleading subpanels from malformed fulfilled timeline rows.
- Fixed: [`web/console/src/pages/TenantDetail.tsx`](../web/console/src/pages/TenantDetail.tsx) accepted notification rows that were structurally typed but operationally unusable, such as `slack` entries without a channel or `webhook` entries without a secret ref.
- Clarified: fuzzing found an over-strict test assumption in [`cmd/console-api/parsing_fuzz_test.go`](../cmd/console-api/parsing_fuzz_test.go), not a product bug; `parseRangeDuration` is allowed to return large positive durations because the handler clamps them later.
- Fixed: [`readme.md`](../readme.md) had a stale CI/CD numbering slip after an editor-side compare/paste; the workflow list is back to the intended `1..13` sequence and the file has no conflict markers.
- Fixed: [`cmd/console-api/tenant_analytics_handlers.go`](../cmd/console-api/tenant_analytics_handlers.go) let huge raw `range` hour values overflow `time.Duration` negative before the handler clamp, which fuzzing reproduced with `range=2700000`.
- Fixed: [`web/console/vite.config.ts`](../web/console/vite.config.ts) needed an explicit `src/**/*.test.{ts,tsx}` include so Vitest would not try to execute the Playwright smoke spec as a unit suite.
- Local environment note, not product bug: bundled Chromium is still blocked by the sandboxed macOS MachPort restriction, but local browser smokes are now unblocked by using Playwright’s system-Chrome channel outside the sandbox on this host.

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

- Keep converting any newly discovered manual/UI mismatch into a reproducer test plus the smallest localized fix.

### P1

- Keep watching future Linux `browser-smoke` artifacts and only harden selectors if a later CI/runtime difference appears; the first uploaded Linux run was already green (`4/4`).
- Add CI reporting that highlights which critical workstreams failed.

### P2

- Continue expanding property/fuzz coverage where failures justify the cost, especially if new bugs cluster around date filters, query builders, or analytics bucketing again.
- Continue expanding observability only when new triage pain appears beyond the current request-id/error-code/occurrence diagnostics and repeated API-failure logging.
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
