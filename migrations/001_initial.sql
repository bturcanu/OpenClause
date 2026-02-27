-- ═══════════════════════════════════════════════════════════════════════════
-- 001_initial.sql — OpenClause schema
-- ═══════════════════════════════════════════════════════════════════════════

-- ── Tenants ─────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS tenants (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    config      JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Agents ──────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS agents (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    name        TEXT NOT NULL,
    labels      JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agents_tenant ON agents(tenant_id);

-- ── Tool events (one per incoming request) ──────────────────────────────────
-- NOTE: For high-volume deployments, consider adding declarative range
-- partitioning on received_at.

CREATE TABLE IF NOT EXISTS tool_events (
    event_seq        BIGSERIAL UNIQUE NOT NULL,
    event_id        TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    agent_id        TEXT NOT NULL,
    tool            TEXT NOT NULL,
    action          TEXT NOT NULL,
    payload_json    JSONB NOT NULL,
    payload_canon   BYTEA NOT NULL,
    risk_score      INTEGER NOT NULL DEFAULT 0 CHECK (risk_score >= 0 AND risk_score <= 10),
    decision        TEXT NOT NULL CHECK (decision IN ('allow', 'deny', 'approve')),
    policy_result   JSONB,
    idempotency_key TEXT NOT NULL,
    session_id      TEXT DEFAULT '',
    user_id         TEXT DEFAULT '',
    source_ip       TEXT DEFAULT '',
    trace_id        TEXT DEFAULT '',
    received_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    hash            TEXT NOT NULL,
    prev_hash       TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_events_idempotency
    ON tool_events(tenant_id, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_tool_events_tenant_ts
    ON tool_events(tenant_id, received_at DESC);

CREATE INDEX IF NOT EXISTS idx_tool_events_tenant_agent_ts
    ON tool_events(tenant_id, agent_id, received_at DESC);

CREATE INDEX IF NOT EXISTS idx_tool_events_decision
    ON tool_events(decision);

CREATE INDEX IF NOT EXISTS idx_tool_events_tool_action
    ON tool_events(tool, action);

CREATE INDEX IF NOT EXISTS idx_tool_events_tenant_seq
    ON tool_events(tenant_id, event_seq ASC);

-- ── Tool results (execution outcomes) ───────────────────────────────────────

CREATE TABLE IF NOT EXISTS tool_results (
    id              BIGSERIAL PRIMARY KEY,
    event_id        TEXT NOT NULL REFERENCES tool_events(event_id),
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    status          TEXT NOT NULL CHECK (status IN ('success', 'error', 'timeout')),
    output_json     JSONB,
    error_msg       TEXT DEFAULT '',
    duration_ms     BIGINT NOT NULL DEFAULT 0,
    result_canon    BYTEA,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_results_event ON tool_results(event_id);

-- ── Tool execution links (approval resume endpoint) ──────────────────────────

CREATE TABLE IF NOT EXISTS tool_executions (
    parent_event_id      TEXT PRIMARY KEY REFERENCES tool_events(event_id),
    execution_event_id   TEXT NOT NULL UNIQUE REFERENCES tool_events(event_id),
    consumed_grant_id    TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tool_executions_execution
    ON tool_executions(execution_event_id);

-- ── Approval requests ───────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS approval_requests (
    id          TEXT PRIMARY KEY,
    event_id    TEXT NOT NULL REFERENCES tool_events(event_id),
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    agent_id    TEXT NOT NULL,
    tool        TEXT NOT NULL,
    action      TEXT NOT NULL,
    resource    TEXT DEFAULT '',
    risk_score  INTEGER NOT NULL DEFAULT 0,
    reason      TEXT DEFAULT '',
    deny_reason TEXT DEFAULT '',
    denied_by   TEXT DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'denied', 'expired')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_approval_requests_tenant_status
    ON approval_requests(tenant_id, status);

CREATE INDEX IF NOT EXISTS idx_approval_requests_event
    ON approval_requests(event_id);

-- ── Approval grants ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS approval_grants (
    id                      TEXT PRIMARY KEY,
    request_id              TEXT NOT NULL REFERENCES approval_requests(id),
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    approver                TEXT NOT NULL,
    scope_tool              TEXT NOT NULL,
    scope_action            TEXT NOT NULL,
    scope_resource_pattern  TEXT DEFAULT '',
    scope_tenant_id         TEXT NOT NULL,
    scope_agent_id          TEXT DEFAULT '',
    max_uses                INTEGER NOT NULL DEFAULT 1,
    uses_left               INTEGER NOT NULL DEFAULT 1,
    expires_at              TIMESTAMPTZ NOT NULL,
    granted_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_approval_grants_tenant
    ON approval_grants(tenant_id, uses_left, expires_at);

-- ── Notification outbox (reliable webhook/slack fanout) ─────────────────────

CREATE TABLE IF NOT EXISTS approval_notification_outbox (
    id                    TEXT PRIMARY KEY,
    approval_request_id   TEXT NOT NULL REFERENCES approval_requests(id),
    tenant_id             TEXT NOT NULL REFERENCES tenants(id),
    event_id              TEXT NOT NULL REFERENCES tool_events(event_id),
    trace_id              TEXT DEFAULT '',
    tool                  TEXT NOT NULL,
    action                TEXT NOT NULL,
    resource              TEXT DEFAULT '',
    risk_score            INTEGER NOT NULL DEFAULT 0,
    risk_factors          JSONB DEFAULT '[]',
    reason                TEXT DEFAULT '',
    approver_group        TEXT DEFAULT '',
    approval_url          TEXT NOT NULL,
    notify_kind           TEXT NOT NULL,          -- webhook | slack
    notify_url            TEXT DEFAULT '',
    secret_ref            TEXT DEFAULT '',
    slack_channel         TEXT DEFAULT '',
    status                TEXT NOT NULL DEFAULT 'pending', -- pending|processing|sent|failed
    attempt_count         INTEGER NOT NULL DEFAULT 0,
    next_attempt_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error            TEXT DEFAULT '',
    sent_at               TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_approval_notification_outbox_due
    ON approval_notification_outbox(status, next_attempt_at);

-- ── Evidence archival checkpoints ───────────────────────────────────────────

CREATE TABLE IF NOT EXISTS evidence_archive_checkpoints (
    tenant_id         TEXT PRIMARY KEY REFERENCES tenants(id),
    last_archived_at  TIMESTAMPTZ NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    last_hash         TEXT NOT NULL DEFAULT '',
    last_event_seq    BIGINT NOT NULL DEFAULT 0,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Policy versions (track bundle deployments) ─────────────────────────────

CREATE TABLE IF NOT EXISTS policy_versions (
    id          BIGSERIAL PRIMARY KEY,
    bundle_hash TEXT NOT NULL,
    version     TEXT NOT NULL,
    deployed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes       TEXT DEFAULT ''
);
