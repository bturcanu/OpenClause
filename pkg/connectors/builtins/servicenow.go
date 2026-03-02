package builtins

import (
	"context"
	"encoding/json"

	"github.com/bturcanu/OpenClause/pkg/connectors"
)

// ServiceNowConnector provides mock ServiceNow incident management actions.
type ServiceNowConnector struct{}

func (c *ServiceNowConnector) Name() string { return "servicenow" }

func (c *ServiceNowConnector) Actions() []string {
	return []string{"incident.create", "incident.list", "incident.get"}
}

func (c *ServiceNowConnector) Exec(_ context.Context, req connectors.ExecRequest) connectors.ExecResponse {
	switch req.Action {
	case "incident.create":
		return c.incidentCreate(req)
	case "incident.list":
		return c.incidentList()
	case "incident.get":
		return c.incidentGet(req)
	default:
		return connectors.ExecResponse{Status: "error", Error: "unsupported action: " + req.Action}
	}
}

func (c *ServiceNowConnector) incidentCreate(req connectors.ExecRequest) connectors.ExecResponse {
	var p struct {
		ShortDescription string `json:"short_description"`
		Description      string `json:"description"`
		Urgency          string `json:"urgency"`
		Impact           string `json:"impact"`
		AssignmentGroup  string `json:"assignment_group"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return connectors.ExecResponse{Status: "error", Error: "invalid params: " + err.Error()}
	}
	out, _ := json.Marshal(map[string]any{
		"result": map[string]any{
			"sys_id":            "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			"number":            "INC0010042",
			"short_description": p.ShortDescription,
			"state":             "1",
			"urgency":           p.Urgency,
			"impact":            p.Impact,
			"assignment_group":  p.AssignmentGroup,
			"opened_at":         "2026-01-15 10:30:00",
			"sys_created_on":    "2026-01-15 10:30:00",
		},
		"mock": true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}

func (c *ServiceNowConnector) incidentList() connectors.ExecResponse {
	out, _ := json.Marshal(map[string]any{
		"result": []map[string]any{
			{
				"sys_id":            "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
				"number":            "INC0010042",
				"short_description": "API gateway returning 503 errors",
				"state":             "2",
				"priority":          "1",
				"assigned_to":       "alice@acme.com",
				"opened_at":         "2026-01-15 10:30:00",
			},
			{
				"sys_id":            "b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6a1",
				"number":            "INC0010043",
				"short_description": "Database replication lag exceeds threshold",
				"state":             "1",
				"priority":          "2",
				"assigned_to":       "bob@acme.com",
				"opened_at":         "2026-01-15 11:00:00",
			},
		},
		"mock": true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}

func (c *ServiceNowConnector) incidentGet(req connectors.ExecRequest) connectors.ExecResponse {
	var p struct {
		SysID  string `json:"sys_id"`
		Number string `json:"number"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return connectors.ExecResponse{Status: "error", Error: "invalid params: " + err.Error()}
	}
	number := p.Number
	if number == "" {
		number = "INC0010042"
	}
	out, _ := json.Marshal(map[string]any{
		"result": map[string]any{
			"sys_id":            "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			"number":            number,
			"short_description": "API gateway returning 503 errors",
			"description":       "Multiple 503 errors observed on the API gateway since 10:15 UTC.",
			"state":             "2",
			"priority":          "1",
			"urgency":           "1",
			"impact":            "2",
			"assigned_to":       "alice@acme.com",
			"assignment_group":  "Platform Engineering",
			"opened_at":         "2026-01-15 10:30:00",
			"updated_on":        "2026-01-15 11:45:00",
		},
		"mock": true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}
