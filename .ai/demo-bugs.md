# Demo Runtime & UI Bug Tracker

Branch: `fix/demo-runtime-and-ui`

---

## A — Execute flow broken (POST /v1/toolcalls/{EVENT_ID}/execute → INTERNAL_ERROR)

- [x] **Repro**: Toolcall with decision=approve created event + approval; execute returns `{"code":"INTERNAL_ERROR","message":"failed to retrieve event","retryable":true}`
- [x] **Root cause**: `evidence.GetEvent` scans `decision` TEXT column directly into `types.Decision` (named string alias). pgx v5 can fail to scan TEXT into named string types depending on codec registration. The scan error is swallowed as a generic internal error.
- [x] **Fix**: Scan `decision` into intermediate `string`, then convert `env.Decision = types.Decision(decisionStr)`. Same pattern as `CheckIdempotency` already uses.
- [x] **Tests**: `TestGetEvent_ScanDecision`, execute flow integration tests in `cmd/gateway/main_test.go`.

## B — Disabled tenant still calls gateway

- [x] **Repro**: After `POST /admin/tenants/tenant1/status {"status":"disabled"}`, gateway still accepts toolcalls with `X-API-Key: sk-test-key-1`.
- [x] **Root cause**: `CompositeKeyStore` tries env `KeyStore` first. Env store maps `sk-test-key-1 → tenant1` without checking `tenants.status` in DB. DB store checks `t.status = 'active'` but is never reached when env store matches first.
- [x] **Fix**: Add `TenantStatusChecker` interface to auth middleware. After key lookup succeeds, verify tenant is active in DB. Returns 403 "tenant disabled" if not.
- [x] **Tests**: `TestAPIKeyAuth_TenantDisabled`, integration tests for enable/disable cycle.

## C — Console UI issues (empty lists + Invalid Date)

- [x] **Repro**: Audit Trail page empty, Approvals Queue empty, date fields show "Invalid Date".
- [x] **Root cause (events)**: API returns `event_id` + `received_at`, UI expects `id` + `created_at`.
- [x] **Root cause (approvals)**: `ListPending` with empty `tenant_id` queries `WHERE tenant_id = ''` (no rows match). Platform admin gets empty string from `tenantScope()`.
- [x] **Root cause (dates)**: Fields `created_at`, `fired_at` etc. are undefined in UI because API returns different field names. `new Date(undefined)` → "Invalid Date".
- [x] **Fix (events)**: Update UI interfaces/field references to use `event_id`/`received_at`. Add safe `formatDate()` helper.
- [x] **Fix (approvals)**: Modify `approvals.ListPending` to omit `tenant_id` filter when tenant is empty string (platform admin sees all).
- [x] **Fix (dates)**: Add `formatDate(val)` utility in `api.ts` that safely handles undefined/null/invalid strings.
- [x] **Additional field fixes**: Alerts `fired_at→created_at`, Policies `created_at→deployed_at`/`created_by→deployed_by`, TenantDetail `label→name`/`slug→id`/`revoked→status`.
- [x] **Tests**: Date formatter unit tests, UI field mapping verification.
