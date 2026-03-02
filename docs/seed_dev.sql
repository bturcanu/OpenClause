-- Development seed data for local testing.
-- Run via: psql -U openclause -d openclause -f docs/seed_dev.sql
-- Or:      Get-Content docs\seed_dev.sql | docker compose -f deploy/docker-compose.yml exec -T postgres psql -U openclause -d openclause

-- Tenants
INSERT INTO tenants (id, name, status, config) VALUES
  ('tenant1', 'Acme Corp', 'active', '{"max_risk_auto_approve": 5}'),
  ('tenant2', 'Globex Inc', 'active', '{"max_risk_auto_approve": 3}')
ON CONFLICT (id) DO NOTHING;

-- Agents
INSERT INTO agents (id, tenant_id, name, status) VALUES
  ('agent-1', 'tenant1', 'Test Agent', 'active'),
  ('agent-2', 'tenant1', 'CI Agent', 'active'),
  ('agent-3', 'tenant2', 'Globex Bot', 'active')
ON CONFLICT (id) DO NOTHING;

-- API keys (raw keys: sk-test-key-1, sk-test-key-2)
INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, status) VALUES
  ('key-1', 'tenant1', 'dev-key-1', 'sk-test-',
   'c1fa602237f88a7c84dc1cff004a4f10f0e85127b2a3461aa33aea6694808262', 'active'),
  ('key-2', 'tenant2', 'dev-key-2', 'sk-test-',
   '1e65193bdb95bdb11459530aacbb10e034c09c302fe378abe229774ea3ddc6f1', 'active')
ON CONFLICT (id) DO NOTHING;

-- Admin user (email: admin@openclause.dev, password: admin123)
INSERT INTO users (id, email, password_hash, name, status) VALUES
  ('user-admin', 'admin@openclause.dev',
   '$2a$10$3Kf3g5CCnYM1OaSq3GifI.PRWNyRu6KWEBRXwBRPeX1/ypbzLfxDu',
   'Admin', 'active')
ON CONFLICT (id) DO NOTHING;

INSERT INTO user_roles (id, user_id, tenant_id, role) VALUES
  ('role-admin', 'user-admin', NULL, 'platform_admin')
ON CONFLICT (id) DO NOTHING;

-- Sessions (for timeline testing)
INSERT INTO sessions (id, tenant_id, agent_id) VALUES
  ('session-1', 'tenant1', 'agent-1'),
  ('session-2', 'tenant1', 'agent-2')
ON CONFLICT (id) DO NOTHING;
