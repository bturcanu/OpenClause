// Package policy provides an HTTP client for Open Policy Agent evaluation.
package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bturcanu/OpenClause/pkg/types"
)

// Client calls OPA over HTTP to evaluate tool-call policies.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new OPA policy client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// opaRequest is the top-level envelope OPA expects.
type opaRequest struct {
	Input types.PolicyInput `json:"input"`
}

// opaResponse is the shape OPA returns.
type opaResponse struct {
	Result opaResult `json:"result"`
}

type opaResult struct {
	Decision     string            `json:"decision"`
	Reason       string            `json:"reason"`
	Requirements map[string]string `json:"requirements,omitempty"`
}

// Evaluate sends a PolicyInput to OPA and returns the decision.
func (c *Client) Evaluate(ctx context.Context, input types.PolicyInput) (*types.PolicyResult, error) {
	body, err := json.Marshal(opaRequest{Input: input})
	if err != nil {
		return nil, fmt.Errorf("policy marshal: %w", err)
	}

	url := c.baseURL + "/v1/data/oc/main"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("policy new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("policy request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("policy OPA returned %d: %s", resp.StatusCode, string(b))
	}

	var opaResp opaResponse
	if err := json.NewDecoder(resp.Body).Decode(&opaResp); err != nil {
		return nil, fmt.Errorf("policy decode response: %w", err)
	}

	decision := types.Decision(opaResp.Result.Decision)
	if decision == "" {
		decision = types.DecisionDeny // default deny if OPA returned nothing
	}

	return &types.PolicyResult{
		Decision:     decision,
		Reason:       opaResp.Result.Reason,
		Requirements: opaResp.Result.Requirements,
	}, nil
}
