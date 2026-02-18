package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Registry maps tool names to connector base URLs.
type Registry struct {
	routes     map[string]string // tool → base URL
	httpClient *http.Client
}

// NewRegistry creates a connector registry.
func NewRegistry() *Registry {
	return &Registry{
		routes: make(map[string]string),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Register maps a tool name to a connector URL.
func (r *Registry) Register(tool, baseURL string) {
	r.routes[tool] = baseURL
}

// Exec routes the request to the correct connector and returns the result.
func (r *Registry) Exec(ctx context.Context, req ExecRequest) (*ExecResponse, error) {
	baseURL, ok := r.routes[req.Tool]
	if !ok {
		return nil, fmt.Errorf("no connector registered for tool %q", req.Tool)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("connector marshal: %w", err)
	}

	url := baseURL + "/exec"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("connector new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("connector request to %s: %w", req.Tool, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("connector read response: %w", err)
	}

	var execResp ExecResponse
	if err := json.Unmarshal(respBody, &execResp); err != nil {
		return nil, fmt.Errorf("connector decode response: %w", err)
	}

	return &execResp, nil
}

// SetTimeout overrides the default HTTP client timeout for connector calls.
func (r *Registry) SetTimeout(d time.Duration) {
	r.httpClient.Timeout = d
}
