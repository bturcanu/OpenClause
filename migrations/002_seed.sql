-- ═══════════════════════════════════════════════════════════════════════════
-- 002_seed.sql — Development seed data (do NOT run in production)
-- ═══════════════════════════════════════════════════════════════════════════

INSERT INTO tenants (id, name) VALUES
    ('tenant1', 'Acme Corp'),
    ('tenant2', 'Globex Inc')
ON CONFLICT (id) DO NOTHING;

INSERT INTO agents (id, tenant_id, name) VALUES
    ('agent-1', 'tenant1', 'Research Assistant'),
    ('agent-2', 'tenant1', 'Ops Bot'),
    ('agent-3', 'tenant2', 'Support Agent')
ON CONFLICT (id) DO NOTHING;
