package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bturcanu/OpenClause/pkg/types"
	"github.com/google/uuid"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Submit sends a tool-call request. If IdempotencyKey is empty, a unique key
// is generated per call — callers wanting retry-safe idempotency should set it
// explicitly before calling Submit.
func (c *Client) Submit(ctx context.Context, req types.ToolCallRequest) (*types.ToolCallResponse, error) {
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = uuid.NewString()
	}
	if req.TraceID == "" {
		req.TraceID = uuid.NewString()
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/toolcalls", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.apiKey)

	var resp types.ToolCallResponse
	if err := c.doJSON(httpReq, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Execute(ctx context.Context, parentEventID string) (*types.ToolCallResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/toolcalls/"+parentEventID+"/execute", http.NoBody)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("X-API-Key", c.apiKey)
	var resp types.ToolCallResponse
	if err := c.doJSON(httpReq, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) WaitForApprovalThenExecute(ctx context.Context, eventID string, pollEvery time.Duration) (*types.ToolCallResponse, error) {
	t := time.NewTicker(pollEvery)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-t.C:
			resp, err := c.Execute(ctx, eventID)
			if err != nil {
				if isRetryable(err) {
					continue
				}
				return nil, err
			}
			return resp, nil
		}
	}
}

func isRetryable(err error) bool {
	var apiErr *types.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable || apiErr.HTTPCode == http.StatusConflict
	}
	return false
}

const maxResponseBytes = 4 << 20 // 4 MB

func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxResponseBytes)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr types.APIError
		if decodeErr := json.NewDecoder(limited).Decode(&apiErr); decodeErr == nil && apiErr.Message != "" {
			apiErr.HTTPCode = resp.StatusCode
			return &apiErr
		}
		return fmt.Errorf("http status %d", resp.StatusCode)
	}
	return json.NewDecoder(limited).Decode(out)
}
