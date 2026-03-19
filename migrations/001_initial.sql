-- ═══════════════════════════════════════════════════════════════════════════
-- 001_initial.sql — OpenClause schema
-- ═══════════════════════════════════════════════════════════════════════════

-- ── Tenants ─────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS tenants (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    config      JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Agents ──────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS agents (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    name        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    labels      JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agents_tenant ON agents(tenant_id);

-- ── API Keys (DB-backed, hashed) ────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS api_keys (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    name        TEXT NOT NULL DEFAULT '',
    key_prefix  TEXT NOT NULL,
    key_hash    TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at  TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_api_keys_tenant ON api_keys(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(key_prefix) WHERE status = 'active';
CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash) WHERE status = 'active';

-- ── Console Users ───────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT,
    name          TEXT NOT NULL DEFAULT '',
    slack_user_id TEXT,
    status        TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Slack approvals must map to a single user; enforce uniqueness when set.
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_slack_user_id_unique
    ON users(slack_user_id)
    WHERE slack_user_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS user_roles (
    id        TEXT PRIMARY KEY,
    user_id   TEXT NOT NULL REFERENCES users(id),
    tenant_id TEXT,
    role      TEXT NOT NULL CHECK (role IN ('platform_admin', 'tenant_admin', 'approver', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, tenant_id, role),
    -- HIGH-02: Non-platform_admin roles MUST have a tenant_id.
    CHECK (role = 'platform_admin' OR tenant_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_user_roles_user ON user_roles(user_id);

-- ── Invites (invite acceptance) ─────────────────────────────────────────

CREATE TABLE IF NOT EXISTS invites (
    token       TEXT PRIMARY KEY,
    email       TEXT NOT NULL,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    role        TEXT NOT NULL CHECK (role IN ('tenant_admin', 'approver', 'viewer')),
    name        TEXT DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_invites_tenant_expires ON invites(tenant_id, expires_at);
CREATE INDEX IF NOT EXISTS idx_invites_email_expires ON invites(email, expires_at);

-- ── Password resets (minimum viable) ────────────────────────────────────

CREATE TABLE IF NOT EXISTS password_resets (
    token       TEXT PRIMARY KEY,
    email       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_password_resets_email_expires ON password_resets(email, expires_at);

-- ── Sessions ────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    agent_id    TEXT NOT NULL,
    metadata    JSONB DEFAULT '{}',
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_sessions_tenant ON sessions(tenant_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_agent ON sessions(tenant_id, agent_id, started_at DESC);

-- ── Tool events (one per incoming request) ──────────────────────────────────

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

CREATE INDEX IF NOT EXISTS idx_tool_events_session
    ON tool_events(tenant_id, session_id, received_at ASC)
    WHERE session_id != '';

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
    max_uses                INTEGER NOT NULL DEFAULT 1 CHECK (max_uses >= 1),
    uses_left               INTEGER NOT NULL DEFAULT 1 CHECK (uses_left >= 0),
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
    notify_kind           TEXT NOT NULL,
    notify_url            TEXT DEFAULT '',
    secret_ref            TEXT DEFAULT '',
    slack_channel         TEXT DEFAULT '',
    status                TEXT NOT NULL DEFAULT 'pending',
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
    id           BIGSERIAL PRIMARY KEY,
    tenant_id    TEXT REFERENCES tenants(id),
    bundle_hash  TEXT NOT NULL,
    version      TEXT NOT NULL,
    policy_data  JSONB DEFAULT '{}',
    deployed_by  TEXT DEFAULT '',
    deployed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes        TEXT DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_policy_versions_tenant
    ON policy_versions(tenant_id, deployed_at DESC);

-- ── Alert rules ─────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS alert_rules (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    name        TEXT NOT NULL,
    rule_type   TEXT NOT NULL CHECK (rule_type IN ('deny_spike', 'approve_backlog', 'unusual_tool', 'volume_spike')),
    config      JSONB NOT NULL DEFAULT '{}',
    notify_kind TEXT NOT NULL DEFAULT 'webhook',
    notify_target TEXT NOT NULL DEFAULT '',
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alert_rules_tenant ON alert_rules(tenant_id, enabled);

-- ── Alert events ────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS alert_events (
    id          TEXT PRIMARY KEY,
    rule_id     TEXT NOT NULL REFERENCES alert_rules(id),
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    severity    TEXT NOT NULL DEFAULT 'warning',
    message     TEXT NOT NULL,
    details     JSONB DEFAULT '{}',
    notified    BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alert_events_tenant ON alert_events(tenant_id, created_at DESC);

-- ── Alert schema evolution (idempotent) ─────────────────────────────────────
-- These columns support the deny_spike worker: tracking delivery + retries.

ALTER TABLE alert_rules
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE alert_events
  ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending',
  ADD COLUMN IF NOT EXISTS attempt_count INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS last_error TEXT DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_alert_events_rule_created
  ON alert_events(rule_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_alert_events_status_due
  ON alert_events(status, next_attempt_at);

-- ── Usage counters (cost/budget controls scaffold) ──────────────────────────

CREATE TABLE IF NOT EXISTS usage_counters (
    tenant_id    TEXT NOT NULL REFERENCES tenants(id),
    counter_date DATE NOT NULL DEFAULT CURRENT_DATE,
    requests     BIGINT NOT NULL DEFAULT 0,
    approvals    BIGINT NOT NULL DEFAULT 0,
    executions   BIGINT NOT NULL DEFAULT 0,
    connector_calls BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, counter_date)
);

-- ── API key schema evolution (expires + primary) ──────────────────────────
ALTER TABLE api_keys
  ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS is_primary BOOLEAN NOT NULL DEFAULT false;

CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_primary_active
  ON api_keys(tenant_id)
  WHERE status = 'active' AND is_primary = true;
