# Console UI Test Tracker

Branch: `codex/console-ui-tests`

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

## Verification

- `npm --prefix web/console run build`
- `npm --prefix web/console run test`

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

## Notes

- Prefer deterministic mocked API responses over browser/network-heavy tests.
- Avoid broad snapshots; every new test should assert visible behavior or an invariant.
- `npm --prefix web/console run test` currently covers 17 files / 67 tests.
- The expected jsdom warning `Not implemented: navigation to another Document` comes from the `apiFetch` 401 redirect test path and does not fail the suite.
- One real UI bug was found and fixed while adding coverage: [`web/console/src/pages/Tenants.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Tenants.tsx) now correctly binds the Search and Create Tenant labels to their inputs for keyboard/accessibility-safe `getByLabelText` behavior.
- Another accessibility bug was found and fixed in the auth flows: [`web/console/src/pages/Login.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Login.tsx), [`web/console/src/pages/SetupWizard.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/SetupWizard.tsx), [`web/console/src/pages/InviteAccept.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/InviteAccept.tsx), and [`web/console/src/pages/PasswordReset.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/PasswordReset.tsx) now bind visible labels to their controls so label-based navigation and testing work correctly.
- A partial-failure dashboard bug was found and fixed in [`web/console/src/pages/Overview.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Overview.tsx): when the timeseries API failed, the UI incorrectly showed the “Not enough activity yet” empty-state. The page now keeps the inline error honest instead of implying there was simply no data.
- The current batch also added deeper app/auth/session coverage: setup-status retry, login loading/error states, invite/reset backend error rendering, overview partial-success behavior, and session-detail copy/export/retry flows.
- The latest sweep added deeper action coverage for [`web/console/src/pages/TenantDetail.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/TenantDetail.tsx), [`web/console/src/pages/Events.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Events.tsx), and [`web/console/src/pages/Sessions.tsx`](/Users/bogdan/dev/personal/OpenClause/web/console/src/pages/Sessions.tsx): agent create/enable-disable, tenant status toggles, API key create/rotate/revoke, notification config validation/save, approver add/remove, tenant alert-rule create/edit/delete, Audit Trail export guardrails and clear-filters, and Sessions filter reset plus copy affordances.
- This sweep did not uncover another product-behavior bug beyond the previously fixed issues, but it substantially reduced the remaining untested operator actions on the console’s busiest pages.
