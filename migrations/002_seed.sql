-- ═══════════════════════════════════════════════════════════════════════════
-- 002_seed.sql — Development seed data (do NOT run in production)
-- ═══════════════════════════════════════════════════════════════════════════

-- Bootstrap admin tenant
INSERT INTO tenants (id, name, status) VALUES
    ('tenant1', 'Acme Corp', 'active'),
    ('tenant2', 'Globex Inc', 'active')
ON CONFLICT (id) DO NOTHING;

INSERT INTO agents (id, tenant_id, name, status) VALUES
    ('agent-1', 'tenant1', 'Research Assistant', 'active'),
    ('agent-2', 'tenant1', 'Ops Bot', 'active'),
    ('agent-3', 'tenant2', 'Support Agent', 'active')
ON CONFLICT (id) DO NOTHING;

-- Bootstrap API keys (hashed with SHA-256)
-- sk-test-key-1 → SHA-256 hash
-- sk-test-key-2 → SHA-256 hash
INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, status) VALUES
    ('key-1', 'tenant1', 'Dev Key 1', 'sk-test-', 'c1fa602237f88a7c84dc1cff004a4f10f0e85127b2a3461aa33aea6694808262', 'active'),
    ('key-2', 'tenant2', 'Dev Key 2', 'sk-test-', '1e65193bdb95bdb11459530aacbb10e034c09c302fe378abe229774ea3ddc6f1', 'active')
ON CONFLICT (id) DO NOTHING;

-- Bootstrap console admin user
-- Email: admin@openclause.dev
-- Password: admin123 (bcrypt hash)
INSERT INTO users (id, email, password_hash, name, status) VALUES
    ('user-admin', 'admin@openclause.dev', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'Platform Admin', 'active')
ON CONFLICT (id) DO NOTHING;

INSERT INTO user_roles (id, user_id, tenant_id, role) VALUES
    ('role-admin', 'user-admin', NULL, 'platform_admin')
ON CONFLICT (id) DO NOTHING;

-- Sample sessions for demo
INSERT INTO sessions (id, tenant_id, agent_id, started_at) VALUES
    ('session-1', 'tenant1', 'agent-1', NOW() - interval '1 hour'),
    ('session-2', 'tenant1', 'agent-2', NOW() - interval '30 minutes')
ON CONFLICT (id) DO NOTHING;
