package policy

import (
	"context"
	"log/slog"

	"github.com/bturcanu/OpenClause/pkg/types"
)

// ShadowResult captures the enforced and shadow policy decisions side-by-side.
type ShadowResult struct {
	EnforcedDecision string `json:"enforced_decision"`
	ShadowDecision   string `json:"shadow_decision"`
	PolicyVersion    string `json:"policy_version"`
	Match            bool   `json:"match"`
}

// ShadowEvaluator runs two policy clients in parallel: the enforced (live)
// policy and a shadow (candidate) policy. The enforced result is always
// returned; the shadow result is logged for comparison.
type ShadowEvaluator struct {
	enforced *Client
	shadow   *Client
}

// NewShadowEvaluator creates a ShadowEvaluator from two OPA clients.
func NewShadowEvaluator(enforced, shadow *Client) *ShadowEvaluator {
	return &ShadowEvaluator{
		enforced: enforced,
		shadow:   shadow,
	}
}

// EvaluateWithShadow evaluates the input against both the enforced and shadow
// policies. The enforced result is always authoritative; shadow failures are
// logged but never propagated to the caller.
func (s *ShadowEvaluator) EvaluateWithShadow(ctx context.Context, input types.PolicyInput, policyVersion string) (*types.PolicyResult, ShadowResult, error) {
	enforcedResult, err := s.enforced.Evaluate(ctx, input)
	if err != nil {
		return nil, ShadowResult{}, err
	}

	sr := ShadowResult{
		EnforcedDecision: string(enforcedResult.Decision),
		PolicyVersion:    policyVersion,
	}

	shadowResult, shadowErr := s.shadow.Evaluate(ctx, input)
	if shadowErr != nil {
		slog.Warn("shadow policy evaluation failed",
			"error", shadowErr,
			"tool", input.ToolCall.Tool,
			"action", input.ToolCall.Action,
		)
		sr.ShadowDecision = "error"
		sr.Match = false
	} else {
		sr.ShadowDecision = string(shadowResult.Decision)
		sr.Match = enforcedResult.Decision == shadowResult.Decision
	}

	if !sr.Match {
		slog.Info("shadow policy divergence",
			"enforced", sr.EnforcedDecision,
			"shadow", sr.ShadowDecision,
			"tool", input.ToolCall.Tool,
			"action", input.ToolCall.Action,
			"resource", input.ToolCall.Resource,
			"policy_version", policyVersion,
		)
	}

	return enforcedResult, sr, nil
}
