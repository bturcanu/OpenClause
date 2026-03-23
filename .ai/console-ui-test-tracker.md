# Console UI Test Tracker

Origin branch: `codex/console-ui-tests`  
Current continuation branch: `codex/bug-minimization-roadmap`

## Goal

Add a maintainable `Vitest + React Testing Library` harness for `web/console`, then cover the console UI at two levels:

1. Focused behavior tests for shared helpers and high-risk pages.
2. Smoke render tests so every page/component surface is exercised at least once.

## Test Harness

- `web/console` test runner: `vitest`
- DOM environment: `jsdom`
- UI assertions: `@testing-library/react`, `@testing-library/jest-dom`
- User interactions: `@testing-library/user-event`
- Router coverage: `MemoryRouter`

## Coverage Plan

| Area | Target | Coverage type | Status |
| --- | --- | --- | --- |
| Shared API helpers | `src/api.ts` | unit | Done |
| Shared UI helpers | `src/ui.tsx` | unit/component | Done |
| App shell / auth gating | `src/App.tsx` | route/integration | Done |
| Auth pages | `Login`, `SetupWizard`, `InviteAccept`, `PasswordReset` | page integration | Done |
| Overview | `pages/Overview.tsx` | page integration | Done |
| Audit Trail | `pages/Events.tsx` | page integration | Done |
| Event detail | `pages/EventDetail.tsx` | page integration | Done |
| Sessions | `pages/Sessions.tsx` | page integration | Done |
| Session detail | `pages/SessionTimeline.tsx` | page integration | Done |
| Tenants | `pages/Tenants.tsx` | page integration | Done |
| Tenant detail | `pages/TenantDetail.tsx` | page integration | Done |
| Users | `pages/Users.tsx` | page integration | Done |
| Approvals | `pages/Approvals.tsx` | page integration | Done |
| Alerts | `pages/Alerts.tsx` | page integration | Done |
| Policies | `pages/Policies.tsx` | page integration | Done |
| Connectors | `pages/Connectors.tsx` | page integration | Done |
| Browser smoke | `e2e/console-smoke.spec.ts` | Playwright e2e smoke | Done (CI-oriented) |

## Verification

- `npm --prefix web/console run build`
- `npm --prefix web/console run test`
- `npm --prefix web/console run test:e2e` after `./scripts/dev.sh` + `./scripts/demo.sh`

## Implemented Tests

- `src/api.test.ts`
- `src/ui.test.tsx`
- `src/App.test.tsx`
- `src/pages/auth-pages.test.tsx`
- `src/pages/Overview.test.tsx`
- `src/pages/Events.test.tsx`
- `src/pages/EventDetail.test.tsx`
- `src/pages/Sessions.test.tsx`
- `src/pages/SessionTimeline.test.tsx`
- `src/pages/Tenants.test.tsx`
- `src/pages/TenantDetail.test.tsx`
- `src/pages/Users.test.tsx`
- `src/pages/Approvals.test.tsx`
- `src/pages/Alerts.test.tsx`
- `src/pages/Policies.test.tsx`
- `src/pages/Connectors.test.tsx`
- `src/pages/operator-pages-smoke.test.tsx`
- `e2e/console-smoke.spec.ts`

## Notes

- Prefer deterministic mocked API responses over browser/network-heavy tests.
- Avoid broad snapshots; every new test should assert visible behavior or an invariant.
- `npm --prefix web/console run test` currently covers 17 files / 94 tests.
- The expected jsdom warning `Not implemented: navigation to another Document` comes from the `apiFetch` 401 redirect test path and does not fail the suite.
- `npm --prefix web/console run test:e2e` is intentionally separate from Vitest. [`web/console/vite.config.ts`](/Users/bogdan/dev/personal/OpenClause/web/console/vite.config.ts) now explicitly scopes unit/integration tests to `src/**/*.test.{ts,tsx}` so Playwright specs do not break the normal console test command.
- One real UI bug was found and fixed while adding coverage: [`web/console/src/pages/Tenants.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Tenants.tsx) now correctly binds the Search and Create Tenant labels to their inputs for keyboard/accessibility-safe `getByLabelText` behavior.
- Another accessibility bug was found and fixed in the auth flows: [`web/console/src/pages/Login.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Login.tsx), [`web/console/src/pages/SetupWizard.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/SetupWizard.tsx), [`web/console/src/pages/InviteAccept.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/InviteAccept.tsx), and [`web/console/src/pages/PasswordReset.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/PasswordReset.tsx) now bind visible labels to their controls so label-based navigation and testing work correctly.
- A partial-failure dashboard bug was found and fixed in [`web/console/src/pages/Overview.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Overview.tsx): when the timeseries API failed, the UI incorrectly showed the “Not enough activity yet” empty-state. The page now keeps the inline error honest instead of implying there was simply no data.
- The current batch also added deeper app/auth/session coverage: setup-status retry, login loading/error states, invite/reset backend error rendering, overview partial-success behavior, and session-detail copy/export/retry flows.
- The latest sweep added deeper action coverage for [`web/console/src/pages/TenantDetail.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/TenantDetail.tsx), [`web/console/src/pages/Events.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Events.tsx), and [`web/console/src/pages/Sessions.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Sessions.tsx): agent create/enable-disable, tenant status toggles, API key create/rotate/revoke, notification config validation/save, approver add/remove, tenant alert-rule create/edit/delete, Audit Trail export guardrails and clear-filters, and Sessions filter reset plus copy affordances.
- The newest contract sweep found and fixed a real API-boundary bug in [`web/console/src/pages/Users.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Users.tsx): valid array-form `/admin/users` and `/admin/auth-sessions` responses were previously rendered as empty data because the page only trusted wrapped payloads.
- The same sweep hardened [`web/console/src/pages/TenantDetail.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/TenantDetail.tsx) against array-vs-wrapped payload drift for approvers and tenant alert subresources, added explicit range-too-large export contract coverage in [`web/console/src/pages/Events.test.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Events.test.tsx), and verified request-ID propagation from failing API responses in [`web/console/src/api.test.ts`](/Users/bogdan/dev/personal/OpenClause/web/console/src/api.test.ts).
- The latest backlog pass fixed another real accessibility bug class: [`web/console/src/pages/Alerts.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Alerts.tsx), [`web/console/src/pages/Policies.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Policies.tsx), [`web/console/src/pages/Connectors.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Connectors.tsx), and [`web/console/src/pages/SessionTimeline.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/SessionTimeline.tsx) now bind their visible labels to real form controls. The updated tests use `getByLabelText` instead of DOM-neighbor helpers so those regressions are now caught automatically.
- Session detail coverage now also asserts that export failures carrying request IDs remain visible to operators, which closes one of the roadmap’s thinner error-metadata branches in [`web/console/src/pages/SessionTimeline.test.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/SessionTimeline.test.tsx).
- The newest contract pass hardened [`web/console/src/pages/Policies.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Policies.tsx) against wrapped version-history payloads and made [`web/console/src/pages/Connectors.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Connectors.tsx) normalize malformed single-string action payloads instead of crashing the catalog.
- The current sweep fixed a real stale-state bug in [`web/console/src/pages/TenantDetail.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/TenantDetail.tsx): if notification config loaded once and a later refetch failed, the page could keep showing the old routing values instead of dropping back to the unavailable state. [`web/console/src/pages/TenantDetail.test.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/TenantDetail.test.tsx) now locks that regression down.
- The same sweep bound the remaining high-traffic operator labels in [`web/console/src/pages/Events.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Events.tsx), [`web/console/src/pages/Sessions.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Sessions.tsx), [`web/console/src/pages/Users.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Users.tsx), [`web/console/src/pages/Approvals.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Approvals.tsx), and [`web/console/src/pages/Policies.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Policies.tsx); the affected tests now use true `getByLabelText` queries instead of helper fallbacks.
- The latest backlog slice added CI-oriented browser smokes for login/overview, tenant detail create flows, audit-trail drill-in, and session-detail execution linkage in [`web/console/e2e/console-smoke.spec.ts`](/Users/bogdan/dev/personal/OpenClause/web/console/e2e/console-smoke.spec.ts). The selectors were hardened after the first local run, and the smoke pack now passes locally on this macOS host when Playwright uses the installed Chrome channel outside the sandbox; CI also uploads Playwright artifacts for Linux debugging.
- Session-detail coverage now fails closed on malformed wrapped summary payloads in [`web/console/src/pages/SessionTimeline.test.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/SessionTimeline.test.tsx), which caught and fixed a real contract bug in [`web/console/src/pages/SessionTimeline.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/SessionTimeline.tsx).
- Tenant-detail coverage now proves stale alert responses cannot overwrite a fresher refetch, complementing the earlier analytics stale-response coverage in [`web/console/src/pages/TenantDetail.test.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/TenantDetail.test.tsx).
- Shared helper coverage in [`web/console/src/api.test.ts`](/Users/bogdan/dev/personal/OpenClause/web/console/src/api.test.ts) and [`web/console/src/ui.test.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/ui.test.tsx) now includes repeated-failure telemetry, broader datetime round-trips, and extra query-builder edge cases.
- The newest hardening pass closes more of the session/tenant observability backlog:
  - [`web/console/src/pages/SessionTimeline.test.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/SessionTimeline.test.tsx) now covers malformed fulfilled timeline payloads, summary request-id failures, and export triage logging.
  - [`web/console/src/pages/TenantDetail.test.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/TenantDetail.test.tsx) now asserts contextual warning logs for alert partial failures and notification-config loss on refetch.
  - [`pkg/console/analytics_integration_test.go`](/Users/bogdan/dev/personal/OpenClause/pkg/console/analytics_integration_test.go) now covers deterministic half-hour bucketing so analytics bucket regressions are not only parser-tested.
