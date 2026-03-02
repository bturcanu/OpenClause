package approvals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// SummaryProvider produces human-readable summaries for approval notifications.
// Implementations range from deterministic templates to LLM-backed summarizers.
type SummaryProvider interface {
	Summarize(ctx context.Context, input SummaryInput) (SummaryOutput, error)
}

type SummaryInput struct {
	Tool        string   `json:"tool"`
	Action      string   `json:"action"`
	Resource    string   `json:"resource"`
	RiskScore   int      `json:"risk_score"`
	RiskFactors []string `json:"risk_factors"`
	Reason      string   `json:"reason"`
	TenantID    string   `json:"tenant_id"`
	AgentID     string   `json:"agent_id"`
}

type SummaryOutput struct {
	SummaryText string `json:"summary_text"`
	ModelID     string `json:"model_id,omitempty"`
	LatencyMS   int64  `json:"latency_ms,omitempty"`
	FromCache   bool   `json:"from_cache,omitempty"`
}

// SummaryInputFromOutbox converts a NotificationOutbox into sanitized SummaryInput.
func SummaryInputFromOutbox(n NotificationOutbox) SummaryInput {
	return SummaryInput{
		Tool:        n.Tool,
		Action:      n.Action,
		Resource:    n.Resource,
		RiskScore:   n.RiskScore,
		RiskFactors: n.RiskFactors,
		Reason:      n.Reason,
		TenantID:    n.TenantID,
		AgentID:     "",
	}
}

// ── TemplateSummaryProvider ─────────────────────────────────────────────────

// TemplateSummaryProvider is a deterministic, zero-dependency summary provider.
type TemplateSummaryProvider struct{}

func (TemplateSummaryProvider) Summarize(_ context.Context, input SummaryInput) (SummaryOutput, error) {
	text := fmt.Sprintf(
		"Approval requested: %s.%s on %s (risk=%d, reason=%s)",
		input.Tool, input.Action, input.Resource, input.RiskScore, input.Reason,
	)
	return SummaryOutput{
		SummaryText: text,
		ModelID:     "template",
	}, nil
}

// ── LLMSummaryProvider ─────────────────────────────────────────────────────

// LLMSummaryProvider calls the llm-summarizer service over HTTP and falls back
// to the template provider on any error.
type LLMSummaryProvider struct {
	llmURL     string
	httpClient *http.Client
	fallback   TemplateSummaryProvider
}

func NewLLMSummaryProvider(llmURL string) *LLMSummaryProvider {
	return &LLMSummaryProvider{
		llmURL: llmURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

type llmSummarizeRequest struct {
	Kind    string       `json:"kind"`
	Payload SummaryInput `json:"payload"`
}

type llmSummarizeResponse struct {
	SummaryText string   `json:"summary_text"`
	ModelID     string   `json:"model_id"`
	LatencyMS   int64    `json:"latency_ms"`
	Warnings    []string `json:"warnings"`
}

func (p *LLMSummaryProvider) Summarize(ctx context.Context, input SummaryInput) (SummaryOutput, error) {
	start := time.Now()

	out, err := p.callLLM(ctx, input)
	if err != nil {
		slog.Warn("llm summarizer failed, falling back to template", "error", err)
		return p.fallback.Summarize(ctx, input)
	}

	if out.LatencyMS == 0 {
		out.LatencyMS = time.Since(start).Milliseconds()
	}
	return out, nil
}

func (p *LLMSummaryProvider) callLLM(ctx context.Context, input SummaryInput) (SummaryOutput, error) {
	reqBody, err := json.Marshal(llmSummarizeRequest{
		Kind:    "approval",
		Payload: input,
	})
	if err != nil {
		return SummaryOutput{}, fmt.Errorf("marshal llm request: %w", err)
	}

	url := p.llmURL + "/v1/summarize"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return SummaryOutput{}, fmt.Errorf("create llm request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return SummaryOutput{}, fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return SummaryOutput{}, fmt.Errorf("read llm response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SummaryOutput{}, fmt.Errorf("llm returned status %d: %s", resp.StatusCode, string(body))
	}

	var llmResp llmSummarizeResponse
	if err := json.Unmarshal(body, &llmResp); err != nil {
		return SummaryOutput{}, fmt.Errorf("decode llm response: %w", err)
	}

	return SummaryOutput{
		SummaryText: llmResp.SummaryText,
		ModelID:     llmResp.ModelID,
		LatencyMS:   llmResp.LatencyMS,
	}, nil
}
