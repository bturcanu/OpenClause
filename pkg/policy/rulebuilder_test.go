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

	// risk_score 6 > MaxRiskAutoApprove 5 → should require approval
	tc := types.ToolCallRequest{
		Tool:      "slack",
		Action:    "msg.post",
		RiskScore: 6,
	}

	got := EvaluateWithRuleBuilder(tc, cfg)
	if got == nil {
		t.Fatal("expected non-nil policy result")
	}
	if got.Decision != types.DecisionApprove {
		t.Fatalf("expected decision %q, got %q", types.DecisionApprove, got.Decision)
	}
}

func TestEvaluateWithRuleBuilder_AllowsAtExactThreshold(t *testing.T) {
	cfg := RuleBuilderConfig{
		MaxRiskAutoApprove:         5,
		ReadActions:                []string{"slack.msg.read"},
		WriteActions:               []string{"slack.msg.post"},
		DestructiveActions:         []string{},
		RequireDestructiveApproval: true,
	}

	// risk_score == MaxRiskAutoApprove → should auto-allow (not require approval)
	tc := types.ToolCallRequest{
		Tool:      "slack",
		Action:    "msg.post",
		RiskScore: 5,
	}

	got := EvaluateWithRuleBuilder(tc, cfg)
	if got == nil {
		t.Fatal("expected non-nil policy result")
	}
	if got.Decision != types.DecisionAllow {
		t.Fatalf("expected decision %q, got %q (risk_score at threshold should auto-allow)", types.DecisionAllow, got.Decision)
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
