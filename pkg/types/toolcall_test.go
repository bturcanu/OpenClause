package types

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestValidate_RequiredFields(t *testing.T) {
	tests := []struct {
		name  string
		req   ToolCallRequest
		field string
	}{
		{"missing tenant_id", ToolCallRequest{AgentID: "a", Tool: "t", Action: "a", IdempotencyKey: "k"}, "tenant_id"},
		{"missing agent_id", ToolCallRequest{TenantID: "t", Tool: "t", Action: "a", IdempotencyKey: "k"}, "agent_id"},
		{"missing tool", ToolCallRequest{TenantID: "t", AgentID: "a", Action: "a", IdempotencyKey: "k"}, "tool"},
		{"missing action", ToolCallRequest{TenantID: "t", AgentID: "a", Tool: "t", IdempotencyKey: "k"}, "action"},
		{"missing idempotency_key", ToolCallRequest{TenantID: "t", AgentID: "a", Tool: "t", Action: "a"}, "idempotency_key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if err == nil {
				t.Fatal("expected error")
			}
			ve, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("expected *ValidationError, got %T", err)
			}
			if ve.Field != tt.field {
				t.Errorf("expected field %q, got %q", tt.field, ve.Field)
			}
		})
	}
}

func TestValidate_RiskScore(t *testing.T) {
	req := ToolCallRequest{
		TenantID: "t", AgentID: "a", Tool: "t", Action: "a",
		IdempotencyKey: "k", RiskScore: 11,
	}
	err := req.Validate()
	if err == nil {
		t.Fatal("expected error for risk_score > 10")
	}
}

func TestValidate_ParamsSize(t *testing.T) {
	big := make([]byte, MaxParamsBytes+1)
	for i := range big {
		big[i] = 'x'
	}
	req := ToolCallRequest{
		TenantID: "t", AgentID: "a", Tool: "t", Action: "a",
		IdempotencyKey: "k", Params: json.RawMessage(big),
	}
	err := req.Validate()
	if err == nil {
		t.Fatal("expected error for oversized params")
	}
}

func TestValidate_LabelCount(t *testing.T) {
	labels := make(map[string]string)
	for i := 0; i < MaxLabelsCount+1; i++ {
		labels[strings.Repeat("k", i+1)] = "v"
	}
	req := ToolCallRequest{
		TenantID: "t", AgentID: "a", Tool: "t", Action: "a",
		IdempotencyKey: "k", Labels: labels,
	}
	err := req.Validate()
	if err == nil {
		t.Fatal("expected error for too many labels")
	}
}

func TestNormalize(t *testing.T) {
	req := ToolCallRequest{
		TenantID: "t", AgentID: "a",
		Tool: "  SLACK  ", Action: " MSG.POST ",
		IdempotencyKey: "k",
	}
	_ = req.Validate()
	if req.Tool != "slack" {
		t.Errorf("expected tool 'slack', got %q", req.Tool)
	}
	if req.Action != "msg.post" {
		t.Errorf("expected action 'msg.post', got %q", req.Action)
	}
}

func TestValidate_OK(t *testing.T) {
	req := ToolCallRequest{
		TenantID:       "tenant1",
		AgentID:        "agent-1",
		Tool:           "slack",
		Action:         "msg.post",
		RiskScore:      3,
		IdempotencyKey: "key-123",
		RequestedAt:    time.Now(),
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestToolAction(t *testing.T) {
	req := ToolCallRequest{Tool: "slack", Action: "msg.post"}
	if got := req.ToolAction(); got != "slack.msg.post" {
		t.Errorf("expected 'slack.msg.post', got %q", got)
	}
}
