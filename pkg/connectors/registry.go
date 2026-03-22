package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

const maxConnectorResponseBytes = 4 << 20 // 4 MB

// ConnectorInfo describes a registered connector for listing/discovery.
type ConnectorInfo struct {
	Name    string   `json:"name"`
	BaseURL string   `json:"base_url,omitempty"`
	Actions []string `json:"actions"`
	Type    string   `json:"type"` // "remote" or "builtin"
}

type builtinEntry struct {
	actions []string
	handler func(context.Context, ExecRequest) ExecResponse
}

type remoteEntry struct {
	baseURL string
	actions []string
}

// Registry maps tool names to connector base URLs. Thread-safe.
type Registry struct {
	mu            sync.RWMutex
	routes        map[string]remoteEntry // tool → remote connector
	builtins      map[string]builtinEntry
	httpClient    *http.Client
	internalToken string
}

// NewRegistry creates a connector registry.
func NewRegistry() *Registry {
	return &Registry{
		routes:   make(map[string]remoteEntry),
		builtins: make(map[string]builtinEntry),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Register maps a tool name to a remote connector URL with known actions.
func (r *Registry) Register(tool, baseURL string, actions ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[tool] = remoteEntry{baseURL: baseURL, actions: actions}
}

// RegisterBuiltin registers an in-process connector that is invoked
// directly without HTTP. Remote connectors take precedence if both exist
// for the same tool name.
func (r *Registry) RegisterBuiltin(name string, actions []string, handler func(context.Context, ExecRequest) ExecResponse) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.builtins[name] = builtinEntry{actions: actions, handler: handler}
}

// ListAll returns metadata for every registered connector (remote + builtin).
// BaseURL is intentionally omitted from the public response to avoid leaking
// internal service URLs (HIGH-06).
func (r *Registry) ListAll() []ConnectorInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool, len(r.routes)+len(r.builtins))
	out := make([]ConnectorInfo, 0, len(r.routes)+len(r.builtins))

	for tool, entry := range r.routes {
		seen[tool] = true
		out = append(out, ConnectorInfo{
			Name:    tool,
			Actions: normalizeActions(entry.actions),
			Type:    "remote",
		})
	}
	for name, entry := range r.builtins {
		if seen[name] {
			continue
		}
		out = append(out, ConnectorInfo{
			Name:    name,
			Actions: normalizeActions(entry.actions),
			Type:    "builtin",
		})
	}
	slices.SortFunc(out, func(a, b ConnectorInfo) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

// Exec routes the request to the correct connector and returns the result.
// Remote connectors (HTTP) take precedence; built-in connectors are used as fallback.
func (r *Registry) Exec(ctx context.Context, req ExecRequest) (*ExecResponse, error) {
	r.mu.RLock()
	remote, hasRemote := r.routes[req.Tool]
	builtin, hasBuiltin := r.builtins[req.Tool]
	token := r.internalToken
	client := r.httpClient
	baseURL := remote.baseURL
	r.mu.RUnlock()

	if !hasRemote && hasBuiltin {
		resp := builtin.handler(ctx, req)
		return &resp, nil
	}
	if !hasRemote {
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

func normalizeActions(actions []string) []string {
	if len(actions) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(actions))
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if action == "" {
			continue
		}
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		out = append(out, action)
	}
	slices.Sort(out)
	return out
}
