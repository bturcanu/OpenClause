package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxConnectorResponseBytes = 4 << 20 // 4 MB

// Registry maps tool names to connector base URLs. Thread-safe.
type Registry struct {
	mu            sync.RWMutex
	routes        map[string]string // tool → base URL
	httpClient    *http.Client
	internalToken string
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
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[tool] = baseURL
}

// Exec routes the request to the correct connector and returns the result.
func (r *Registry) Exec(ctx context.Context, req ExecRequest) (*ExecResponse, error) {
	r.mu.RLock()
	baseURL, ok := r.routes[req.Tool]
	token := r.internalToken
	client := r.httpClient
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no connector registered for tool %q", req.Tool)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("connector marshal: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + "/exec"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("connector new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token != "" {
		httpReq.Header.Set("X-Internal-Token", token)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("connector request to %s: %w", req.Tool, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxConnectorResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("connector read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(respBody)
		if len(snippet) > 512 {
			snippet = snippet[:512]
		}
		return nil, fmt.Errorf("connector %s returned HTTP %d: %s", req.Tool, resp.StatusCode, snippet)
	}

	var execResp ExecResponse
	if err := json.Unmarshal(respBody, &execResp); err != nil {
		return nil, fmt.Errorf("connector decode response: %w", err)
	}

	return &execResp, nil
}

// SetTimeout overrides the default HTTP client timeout for connector calls.
func (r *Registry) SetTimeout(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.httpClient = &http.Client{Timeout: d}
}

// SetInternalToken configures service-to-service auth header for connectors.
func (r *Registry) SetInternalToken(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.internalToken = token
}
