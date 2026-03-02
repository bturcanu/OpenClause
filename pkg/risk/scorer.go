// Package risk provides pluggable risk-scoring strategies for tool-call events.
package risk

import (
	"context"
	"encoding/json"
	"strings"
)

// RiskScorer computes a risk score for a tool-call event.
type RiskScorer interface {
	Score(ctx context.Context, input RiskInput) (RiskOutput, error)
}

type RiskInput struct {
	Tool         string          `json:"tool"`
	Action       string          `json:"action"`
	Resource     string          `json:"resource"`
	ClaimedScore int             `json:"claimed_score"`
	Params       json.RawMessage `json:"params,omitempty"`
}

type RiskOutput struct {
	ComputedScore int      `json:"computed_score"`
	ClaimedScore  int      `json:"claimed_score"`
	Factors       []string `json:"factors"`
	ModelID       string   `json:"model_id,omitempty"`
}

// ── PassthroughScorer ──────────────────────────────────────────────────────

// PassthroughScorer trusts the agent-claimed score without modification.
type PassthroughScorer struct{}

func (PassthroughScorer) Score(_ context.Context, input RiskInput) (RiskOutput, error) {
	return RiskOutput{
		ComputedScore: input.ClaimedScore,
		ClaimedScore:  input.ClaimedScore,
		Factors:       nil,
		ModelID:       "passthrough",
	}, nil
}

// ── RulesBasedScorer ───────────────────────────────────────────────────────

// RulesBasedScorer applies simple heuristic rules on top of the claimed score.
//   - action contains "delete" or "drop" → +3
//   - resource contains "prod"           → +2
type RulesBasedScorer struct{}

func (RulesBasedScorer) Score(_ context.Context, input RiskInput) (RiskOutput, error) {
	score := input.ClaimedScore
	var factors []string

	actionLower := strings.ToLower(input.Action)
	if strings.Contains(actionLower, "delete") || strings.Contains(actionLower, "drop") {
		score += 3
		factors = append(factors, "destructive_action")
	}

	resourceLower := strings.ToLower(input.Resource)
	if strings.Contains(resourceLower, "prod") {
		score += 2
		factors = append(factors, "production_resource")
	}

	return RiskOutput{
		ComputedScore: score,
		ClaimedScore:  input.ClaimedScore,
		Factors:       factors,
		ModelID:       "rules-v1",
	}, nil
}
