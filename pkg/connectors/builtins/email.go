package builtins

import (
	"context"
	"encoding/json"

	"github.com/bturcanu/OpenClause/pkg/connectors"
)

// EmailConnector provides mock email send/inbox actions.
type EmailConnector struct{}

func (c *EmailConnector) Name() string { return "email" }

func (c *EmailConnector) Actions() []string {
	return []string{"send", "list_inbox"}
}

func (c *EmailConnector) Exec(_ context.Context, req connectors.ExecRequest) connectors.ExecResponse {
	switch req.Action {
	case "send":
		return c.send(req)
	case "list_inbox":
		return c.listInbox()
	default:
		return connectors.ExecResponse{Status: "error", Error: "unsupported action: " + req.Action}
	}
}

func (c *EmailConnector) send(req connectors.ExecRequest) connectors.ExecResponse {
	var p struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return connectors.ExecResponse{Status: "error", Error: "invalid params: " + err.Error()}
	}
	out, _ := json.Marshal(map[string]any{
		"message_id": "mock-msg-20260115-001@openclause.local",
		"to":         p.To,
		"subject":    p.Subject,
		"status":     "queued",
		"mock":       true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}

func (c *EmailConnector) listInbox() connectors.ExecResponse {
	out, _ := json.Marshal(map[string]any{
		"messages": []map[string]any{
			{
				"message_id": "msg-001@sender.example.com",
				"from":       "alice@acme.com",
				"subject":    "Deploy approval needed",
				"date":       "2026-01-15T09:00:00Z",
				"snippet":    "Please approve the production deploy for v2.3.1.",
				"read":       false,
			},
			{
				"message_id": "msg-002@sender.example.com",
				"from":       "ci-bot@acme.com",
				"subject":    "Build #847 passed",
				"date":       "2026-01-15T08:30:00Z",
				"snippet":    "All 312 tests passed. Coverage: 89%.",
				"read":       true,
			},
		},
		"total_count": 2,
		"mock":        true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}
