package builtins

import (
	"context"
	"encoding/json"

	"github.com/bturcanu/OpenClause/pkg/connectors"
)

// PostgresConnector provides a mock read-only SQL query action.
type PostgresConnector struct{}

func (c *PostgresConnector) Name() string { return "postgres" }

func (c *PostgresConnector) Actions() []string {
	return []string{"query.readonly"}
}

func (c *PostgresConnector) Exec(_ context.Context, req connectors.ExecRequest) connectors.ExecResponse {
	switch req.Action {
	case "query.readonly":
		return c.queryReadonly(req)
	default:
		return connectors.ExecResponse{Status: "error", Error: "unsupported action: " + req.Action}
	}
}

func (c *PostgresConnector) queryReadonly(req connectors.ExecRequest) connectors.ExecResponse {
	var p struct {
		SQL    string `json:"sql"`
		Params []any  `json:"params"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return connectors.ExecResponse{Status: "error", Error: "invalid params: " + err.Error()}
	}
	out, _ := json.Marshal(map[string]any{
		"columns": []string{"id", "name", "email", "created_at"},
		"rows": [][]any{
			{1, "Alice", "alice@acme.com", "2025-06-01T00:00:00Z"},
			{2, "Bob", "bob@acme.com", "2025-07-15T00:00:00Z"},
			{3, "Charlie", "charlie@acme.com", "2025-09-20T00:00:00Z"},
		},
		"row_count": 3,
		"query":     p.SQL,
		"mock":      true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}
