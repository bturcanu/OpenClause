package policy

import (
	"strings"

	"github.com/bturcanu/OpenClause/pkg/types"
)

type RuleBuilderConfig struct {
	MaxRiskAutoApprove         int
	ReadActions                []string
	WriteActions               []string
	DestructiveActions         []string
	RequireDestructiveApproval bool
}

func containsAction(actions []string, action string) bool {
	for _, v := range actions {
		if strings.EqualFold(strings.TrimSpace(v), action) {
			return true
		}
	}
	return false
}

func EvaluateWithRuleBuilder(tc types.ToolCallRequest, cfg RuleBuilderConfig) *types.PolicyResult {
	toolAction := strings.ToLower(strings.TrimSpace(tc.Tool + "." + tc.Action))
	isRead := containsAction(cfg.ReadActions, toolAction)
	isWrite := containsAction(cfg.WriteActions, toolAction)
	isDestructive := containsAction(cfg.DestructiveActions, toolAction)

	if tc.RiskScore >= cfg.MaxRiskAutoApprove {
		return &types.PolicyResult{
			Decision:     types.DecisionApprove,
			Reason:       "high risk score requires approval",
			Requirements: map[string]string{"approval_scope": "single_use"},
		}
	}
	if isDestructive && cfg.RequireDestructiveApproval {
		return &types.PolicyResult{
			Decision:     types.DecisionApprove,
			Reason:       "destructive action requires approval",
			Requirements: map[string]string{"approval_scope": "single_use"},
		}
	}
	if (isRead || isWrite || isDestructive) && tc.RiskScore < cfg.MaxRiskAutoApprove {
		reason := "action on allowlist within tenant threshold"
		if isRead {
			reason = "read action on allowlist within tenant threshold"
		} else if isWrite || isDestructive {
			reason = "write action on allowlist within tenant threshold"
		}
		return &types.PolicyResult{
			Decision: types.DecisionAllow,
			Reason:   reason,
		}
	}

	return &types.PolicyResult{
		Decision: types.DecisionDeny,
		Reason:   "action not in allowlist",
	}
}

