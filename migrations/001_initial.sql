-- ═══════════════════════════════════════════════════════════════════════════
-- 001_initial.sql — Agentic Access Governance schema
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

CREATE TABLE IF NOT EXISTS tool_events (
    event_id        TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    agent_id        TEXT NOT NULL,
    tool            TEXT NOT NULL,
    action          TEXT NOT NULL,
    payload_json    JSONB NOT NULL,
    payload_canon   BYTEA NOT NULL,
    risk_score      INTEGER NOT NULL DEFAULT 0,
    decision        TEXT NOT NULL,       -- 'allow', 'deny', 'approve'
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

-- ── Tool results (execution outcomes) ───────────────────────────────────────

CREATE TABLE IF NOT EXISTS tool_results (
    id              BIGSERIAL PRIMARY KEY,
    event_id        TEXT NOT NULL REFERENCES tool_events(event_id),
    tenant_id       TEXT NOT NULL,
    status          TEXT NOT NULL,        -- 'success', 'error', 'timeout'
    output_json     JSONB,
    error_msg       TEXT DEFAULT '',
    duration_ms     BIGINT NOT NULL DEFAULT 0,
    result_canon    BYTEA,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tool_results_event ON tool_results(event_id);

-- ── Approval requests ───────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS approval_requests (
    id          TEXT PRIMARY KEY,
    event_id    TEXT NOT NULL,
    tenant_id   TEXT NOT NULL,
    agent_id    TEXT NOT NULL,
    tool        TEXT NOT NULL,
    action      TEXT NOT NULL,
    resource    TEXT DEFAULT '',
    risk_score  INTEGER NOT NULL DEFAULT 0,
    reason      TEXT DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'pending',  -- 'pending', 'approved', 'denied', 'expired'
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
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
    tenant_id               TEXT NOT NULL,
    approver                TEXT NOT NULL,
    scope_tool              TEXT NOT NULL,
    scope_action            TEXT NOT NULL,
    scope_resource_pattern  TEXT DEFAULT '',
    scope_tenant_id         TEXT NOT NULL,
    scope_agent_id          TEXT DEFAULT '',
    max_uses                INTEGER NOT NULL DEFAULT 1,
    uses_left               INTEGER NOT NULL DEFAULT 1,
    expires_at              TIMESTAMPTZ NOT NULL,
    granted_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_approval_grants_tenant
    ON approval_grants(tenant_id, uses_left, expires_at);

-- ── Policy versions (track bundle deployments) ─────────────────────────────

CREATE TABLE IF NOT EXISTS policy_versions (
    id          BIGSERIAL PRIMARY KEY,
    bundle_hash TEXT NOT NULL,
    version     TEXT NOT NULL,
    deployed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes       TEXT DEFAULT ''
);

-- ── Seed data ───────────────────────────────────────────────────────────────

INSERT INTO tenants (id, name) VALUES
    ('tenant1', 'Acme Corp'),
    ('tenant2', 'Globex Inc')
ON CONFLICT (id) DO NOTHING;

INSERT INTO agents (id, tenant_id, name) VALUES
    ('agent-1', 'tenant1', 'Research Assistant'),
    ('agent-2', 'tenant1', 'Ops Bot'),
    ('agent-3', 'tenant2', 'Support Agent')
ON CONFLICT (id) DO NOTHING;
