# Next Logical Priorities Tracking

Branch: `feature/next-priorities`

## Phase 1 — Close approval loop (release blocker)

- [x] Add `POST /v1/toolcalls/{event_id}/execute` with idempotent-by-event semantics, grant consumption, and append-only execution evidence.
  - **Acceptance:** existing approved request can be executed exactly once logically; repeated calls return prior execution; no overwrite of original evidence rows.
  - **Commit:** _(pending)_
- [x] Update `api/openapi.yaml` to document `/execute` endpoint and responses.
  - **Acceptance:** OpenAPI includes success + awaiting approval + invalid decision/not found responses.
  - **Commit:** _(pending)_
- [x] Add integration + race tests for execute flow.
  - **Acceptance:** happy-path and concurrent execute behavior pass deterministically.
  - **Commit:** _(pending)_

## Phase 2 — Webhook notifications (CloudEvents + signed delivery)

- [x] Add transactional outbox write when approval request is created.
  - **Acceptance:** request + outbox rows are committed atomically.
  - **Commit:** _(pending)_
- [x] Add dispatcher worker with retry/backoff and restart-safe processing.
  - **Acceptance:** failed deliveries are retried and successful deliveries are not re-sent.
  - **Commit:** _(pending)_
- [x] Emit CloudEvents structured payload for `oc.approval.requested`.
  - **Acceptance:** payload includes required approval/event/tenant/risk/url metadata.
  - **Commit:** _(pending)_
- [x] Add HMAC-SHA256 signature header `X-OC-Signature-256`.
  - **Acceptance:** signature computed from exact raw body bytes.
  - **Commit:** _(pending)_
- [x] Extend policy output + data for notification routing directives.
  - **Acceptance:** notify routing is data-driven from policy output.
  - **Commit:** _(pending)_
- [x] Add unit/dispatcher tests for envelope, signing, retry/idempotency.
  - **Acceptance:** tests cover retries on 500 and single-send-on-success semantics.
  - **Commit:** _(pending)_

## Phase 3 — Slack one-click approvals

- [x] Extend Slack connector with interactive approval message action.
  - **Acceptance:** Block Kit message contains summary, approve/deny actions, correlation IDs.
  - **Commit:** _(pending)_
- [x] Add `POST /v1/integrations/slack/interactions` in approvals service.
  - **Acceptance:** verifies Slack signature/timestamp, handles approve/deny actions, updates response message.
  - **Commit:** _(pending)_
- [x] Add tests for Slack signature verification + approve handler.
  - **Acceptance:** fixed-fixture signature test and handler path test pass.
  - **Commit:** _(pending)_

## Phase 4 — Evidence archival to MinIO/S3

- [x] Add archiver component with incremental checkpoints.
  - **Acceptance:** verifies chain, builds bundle, uploads to object storage, advances checkpoint.
  - **Commit:** _(pending)_
- [x] Add one-shot CLI mode for local tenant archival.
  - **Acceptance:** command runs once for specified tenant without daemon mode.
  - **Commit:** _(pending)_
- [x] Add bundle-builder tests (+ optional MinIO integration).
  - **Acceptance:** unit test verifies stable bundle content and checkpoint progression.
  - **Commit:** _(pending)_

## Phase 5 — More connectors + Connector SDK

- [x] Add allowlisted read actions in Slack/Jira connectors.
  - **Acceptance:** `slack.channel.list` and `jira.issue.list` return deterministic mock outputs.
  - **Commit:** _(pending)_
- [x] Publish connector SDK/pattern and template connector.
  - **Acceptance:** shared helper covers validation, timeout, structured errors/logging, internal auth.
  - **Commit:** _(pending)_

## Phase 6 — Approver auth + basic RBAC

- [x] Add minimal approver identity + per-tenant allowlist enforcement.
  - **Acceptance:** approve/deny paths reject unauthorized approvers (API + Slack).
  - **Commit:** _(pending)_

## Phase 7 — Agent SDK

- [x] Add thin SDK (Go) with submit/poll/execute helpers.
  - **Acceptance:** helper handles idempotency key + trace id and exposes approve/execute loop ergonomics.
  - **Commit:** _(pending)_

## LLM-assisted summaries scaffold

- [x] Add summary interface with deterministic default + feature-flagged extension hook.
  - **Acceptance:** notifications include deterministic sanitized summary + raw structured fields.
  - **Commit:** _(pending)_

## Docs + Quality Gates

- [x] Update README for approve/execute, webhooks + signatures, Slack interactions, archival, SDK usage.
  - **Acceptance:** setup/verification instructions are end-to-end and current.
  - **Commit:** _(pending)_
- [x] Run and pass `go test ./...` and `make policy-test`.
  - **Acceptance:** all tests green with no regressions from prior 67 fixes.
  - **Commit:** _(pending)_

## PR

- [ ] Open PR with all deliverables.
  - **PR:** _(pending)_

## Post-implementation hardening fixes

- [x] Fail closed when gateway idempotency check errors.
  - **Acceptance:** `POST /v1/toolcalls` returns internal error on idempotency storage failures and does not continue execution path.
  - **Commit:** _(pending)_
- [x] Return internal error when `/execute` replay polling hits storage errors.
  - **Acceptance:** `POST /v1/toolcalls/{event_id}/execute` does not return misleading `409 awaiting approval` when replay lookup fails.
  - **Commit:** _(pending)_

