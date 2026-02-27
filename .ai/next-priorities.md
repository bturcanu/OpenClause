# Next Logical Priorities Tracking

Branch: `main` (merged from `feature/next-priorities`)

## Phase 1 — Close approval loop (release blocker)

- [x] Add `POST /v1/toolcalls/{event_id}/execute` with idempotent-by-event semantics, grant consumption, and append-only execution evidence.
  - **Acceptance:** existing approved request can be executed exactly once logically; repeated calls return prior execution; no overwrite of original evidence rows.
  - **Commit:** `c242cda`
- [x] Update `api/openapi.yaml` to document `/execute` endpoint and responses.
  - **Acceptance:** OpenAPI includes success + awaiting approval + invalid decision/not found responses.
  - **Commit:** `c242cda`
- [x] Add integration + race tests for execute flow.
  - **Acceptance:** happy-path and concurrent execute behavior pass deterministically.
  - **Commit:** `c242cda`

## Phase 2 — Webhook notifications (CloudEvents + signed delivery)

- [x] Add transactional outbox write when approval request is created.
  - **Acceptance:** request + outbox rows are committed atomically.
  - **Commit:** `c242cda`
- [x] Add dispatcher worker with retry/backoff and restart-safe processing.
  - **Acceptance:** failed deliveries are retried and successful deliveries are not re-sent.
  - **Commit:** `c242cda`
- [x] Emit CloudEvents structured payload for `oc.approval.requested`.
  - **Acceptance:** payload includes required approval/event/tenant/risk/url metadata.
  - **Commit:** `c242cda`
- [x] Add HMAC-SHA256 signature header `X-OC-Signature-256`.
  - **Acceptance:** signature computed from exact raw body bytes.
  - **Commit:** `c242cda`
- [x] Extend policy output + data for notification routing directives.
  - **Acceptance:** notify routing is data-driven from policy output.
  - **Commit:** `c242cda`
- [x] Add unit/dispatcher tests for envelope, signing, retry/idempotency.
  - **Acceptance:** tests cover retries on 500 and single-send-on-success semantics.
  - **Commit:** `c242cda`

## Phase 3 — Slack one-click approvals

- [x] Extend Slack connector with interactive approval message action.
  - **Acceptance:** Block Kit message contains summary, approve/deny actions, correlation IDs.
  - **Commit:** `c242cda`
- [x] Add `POST /v1/integrations/slack/interactions` in approvals service.
  - **Acceptance:** verifies Slack signature/timestamp, handles approve/deny actions, updates response message.
  - **Commit:** `c242cda`
- [x] Add tests for Slack signature verification + approve handler.
  - **Acceptance:** fixed-fixture signature test and handler path test pass.
  - **Commit:** `c242cda`

## Phase 4 — Evidence archival to MinIO/S3

- [x] Add archiver component with incremental checkpoints.
  - **Acceptance:** verifies chain, builds bundle, uploads to object storage, advances checkpoint.
  - **Commit:** `c242cda`
- [x] Add one-shot CLI mode for local tenant archival.
  - **Acceptance:** command runs once for specified tenant without daemon mode.
  - **Commit:** `c242cda`
- [x] Add bundle-builder tests (+ optional MinIO integration).
  - **Acceptance:** unit test verifies stable bundle content and checkpoint progression.
  - **Commit:** `c242cda`

## Phase 5 — More connectors + Connector SDK

- [x] Add allowlisted read actions in Slack/Jira connectors.
  - **Acceptance:** `slack.channel.list` and `jira.issue.list` return deterministic mock outputs.
  - **Commit:** `c242cda`
- [x] Publish connector SDK/pattern and template connector.
  - **Acceptance:** shared helper covers validation, timeout, structured errors/logging, internal auth.
  - **Commit:** `c242cda`

## Phase 6 — Approver auth + basic RBAC

- [x] Add minimal approver identity + per-tenant allowlist enforcement.
  - **Acceptance:** approve/deny paths reject unauthorized approvers (API + Slack).
  - **Commit:** `c242cda`

## Phase 7 — Agent SDK

- [x] Add thin SDK (Go) with submit/poll/execute helpers.
  - **Acceptance:** helper handles idempotency key + trace id and exposes approve/execute loop ergonomics.
  - **Commit:** `c242cda`

## LLM-assisted summaries scaffold

- [x] Add summary interface with deterministic default + feature-flagged extension hook.
  - **Acceptance:** notifications include deterministic sanitized summary + raw structured fields.
  - **Commit:** `c242cda`

## Docs + Quality Gates

- [x] Update README for approve/execute, webhooks + signatures, Slack interactions, archival, SDK usage.
  - **Acceptance:** setup/verification instructions are end-to-end and current.
  - **Commit:** `72a207f`
- [x] Run and pass `go test ./...` and `make policy-test`.
  - **Acceptance:** all tests green with no regressions from prior 67 fixes.
  - **Commit:** `b48fd35` (final verification + merged main)

## PR

- [x] Integrate deliverables into `main`.
  - **PR:** Not opened (merged directly to `main` via `b48fd35`)

## Post-implementation hardening fixes

- [x] Fail closed when gateway idempotency check errors.
  - **Acceptance:** `POST /v1/toolcalls` returns internal error on idempotency storage failures and does not continue execution path.
  - **Commit:** `c242cda`
- [x] Return internal error when `/execute` replay polling hits storage errors.
  - **Acceptance:** `POST /v1/toolcalls/{event_id}/execute` does not return misleading `409 awaiting approval` when replay lookup fails.
  - **Commit:** `c242cda`

