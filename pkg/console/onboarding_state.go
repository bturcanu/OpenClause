package console

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type onboardingQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type OnboardingCreateStateInput struct {
	ExistingTenantID string
	NewTenantName    string
	AgentName        string
	APIKeyName       string
	APIKeyExpiresAt  *time.Time
	PolicyConfig     *TenantPolicyConfig
	Integration      AgentIntegrationUpsertInput
}

type OnboardingCreateStateResult struct {
	Tenant        *Tenant
	CreatedTenant bool
	Agent         *Agent
	APIKey        *APIKeyCreateResult
	Integration   *AgentIntegration
}

func (s *Store) CreateOnboardingState(ctx context.Context, in OnboardingCreateStateInput) (*OnboardingCreateStateResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("console.CreateOnboardingState begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var (
		tenant        *Tenant
		createdTenant bool
	)
	if strings.TrimSpace(in.NewTenantName) != "" {
		tenant, err = createTenantWithQuerier(ctx, tx, strings.TrimSpace(in.NewTenantName), nil)
		if err != nil {
			return nil, fmt.Errorf("console.CreateOnboardingState create tenant: %w", err)
		}
		createdTenant = true
	} else {
		tenant, err = getTenantWithQuerier(ctx, tx, strings.TrimSpace(in.ExistingTenantID))
		if err != nil {
			return nil, fmt.Errorf("console.CreateOnboardingState load tenant: %w", err)
		}
		if tenant == nil {
			return nil, fmt.Errorf("console.CreateOnboardingState: %w", ErrTenantNotFound)
		}
	}

	if in.PolicyConfig != nil {
		if err := setTenantPolicyConfigWithQuerier(ctx, tx, tenant.ID, *in.PolicyConfig); err != nil {
			return nil, fmt.Errorf("console.CreateOnboardingState policy config: %w", err)
		}
	}

	agent, err := createAgentWithLabelsQuerier(ctx, tx, tenant.ID, strings.TrimSpace(in.AgentName), nil)
	if err != nil {
		return nil, fmt.Errorf("console.CreateOnboardingState create agent: %w", err)
	}

	keyResult, err := createAPIKeyWithQuerier(ctx, tx, tenant.ID, strings.TrimSpace(in.APIKeyName), in.APIKeyExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("console.CreateOnboardingState create api key: %w", err)
	}

	in.Integration.TenantID = tenant.ID
	in.Integration.AgentID = agent.ID
	integration, err := upsertAgentIntegrationWithQuerier(ctx, tx, in.Integration)
	if err != nil {
		return nil, fmt.Errorf("console.CreateOnboardingState persist integration: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("console.CreateOnboardingState commit: %w", err)
	}
	return &OnboardingCreateStateResult{
		Tenant:        tenant,
		CreatedTenant: createdTenant,
		Agent:         agent,
		APIKey:        keyResult,
		Integration:   integration,
	}, nil
}

func (s *Store) PersistOnboardingIntegration(ctx context.Context, tenantID, agentID string, integration AgentIntegrationUpsertInput) (*AgentIntegration, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("console.PersistOnboardingIntegration begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	integration.TenantID = strings.TrimSpace(tenantID)
	integration.AgentID = strings.TrimSpace(agentID)
	record, err := upsertAgentIntegrationWithQuerier(ctx, tx, integration)
	if err != nil {
		return nil, fmt.Errorf("console.PersistOnboardingIntegration persist integration: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("console.PersistOnboardingIntegration commit: %w", err)
	}
	return record, nil
}

func createTenantWithQuerier(ctx context.Context, q onboardingQuerier, name string, config json.RawMessage) (*Tenant, error) {
	t := &Tenant{
		ID:     uuid.NewString(),
		Name:   name,
		Status: "active",
		Config: config,
	}
	if len(t.Config) == 0 {
		t.Config = json.RawMessage(`{}`)
	}
	if err := q.QueryRow(ctx, `
		INSERT INTO tenants (id, name, status, config)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at`,
		t.ID, t.Name, t.Status, t.Config,
	).Scan(&t.CreatedAt); err != nil {
		return nil, err
	}
	return t, nil
}

func getTenantWithQuerier(ctx context.Context, q onboardingQuerier, id string) (*Tenant, error) {
	var t Tenant
	err := q.QueryRow(ctx, `
		SELECT id, name, status, config, created_at
		FROM tenants WHERE id = $1`, id,
	).Scan(&t.ID, &t.Name, &t.Status, &t.Config, &t.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func setTenantPolicyConfigWithQuerier(ctx context.Context, q onboardingQuerier, tenantID string, cfg TenantPolicyConfig) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	res, err := q.Exec(ctx, `
		UPDATE tenants
		SET config = jsonb_set(
			COALESCE(config, '{}'::jsonb),
			'{policy_config}',
			$1::jsonb,
			true
		)
		WHERE id = $2`, b, tenantID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrTenantNotFound, tenantID)
	}
	return nil
}

func createAgentWithLabelsQuerier(ctx context.Context, q onboardingQuerier, tenantID, name string, labels json.RawMessage) (*Agent, error) {
	if len(strings.TrimSpace(string(labels))) == 0 {
		labels = json.RawMessage(`{}`)
	}
	a := &Agent{
		ID:       uuid.NewString(),
		TenantID: tenantID,
		Name:     name,
		Status:   "active",
		Labels:   labels,
	}
	if err := q.QueryRow(ctx, `
		INSERT INTO agents (id, tenant_id, name, status, labels)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at`,
		a.ID, a.TenantID, a.Name, a.Status, a.Labels,
	).Scan(&a.CreatedAt); err != nil {
		return nil, err
	}
	return a, nil
}

func updateAgentLabelsForTenantWithQuerier(ctx context.Context, q onboardingQuerier, tenantID, agentID string, labels json.RawMessage) error {
	if len(strings.TrimSpace(string(labels))) == 0 {
		labels = json.RawMessage(`{}`)
	}
	res, err := q.Exec(ctx, `
		UPDATE agents
		SET labels = $3
		WHERE tenant_id = $1 AND id = $2`,
		tenantID, agentID, labels,
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}
	return nil
}

func createAPIKeyWithQuerier(ctx context.Context, q onboardingQuerier, tenantID, name string, expiresAt *time.Time) (*APIKeyCreateResult, error) {
	raw, prefix, keyHash, err := generateAPIKey()
	if err != nil {
		return nil, err
	}
	var hasActivePrimary bool
	if err := q.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM api_keys
			WHERE tenant_id = $1 AND status = 'active' AND is_primary = true
		)`, tenantID,
	).Scan(&hasActivePrimary); err != nil {
		return nil, err
	}

	k := APIKey{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Name:      name,
		KeyPrefix: prefix,
		Status:    "active",
		ExpiresAt: expiresAt,
		IsPrimary: !hasActivePrimary,
	}
	if err := q.QueryRow(ctx, `
		INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, status, expires_at, is_primary)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`,
		k.ID, k.TenantID, k.Name, k.KeyPrefix, keyHash, k.Status, k.ExpiresAt, k.IsPrimary,
	).Scan(&k.CreatedAt); err != nil {
		return nil, err
	}

	return &APIKeyCreateResult{APIKey: k, RawKey: raw}, nil
}

func upsertAgentIntegrationWithQuerier(ctx context.Context, q onboardingQuerier, in AgentIntegrationUpsertInput) (*AgentIntegration, error) {
	in = normalizeAgentIntegrationInput(in)
	if in.TenantID == "" || in.AgentID == "" || in.Runtime == "" {
		return nil, fmt.Errorf("tenant_id, agent_id, and runtime are required")
	}

	toolsJSON, err := json.Marshal(in.Tools)
	if err != nil {
		return nil, fmt.Errorf("encode tools: %w", err)
	}

	row := q.QueryRow(ctx, `
		INSERT INTO agent_integrations (
			id, tenant_id, agent_id, runtime, environment_label, owner_name, description, approval_posture, tools
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (tenant_id, agent_id)
		DO UPDATE SET
			runtime = EXCLUDED.runtime,
			environment_label = EXCLUDED.environment_label,
			owner_name = EXCLUDED.owner_name,
			description = EXCLUDED.description,
			approval_posture = EXCLUDED.approval_posture,
			tools = EXCLUDED.tools,
			updated_at = NOW()
		RETURNING id, tenant_id, agent_id, runtime, environment_label, owner_name, description, approval_posture, tools, created_at, updated_at
	`, uuid.NewString(), in.TenantID, in.AgentID, in.Runtime, in.EnvironmentLabel, in.OwnerName, in.Description, in.ApprovalPosture, toolsJSON)

	integration, err := scanAgentIntegration(row)
	if err != nil {
		return nil, err
	}

	if _, err := q.Exec(ctx, `
		INSERT INTO agent_integration_revisions (
			id, integration_id, tenant_id, agent_id, mode, runtime, environment_label, owner_name, description, approval_posture, tools
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, uuid.NewString(), integration.ID, integration.TenantID, integration.AgentID, in.Mode, integration.Runtime, integration.EnvironmentLabel, integration.OwnerName, integration.Description, integration.ApprovalPosture, toolsJSON); err != nil {
		return nil, err
	}

	return integration, nil
}
