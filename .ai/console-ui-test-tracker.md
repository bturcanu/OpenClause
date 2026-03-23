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
| Shared API helpers | `src/api.ts` | unit | In progress |
| Shared API helpers | `src/api.ts` | unit | Done |
| Shared UI helpers | `src/ui.tsx` | unit/component | Done |
| App shell / auth gating | `src/App.tsx` | route/integration | Done |
| Auth pages | `Login`, `SetupWizard`, `InviteAccept`, `PasswordReset` | page integration | Done |
| Overview | `pages/Overview.tsx` | page integration | Done |
| Audit Trail | `pages/Events.tsx` | page integration | Done |
| Event detail | `pages/EventDetail.tsx` | smoke + focused render | Done |
| Sessions | `pages/Sessions.tsx` | page integration | Done |
| Session detail | `pages/SessionTimeline.tsx` | page integration | Done |
| Tenants | `pages/Tenants.tsx` | smoke + focused interactions | Done |
| Tenant detail | `pages/TenantDetail.tsx` | page integration | Done |
| Users | `pages/Users.tsx` | smoke + focused interactions | Done |
| Approvals | `pages/Approvals.tsx` | smoke + focused interactions | Done |
| Alerts | `pages/Alerts.tsx` | smoke render | Done |
| Policies | `pages/Policies.tsx` | smoke + focused interactions | Done |
| Connectors | `pages/Connectors.tsx` | smoke + focused interactions | Done |

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
- `src/pages/Sessions.test.tsx`
- `src/pages/SessionTimeline.test.tsx`
- `src/pages/TenantDetail.test.tsx`
- `src/pages/operator-pages-smoke.test.tsx`

## Notes

- Prefer deterministic mocked API responses over browser/network-heavy tests.
- Avoid broad snapshots; every new test should assert visible behavior or an invariant.
- If a page is too stateful for deep coverage in the first pass, add smoke coverage now and leave a specific follow-up item here.
