// Package connectors defines the interface and types for tool connectors.
package connectors

import (
	"context"
	"encoding/json"
)

// Connector executes a tool action on an external system.
type Connector interface {
	// Exec executes the given request and returns a result.
	Exec(ctx context.Context, req ExecRequest) ExecResponse
}

// ExecRequest is the payload sent from the gateway to a connector.
type ExecRequest struct {
	EventID  string          `json:"event_id"`
	TenantID string          `json:"tenant_id"`
	AgentID  string          `json:"agent_id"`
	Tool     string          `json:"tool"`
	Action   string          `json:"action"`
	Params   json.RawMessage `json:"params"`
	Resource string          `json:"resource,omitempty"`
}

// ExecResponse is what the connector returns.
type ExecResponse struct {
	Status     string          `json:"status"` // "success" | "error"
	OutputJSON json.RawMessage `json:"output_json,omitempty"`
	Error      string          `json:"error,omitempty"`
}
