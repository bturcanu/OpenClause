package policy

import (
	"testing"

	"github.com/bturcanu/OpenClause/pkg/types"
)

func TestEvaluateWithRuleBuilder_UsesConfigurableRiskThresholdForApproval(t *testing.T) {
	cfg := RuleBuilderConfig{
		MaxRiskAutoApprove:         5,
		ReadActions:                []string{"slack.msg.read"},
		WriteActions:               []string{"slack.msg.post"},
		DestructiveActions:         []string{},
		RequireDestructiveApproval: true,
	}

	tc := types.ToolCallRequest{
		Tool:      "slack",
		Action:    "msg.post",
		RiskScore: 5,
	}

	got := EvaluateWithRuleBuilder(tc, cfg)
	if got == nil {
		t.Fatal("expected non-nil policy result")
	}
	if got.Decision != types.DecisionApprove {
		t.Fatalf("expected decision %q, got %q", types.DecisionApprove, got.Decision)
	}
}

func TestEvaluateWithRuleBuilder_AllowsWhenBelowConfiguredRiskThreshold(t *testing.T) {
	cfg := RuleBuilderConfig{
		MaxRiskAutoApprove:         9,
		ReadActions:                []string{"slack.msg.read"},
		WriteActions:               []string{"slack.msg.post"},
		DestructiveActions:         []string{},
		RequireDestructiveApproval: true,
	}

	tc := types.ToolCallRequest{
		Tool:      "slack",
		Action:    "msg.post",
		RiskScore: 8,
	}

	got := EvaluateWithRuleBuilder(tc, cfg)
	if got == nil {
		t.Fatal("expected non-nil policy result")
	}
	if got.Decision != types.DecisionAllow {
		t.Fatalf("expected decision %q, got %q", types.DecisionAllow, got.Decision)
	}
}
