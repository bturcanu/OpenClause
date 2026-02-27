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
			err := tt.req.NormalizeAndValidate()
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

func TestValidate_RiskScoreAboveMax(t *testing.T) {
	req := ToolCallRequest{
		TenantID: "t", AgentID: "a", Tool: "t", Action: "a",
		IdempotencyKey: "k", RiskScore: 11,
	}
	err := req.NormalizeAndValidate()
	if err == nil {
		t.Fatal("expected error for risk_score > 10")
	}
}

func TestValidate_RiskScoreNegative(t *testing.T) {
	req := ToolCallRequest{
		TenantID: "t", AgentID: "a", Tool: "t", Action: "a",
		IdempotencyKey: "k", RiskScore: -1,
	}
	err := req.NormalizeAndValidate()
	if err == nil {
		t.Fatal("expected error for risk_score < 0")
	}
	ve := err.(*ValidationError)
	if ve.Field != "risk_score" {
		t.Errorf("expected field risk_score, got %q", ve.Field)
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
	err := req.NormalizeAndValidate()
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
	err := req.NormalizeAndValidate()
	if err == nil {
		t.Fatal("expected error for too many labels")
	}
}

func TestValidate_ResourceByteLength(t *testing.T) {
	// len() measures bytes, not runes. 2049 ASCII bytes should fail.
	req := ToolCallRequest{
		TenantID: "t", AgentID: "a", Tool: "t", Action: "a",
		IdempotencyKey: "k",
		Resource:       strings.Repeat("a", MaxResourceBytes+1),
	}
	err := req.NormalizeAndValidate()
	if err == nil {
		t.Fatal("expected error for oversized resource")
	}
	ve := err.(*ValidationError)
	if ve.Field != "resource" {
		t.Errorf("expected field resource, got %q", ve.Field)
	}
}

func TestValidate_IdempotencyKeyMaxLength(t *testing.T) {
	req := ToolCallRequest{
		TenantID: "t", AgentID: "a", Tool: "t", Action: "a",
		IdempotencyKey: strings.Repeat("k", MaxIdempotencyKeyBytes+1),
	}
	err := req.NormalizeAndValidate()
	if err == nil {
		t.Fatal("expected error for oversized idempotency_key")
	}
	ve := err.(*ValidationError)
	if ve.Field != "idempotency_key" {
		t.Errorf("expected field idempotency_key, got %q", ve.Field)
	}
}

func TestValidate_SchemaVersionUnknown(t *testing.T) {
	req := ToolCallRequest{
		TenantID: "t", AgentID: "a", Tool: "t", Action: "a",
		IdempotencyKey: "k", SchemaVersion: "99.0",
	}
	err := req.NormalizeAndValidate()
	if err == nil {
		t.Fatal("expected error for unknown schema version")
	}
	ve := err.(*ValidationError)
	if ve.Field != "schema_version" {
		t.Errorf("expected field schema_version, got %q", ve.Field)
	}
}

func TestValidate_SchemaVersionDefault(t *testing.T) {
	req := ToolCallRequest{
		TenantID: "t", AgentID: "a", Tool: "t", Action: "a",
		IdempotencyKey: "k",
	}
	if err := req.NormalizeAndValidate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.SchemaVersion != CurrentSchemaVer {
		t.Errorf("expected schema_version %q, got %q", CurrentSchemaVer, req.SchemaVersion)
	}
}

func TestValidate_RequestedAtDefault(t *testing.T) {
	req := ToolCallRequest{
		TenantID: "t", AgentID: "a", Tool: "t", Action: "a",
		IdempotencyKey: "k",
	}
	if err := req.NormalizeAndValidate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.RequestedAt.IsZero() {
		t.Error("expected RequestedAt to be auto-filled")
	}
}

func TestNormalize(t *testing.T) {
	req := ToolCallRequest{
		TenantID: "t", AgentID: "a",
		Tool: "  SLACK  ", Action: " MSG.POST ",
		IdempotencyKey: "k",
	}
	_ = req.NormalizeAndValidate()
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
	if err := req.NormalizeAndValidate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestToolAction(t *testing.T) {
	req := ToolCallRequest{Tool: "slack", Action: "msg.post"}
	if got := req.ToolAction(); got != "slack.msg.post" {
		t.Errorf("expected 'slack.msg.post', got %q", got)
	}
}
