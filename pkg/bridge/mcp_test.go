package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bturcanu/OpenClause/pkg/types"
)

func TestServeMCPStdioSupportsInitializeListAndCall(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/toolcalls" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		var req types.ToolCallRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if req.Tool != "postgres" || req.Action != "query.readonly" {
			t.Fatalf("unexpected tool request %+v", req)
		}
		_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
			EventID:  "evt-1",
			Decision: types.DecisionAllow,
			Reason:   "ok",
			Result: &types.ExecutionResult{
				Status:     "success",
				OutputJSON: json.RawMessage(`{"row_count":3}`),
			},
		})
	}))
	defer upstream.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  upstream.URL,
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Tools: []ToolConfig{
			{Tool: "postgres", Action: "query.readonly", RiskScore: 2, Description: "List demo users"},
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"openclause_postgres_query_readonly","arguments":{"sql":"select 1","params":[]}}}`,
	}, "\n")
	var output bytes.Buffer
	if err := server.ServeMCPStdio(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatalf("ServeMCPStdio: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 stdio responses, got %d body=%s", len(lines), output.String())
	}

	var initResp mcpResponse
	if err := json.Unmarshal([]byte(lines[0]), &initResp); err != nil {
		t.Fatalf("decode initialize response: %v", err)
	}
	resultMap, ok := initResp.Result.(map[string]any)
	if !ok || resultMap["protocolVersion"] != mcpProtocolVersion {
		t.Fatalf("unexpected initialize result %+v", initResp.Result)
	}

	var listResp struct {
		Result struct {
			Tools []mcpTool `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &listResp); err != nil {
		t.Fatalf("decode tools/list response: %v", err)
	}
	if len(listResp.Result.Tools) != 1 || listResp.Result.Tools[0].Name != "openclause_postgres_query_readonly" {
		t.Fatalf("unexpected MCP tools list %+v", listResp.Result.Tools)
	}
	if got := listResp.Result.Tools[0].InputSchema["required"]; got == nil {
		t.Fatalf("expected input schema to include required fields, got %+v", listResp.Result.Tools[0].InputSchema)
	}

	var callResp struct {
		Result mcpToolCallResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[2]), &callResp); err != nil {
		t.Fatalf("decode tools/call response: %v", err)
	}
	if callResp.Result.IsError {
		t.Fatalf("did not expect MCP tool call to be marked error: %+v", callResp.Result)
	}
	payload, ok := callResp.Result.StructuredContent.(map[string]any)
	if !ok || payload["event_id"] != "evt-1" {
		t.Fatalf("unexpected MCP structured content %+v", callResp.Result.StructuredContent)
	}
}

func TestMCPHTTPEndpointReturnsJSONRPCResponse(t *testing.T) {
	cfg, err := ResolveConfig(Config{
		BaseURL:  "http://localhost:8080",
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Tools: []ToolConfig{
			{Tool: "slack", Action: "msg.post", RiskScore: 4, Description: "Post Slack message"},
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected application/json response, got %q", got)
	}
	if !strings.Contains(rec.Body.String(), `"openclause_slack_msg_post"`) {
		t.Fatalf("expected slack MCP tool in response, got %s", rec.Body.String())
	}
}

func TestMCPHTTPEndpointCanCallConfiguredTool(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/toolcalls" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
			EventID:  "evt-http-1",
			Decision: types.DecisionAllow,
			Result: &types.ExecutionResult{
				Status:     "success",
				OutputJSON: json.RawMessage(`{"row_count":3}`),
			},
		})
	}))
	defer upstream.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  upstream.URL,
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Tools: []ToolConfig{
			{Tool: "postgres", Action: "query.readonly", RiskScore: 2, Description: "List demo users"},
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"openclause_postgres_query_readonly","arguments":{"sql":"select 1","params":[]}}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"event_id":"evt-http-1"`) || !strings.Contains(rec.Body.String(), `"row_count":3`) {
		t.Fatalf("expected MCP tools/call response payload, got %s", rec.Body.String())
	}
}

func TestMCPHTTPEndpointCreatesAndUsesSession(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
			EventID:  "evt-http-session-1",
			Decision: types.DecisionAllow,
		})
	}))
	defer upstream.Close()

	cfg, err := ResolveConfig(Config{
		DefaultProfile: "tenant-b",
		Profiles: map[string]ProfileConfig{
			"tenant-a": {
				BaseURL:  upstream.URL,
				TenantID: "tenant-a",
				AgentID:  "agent-a",
				APIKey:   "sk-a",
				Tools:    []ToolConfig{{Tool: "postgres", Action: "query.readonly", RiskScore: 2}},
			},
			"tenant-b": {
				BaseURL:  upstream.URL,
				TenantID: "tenant-b",
				AgentID:  "agent-b",
				APIKey:   "sk-b",
				Tools:    []ToolConfig{{Tool: "postgres", Action: "query.readonly", RiskScore: 2}},
			},
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	initReq := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	initReq.Header.Set(bridgeProfileHeader, "tenant-a")
	initRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(initRec, initReq)
	if initRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on initialize, got %d body=%s", initRec.Code, initRec.Body.String())
	}
	sessionID := initRec.Header().Get(mcpSessionHeader)
	if sessionID == "" {
		t.Fatalf("expected session header on initialize, got headers=%v", initRec.Header())
	}

	callReq := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"openclause_postgres_query_readonly","arguments":{"sql":"select 1"}}}`))
	callReq.Header.Set(mcpSessionHeader, sessionID)
	callRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(callRec, callReq)
	if callRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on tools/call, got %d body=%s", callRec.Code, callRec.Body.String())
	}
	if !strings.Contains(callRec.Body.String(), `"event_id":"evt-http-session-1"`) {
		t.Fatalf("expected tools/call success payload, got %s", callRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/mcp", http.NoBody)
	deleteReq.Header.Set(mcpSessionHeader, sessionID)
	deleteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on delete, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
}
