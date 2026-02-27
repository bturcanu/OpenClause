package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
			if err == nil {
				return resp, nil
			}
		}
	}
}

func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr types.APIError
		if decodeErr := json.NewDecoder(resp.Body).Decode(&apiErr); decodeErr == nil && apiErr.Message != "" {
			return fmt.Errorf("api error %s: %s", apiErr.Code, apiErr.Message)
		}
		return fmt.Errorf("http status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}
	return nil
}
