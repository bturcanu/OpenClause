package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type AgentIntegration struct {
	ID               string                 `json:"id"`
	TenantID         string                 `json:"tenant_id"`
	AgentID          string                 `json:"agent_id"`
	Runtime          string                 `json:"runtime"`
	EnvironmentLabel string                 `json:"environment_label,omitempty"`
	OwnerName        string                 `json:"owner_name,omitempty"`
	Description      string                 `json:"description,omitempty"`
	ApprovalPosture  string                 `json:"approval_posture,omitempty"`
	Tools            []AgentIntegrationTool `json:"tools,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type AgentIntegrationRevision struct {
	ID               string                 `json:"id"`
	IntegrationID    string                 `json:"integration_id"`
	TenantID         string                 `json:"tenant_id"`
	AgentID          string                 `json:"agent_id"`
	Mode             string                 `json:"mode"`
	Runtime          string                 `json:"runtime"`
	EnvironmentLabel string                 `json:"environment_label,omitempty"`
	OwnerName        string                 `json:"owner_name,omitempty"`
	Description      string                 `json:"description,omitempty"`
	ApprovalPosture  string                 `json:"approval_posture,omitempty"`
	Tools            []AgentIntegrationTool `json:"tools,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
}

type AgentIntegrationTool struct {
	Tool   string `json:"tool"`
	Action string `json:"action"`
}

type AgentIntegrationUpsertInput struct {
	TenantID         string
	AgentID          string
	Mode             string
	Runtime          string
	EnvironmentLabel string
	OwnerName        string
	Description      string
	ApprovalPosture  string
	Tools            []AgentIntegrationTool
}

func normalizeAgentIntegrationInput(in AgentIntegrationUpsertInput) AgentIntegrationUpsertInput {
	in.TenantID = strings.TrimSpace(in.TenantID)
	in.AgentID = strings.TrimSpace(in.AgentID)
	in.Mode = strings.TrimSpace(strings.ToLower(in.Mode))
	in.Runtime = strings.TrimSpace(in.Runtime)
	in.EnvironmentLabel = strings.TrimSpace(in.EnvironmentLabel)
	in.OwnerName = strings.TrimSpace(in.OwnerName)
	in.Description = strings.TrimSpace(in.Description)
	in.ApprovalPosture = strings.TrimSpace(in.ApprovalPosture)
	tools := make([]AgentIntegrationTool, 0, len(in.Tools))
	for _, tool := range in.Tools {
		name := strings.TrimSpace(tool.Tool)
		action := strings.TrimSpace(tool.Action)
		if name == "" || action == "" {
			continue
		}
		tools = append(tools, AgentIntegrationTool{Tool: name, Action: action})
	}
	in.Tools = tools
	return in
}

func (s *Store) UpsertAgentIntegration(ctx context.Context, in AgentIntegrationUpsertInput) (*AgentIntegration, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("console.UpsertAgentIntegration begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	integration, err := upsertAgentIntegrationWithQuerier(ctx, tx, in)
	if err != nil {
		return nil, fmt.Errorf("console.UpsertAgentIntegration: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("console.UpsertAgentIntegration commit: %w", err)
	}
	return integration, nil
}

func (s *Store) GetAgentIntegration(ctx context.Context, tenantID, agentID string) (*AgentIntegration, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, agent_id, runtime, environment_label, owner_name, description, approval_posture, tools, created_at, updated_at
		FROM agent_integrations
		WHERE tenant_id = $1 AND agent_id = $2
	`, strings.TrimSpace(tenantID), strings.TrimSpace(agentID))
	integration, err := scanAgentIntegration(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("console.GetAgentIntegration: %w", ErrAgentIntegrationNotFound)
		}
		return nil, fmt.Errorf("console.GetAgentIntegration: %w", err)
	}
	return integration, nil
}

func (s *Store) ListAgentIntegrationRevisions(ctx context.Context, tenantID, agentID string, limit int) ([]AgentIntegrationRevision, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, integration_id, tenant_id, agent_id, mode, runtime, environment_label, owner_name, description, approval_posture, tools, created_at
		FROM agent_integration_revisions
		WHERE tenant_id = $1 AND agent_id = $2
		ORDER BY created_at DESC, id DESC
		LIMIT $3
	`, strings.TrimSpace(tenantID), strings.TrimSpace(agentID), limit)
	if err != nil {
		return nil, fmt.Errorf("console.ListAgentIntegrationRevisions: %w", err)
	}
	defer rows.Close()

	revisions := make([]AgentIntegrationRevision, 0, limit)
	for rows.Next() {
		revision, err := scanAgentIntegrationRevision(rows)
		if err != nil {
			return nil, fmt.Errorf("console.ListAgentIntegrationRevisions: %w", err)
		}
		revisions = append(revisions, *revision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListAgentIntegrationRevisions: %w", err)
	}
	return revisions, nil
}

type agentIntegrationScanner interface {
	Scan(dest ...any) error
}

func scanAgentIntegration(row agentIntegrationScanner) (*AgentIntegration, error) {
	var integration AgentIntegration
	var toolsJSON []byte
	if err := row.Scan(
		&integration.ID,
		&integration.TenantID,
		&integration.AgentID,
		&integration.Runtime,
		&integration.EnvironmentLabel,
		&integration.OwnerName,
		&integration.Description,
		&integration.ApprovalPosture,
		&toolsJSON,
		&integration.CreatedAt,
		&integration.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(toolsJSON) > 0 {
		if err := json.Unmarshal(toolsJSON, &integration.Tools); err != nil {
			return nil, fmt.Errorf("decode agent integration tools: %w", err)
		}
	}
	return &integration, nil
}

func scanAgentIntegrationRevision(row agentIntegrationScanner) (*AgentIntegrationRevision, error) {
	var revision AgentIntegrationRevision
	var toolsJSON []byte
	if err := row.Scan(
		&revision.ID,
		&revision.IntegrationID,
		&revision.TenantID,
		&revision.AgentID,
		&revision.Mode,
		&revision.Runtime,
		&revision.EnvironmentLabel,
		&revision.OwnerName,
		&revision.Description,
		&revision.ApprovalPosture,
		&toolsJSON,
		&revision.CreatedAt,
	); err != nil {
		return nil, err
	}
	if len(toolsJSON) > 0 {
		if err := json.Unmarshal(toolsJSON, &revision.Tools); err != nil {
			return nil, fmt.Errorf("decode agent integration revision tools: %w", err)
		}
	}
	return &revision, nil
}
