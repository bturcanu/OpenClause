package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/types"
)

func TestBridgeSubmitInjectsDefaultsAndConfiguredRisk(t *testing.T) {
	var gotReq types.ToolCallRequest
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/toolcalls" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("X-API-Key"); got != "sk-oc-bridge" {
			t.Fatalf("expected upstream api key, got %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
			EventID:  "evt-1",
			Decision: types.DecisionAllow,
		})
	}))
	defer upstream.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  upstream.URL,
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Defaults: DefaultsConfig{
			UserID:        "pilot-user",
			SessionPrefix: "support-bot",
			RiskMode:      "configured",
		},
		Tools: []ToolConfig{
			{Tool: "postgres", Action: "query.readonly", RiskScore: 2, Description: "demo"},
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/toolcalls", strings.NewReader(`{"tool":"postgres","action":"query.readonly","params":{"sql":"select 1"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if gotReq.TenantID != "tenant-1" || gotReq.AgentID != "agent-1" {
		t.Fatalf("expected injected tenant/agent, got %+v", gotReq)
	}
	if gotReq.UserID != "pilot-user" {
		t.Fatalf("expected injected user_id, got %+v", gotReq)
	}
	if gotReq.RiskScore != 2 {
		t.Fatalf("expected configured risk score, got %+v", gotReq)
	}
	if gotReq.SessionID == "" || !strings.HasPrefix(gotReq.SessionID, "support-bot-") {
		t.Fatalf("expected generated session id, got %+v", gotReq)
	}
	if gotReq.TraceID == "" || gotReq.IdempotencyKey == "" {
		t.Fatalf("expected generated trace/idempotency, got %+v", gotReq)
	}
}

func TestBridgeSubmitRejectsUnknownConfiguredTool(t *testing.T) {
	cfg, err := ResolveConfig(Config{
		BaseURL:  "http://localhost:8080",
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Tools: []ToolConfig{
			{Tool: "slack", Action: "channel.list", RiskScore: 1},
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/toolcalls", strings.NewReader(`{"tool":"postgres","action":"query.readonly","params":{"sql":"select 1"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not configured for bridge profile") {
		t.Fatalf("expected configured-tool error, got %s", rec.Body.String())
	}
}

func TestBridgeSubmitRejectsTenantMismatch(t *testing.T) {
	cfg, err := ResolveConfig(Config{
		BaseURL:  "http://localhost:8080",
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/toolcalls", strings.NewReader(`{"tenant_id":"tenant-2","tool":"postgres","action":"query.readonly","risk_score":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBridgeExecuteAndGetEventProxyUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/toolcalls/evt-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"event_id":  "evt-1",
				"tenant_id": "tenant-1",
				"agent_id":  "agent-1",
				"decision":  "allow",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/toolcalls/evt-1/execute":
			_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
				EventID:  "evt-1",
				Decision: types.DecisionAllow,
			})
		default:
			t.Fatalf("unexpected upstream route %s %s", r.Method, r.URL.Path)
		}
	}))
	defer upstream.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  upstream.URL,
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/toolcalls/evt-1", http.NoBody)
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK || !strings.Contains(getRec.Body.String(), `"event_id":"evt-1"`) {
		t.Fatalf("expected get event proxy success, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	executeReq := httptest.NewRequest(http.MethodPost, "/v1/toolcalls/evt-1/execute", bytes.NewReader(nil))
	executeRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(executeRec, executeReq)
	if executeRec.Code != http.StatusOK || !strings.Contains(executeRec.Body.String(), `"event_id":"evt-1"`) {
		t.Fatalf("expected execute proxy success, got %d body=%s", executeRec.Code, executeRec.Body.String())
	}
}

func TestBridgeToolsEndpointReturnsConfiguredTools(t *testing.T) {
	cfg, err := ResolveConfig(Config{
		BaseURL:  "http://localhost:8080",
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Tools: []ToolConfig{
			{Tool: "postgres", Action: "query.readonly", RiskScore: 2},
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/bridge/tools", http.NoBody)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"query.readonly"`) {
		t.Fatalf("expected configured tools response, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBridgeRoutesRequestsToSelectedProfile(t *testing.T) {
	var seenAPIKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAPIKey = r.Header.Get("X-API-Key")
		_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
			EventID:  "evt-profile-1",
			Decision: types.DecisionAllow,
		})
	}))
	defer upstream.Close()

	cfg, err := ResolveConfig(Config{
		DefaultProfile: "tenant-a",
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
				Tools:    []ToolConfig{{Tool: "postgres", Action: "query.readonly", RiskScore: 4}},
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

	req := httptest.NewRequest(http.MethodPost, "/v1/toolcalls", strings.NewReader(`{"tool":"postgres","action":"query.readonly","params":{"sql":"select 1"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(bridgeProfileHeader, "tenant-b")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if seenAPIKey != "sk-b" {
		t.Fatalf("expected selected profile api key, got %q", seenAPIKey)
	}
}

func TestBridgeProfilesEndpointListsConfiguredProfiles(t *testing.T) {
	cfg, err := ResolveConfig(Config{
		DefaultProfile: "tenant-a",
		Profiles: map[string]ProfileConfig{
			"tenant-a": {
				BaseURL:  "http://localhost:8080",
				TenantID: "tenant-a",
				AgentID:  "agent-a",
				APIKey:   "sk-a",
				Tools:    []ToolConfig{{Tool: "postgres", Action: "query.readonly", RiskScore: 2}},
			},
			"tenant-b": {
				BaseURL:  "http://localhost:8080",
				TenantID: "tenant-b",
				AgentID:  "agent-b",
				APIKey:   "sk-b",
				Tools:    []ToolConfig{{Tool: "jira", Action: "jira.issue.create", RiskScore: 5}},
				OpenAI: OpenAIConfig{
					UpstreamBaseURL: "http://localhost:1234/v1",
					Model:           "qwen/qwen3.5-9b",
				},
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

	req := httptest.NewRequest(http.MethodGet, "/v1/bridge/profiles", http.NoBody)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"default_profile":"tenant-a"`) || !strings.Contains(body, `"name":"tenant-b"`) || !strings.Contains(body, `"openai_enabled":true`) {
		t.Fatalf("expected profile summary payload, got %s", body)
	}
	if strings.Contains(body, "sk-a") || strings.Contains(body, "sk-b") {
		t.Fatalf("expected profile endpoint not to leak api keys, got %s", body)
	}
}

func TestBridgeModelsProxyRequiresOpenAIConfig(t *testing.T) {
	cfg, err := ResolveConfig(Config{
		BaseURL:  "http://localhost:8080",
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", http.NoBody)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 without openai config, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBridgeModelsProxyPassesThroughUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(openAIModelsResponse{
			Object: "list",
			Data:   []openAIModel{{ID: "qwen/qwen3.5-9b", Object: "model"}},
		})
	}))
	defer upstream.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  "http://localhost:8080",
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		OpenAI: OpenAIConfig{
			UpstreamBaseURL: upstream.URL + "/v1",
			Model:           "qwen/qwen3.5-9b",
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", http.NoBody)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"qwen/qwen3.5-9b"`) {
		t.Fatalf("expected proxied models response, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBridgeChatCompletionsRunsGovernedToolLoop(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		var req openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode upstream chat request: %v", err)
		}
		upstreamCalls++
		switch upstreamCalls {
		case 1:
			if len(req.Tools) != 1 {
				t.Fatalf("expected bridge-managed tool injection, got %+v", req.Tools)
			}
			_ = json.NewEncoder(w).Encode(openAIChatResponse{
				ID:     "chatcmpl-1",
				Object: "chat.completion",
				Model:  "qwen/qwen3.5-9b",
				Choices: []openAIChoice{{
					Index: 0,
					Message: openAIChatMessage{
						Role: "assistant",
						ToolCalls: []openAIToolCall{{
							ID:   "call-1",
							Type: "function",
							Function: openAIFunctionCall{
								Name:      "governed_action",
								Arguments: `{"operation":"postgres.query.readonly","params":{"sql":"select 1","params":[]}}`,
							},
						}},
					},
					FinishReason: "tool_calls",
				}},
			})
		case 2:
			if len(req.Messages) < 3 {
				t.Fatalf("expected follow-up tool messages, got %+v", req.Messages)
			}
			last := req.Messages[len(req.Messages)-1]
			if last.Role != "tool" || !strings.Contains(asString(t, last.Content), `"decision":"allow"`) {
				t.Fatalf("expected tool result message, got %+v", last)
			}
			_ = json.NewEncoder(w).Encode(openAIChatResponse{
				ID:     "chatcmpl-2",
				Object: "chat.completion",
				Model:  "qwen/qwen3.5-9b",
				Choices: []openAIChoice{{
					Index: 0,
					Message: openAIChatMessage{
						Role:    "assistant",
						Content: "The newest demo users are Alice, Bob, and Charlie.",
					},
					FinishReason: "stop",
				}},
			})
		default:
			t.Fatalf("unexpected upstream call count %d", upstreamCalls)
		}
	}))
	defer upstream.Close()

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/toolcalls" {
			t.Fatalf("unexpected gateway path %s", r.URL.Path)
		}
		var req types.ToolCallRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode gateway request: %v", err)
		}
		if req.TenantID != "tenant-1" || req.AgentID != "agent-1" || req.SessionID == "" || req.TraceID == "" {
			t.Fatalf("expected bridge to inject identities and metadata, got %+v", req)
		}
		_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
			EventID:  "evt-1",
			Decision: types.DecisionAllow,
			Reason:   "allowed",
			Result: &types.ExecutionResult{
				Status:     "success",
				OutputJSON: json.RawMessage(`{"row_count":1}`),
			},
		})
	}))
	defer gateway.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  gateway.URL,
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Defaults: DefaultsConfig{
			UserID:        "pilot-user",
			SessionPrefix: "support-bot",
			RiskMode:      "configured",
		},
		Tools: []ToolConfig{
			{Tool: "postgres", Action: "query.readonly", RiskScore: 2},
		},
		OpenAI: OpenAIConfig{
			UpstreamBaseURL: upstream.URL + "/v1",
			Model:           "qwen/qwen3.5-9b",
			ToolName:        "governed_action",
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"qwen/qwen3.5-9b","messages":[{"role":"user","content":"fetch users"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "The newest demo users are Alice, Bob, and Charlie.") {
		t.Fatalf("expected final assistant response, got %s", rec.Body.String())
	}
}

func TestBridgeChatCompletionsWaitsForApprovalAndResumesExecution(t *testing.T) {
	var upstreamCalls int
	var finalUpstreamReq openAIChatRequest
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode upstream chat request: %v", err)
		}
		upstreamCalls++
		switch upstreamCalls {
		case 1:
			_ = json.NewEncoder(w).Encode(openAIChatResponse{
				ID:     "chatcmpl-approval-1",
				Object: "chat.completion",
				Model:  "qwen/qwen3.5-9b",
				Choices: []openAIChoice{{
					Index: 0,
					Message: openAIChatMessage{
						Role: "assistant",
						ToolCalls: []openAIToolCall{{
							ID:   "approval-call-1",
							Type: "function",
							Function: openAIFunctionCall{
								Name:      "governed_action",
								Arguments: `{"operation":"postgres.query.readonly","params":{"sql":"select 1","params":[]}}`,
							},
						}},
					},
					FinishReason: "tool_calls",
				}},
			})
		case 2:
			finalUpstreamReq = req
			_ = json.NewEncoder(w).Encode(openAIChatResponse{
				ID:     "chatcmpl-approval-2",
				Object: "chat.completion",
				Model:  "qwen/qwen3.5-9b",
				Choices: []openAIChoice{{
					Index: 0,
					Message: openAIChatMessage{
						Role:    "assistant",
						Content: "The approval completed and the governed query returned one row.",
					},
					FinishReason: "stop",
				}},
			})
		default:
			t.Fatalf("unexpected upstream call count %d", upstreamCalls)
		}
	}))
	defer upstream.Close()

	var executeCalls int
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/toolcalls":
			_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
				EventID:     "evt-approval",
				Decision:    types.DecisionApprove,
				Reason:      "manual review required",
				ApprovalURL: "https://approvals.example.com/v1/approvals/requests/req-1",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/toolcalls/evt-approval/execute":
			executeCalls++
			w.Header().Set("Content-Type", "application/json")
			if executeCalls == 1 {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(types.APIError{
					Code:      "CONFLICT",
					Message:   "awaiting approval",
					Retryable: false,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
				EventID:  "exec-approval-1",
				Decision: types.DecisionAllow,
				Reason:   "approved execution",
				Result: &types.ExecutionResult{
					Status:     "success",
					OutputJSON: json.RawMessage(`{"row_count":1}`),
				},
			})
		default:
			t.Fatalf("unexpected gateway request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer gateway.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  gateway.URL,
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Defaults: DefaultsConfig{SessionPrefix: "support-bot"},
		Tools:    []ToolConfig{{Tool: "postgres", Action: "query.readonly", RiskScore: 2}},
		OpenAI: OpenAIConfig{
			UpstreamBaseURL: upstream.URL + "/v1",
			Model:           "qwen/qwen3.5-9b",
			ToolName:        "governed_action",
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"qwen/qwen3.5-9b","messages":[{"role":"user","content":"fetch users"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if executeCalls != 2 {
		t.Fatalf("expected bridge to retry execute until approval was available, got %d execute calls", executeCalls)
	}
	if !strings.Contains(rec.Body.String(), "approval completed") {
		t.Fatalf("expected final assistant response, got %s", rec.Body.String())
	}
	if len(finalUpstreamReq.Messages) == 0 {
		t.Fatalf("expected follow-up upstream request to include governed tool result")
	}
	last := finalUpstreamReq.Messages[len(finalUpstreamReq.Messages)-1]
	if last.Role != "tool" {
		t.Fatalf("expected last upstream message to be tool result, got %+v", last)
	}
	content := asString(t, last.Content)
	if !strings.Contains(content, `"event_id":"exec-approval-1"`) || !strings.Contains(content, `"approval_event_id":"evt-approval"`) || !strings.Contains(content, `"approval_reason":"manual review required"`) {
		t.Fatalf("expected resumed execution payload with approval metadata, got %s", content)
	}
}

func TestBridgeChatCompletionsReturnsTimeoutWhenApprovalNeverArrives(t *testing.T) {
	originalPoll := bridgeApprovalPollInterval
	originalTimeout := bridgeApprovalWaitTimeout
	bridgeApprovalPollInterval = 5 * time.Millisecond
	bridgeApprovalWaitTimeout = 25 * time.Millisecond
	t.Cleanup(func() {
		bridgeApprovalPollInterval = originalPoll
		bridgeApprovalWaitTimeout = originalTimeout
	})

	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		_ = json.NewEncoder(w).Encode(openAIChatResponse{
			ID:     "chatcmpl-timeout-1",
			Object: "chat.completion",
			Model:  "qwen/qwen3.5-9b",
			Choices: []openAIChoice{{
				Index: 0,
				Message: openAIChatMessage{
					Role: "assistant",
					ToolCalls: []openAIToolCall{{
						ID:   "approval-call-timeout",
						Type: "function",
						Function: openAIFunctionCall{
							Name:      "governed_action",
							Arguments: `{"operation":"postgres.query.readonly","params":{"sql":"select 1","params":[]}}`,
						},
					}},
				},
				FinishReason: "tool_calls",
			}},
		})
	}))
	defer upstream.Close()

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/toolcalls":
			_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
				EventID:     "evt-timeout",
				Decision:    types.DecisionApprove,
				Reason:      "manual review required",
				ApprovalURL: "https://approvals.example.com/v1/approvals/requests/req-timeout",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/toolcalls/evt-timeout/execute":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(types.APIError{
				Code:      "CONFLICT",
				Message:   "awaiting approval",
				Retryable: false,
			})
		default:
			t.Fatalf("unexpected gateway request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer gateway.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  gateway.URL,
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Tools:    []ToolConfig{{Tool: "postgres", Action: "query.readonly", RiskScore: 2}},
		OpenAI: OpenAIConfig{
			UpstreamBaseURL: upstream.URL + "/v1",
			Model:           "qwen/qwen3.5-9b",
			ToolName:        "governed_action",
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"qwen/qwen3.5-9b","messages":[{"role":"user","content":"fetch users"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected bridge to stop before a follow-up model turn, got %d upstream calls", upstreamCalls)
	}
	if !strings.Contains(rec.Body.String(), `"code":"APPROVAL_TIMEOUT"`) || !strings.Contains(rec.Body.String(), `approval for postgres.query.readonly did not complete`) {
		t.Fatalf("expected approval timeout payload, got %s", rec.Body.String())
	}
}

func TestBridgeChatCompletionsAllowsClientToolsAndPassesThemThrough(t *testing.T) {
	var upstreamReq openAIChatRequest
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamReq); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(openAIChatResponse{
			ID:     "chatcmpl-client-tool",
			Object: "chat.completion",
			Model:  "qwen/qwen3.5-9b",
			Choices: []openAIChoice{{
				Index: 0,
				Message: openAIChatMessage{
					Role: "assistant",
					ToolCalls: []openAIToolCall{{
						ID:   "client-call-1",
						Type: "function",
						Function: openAIFunctionCall{
							Name:      "lookup_docs",
							Arguments: `{"query":"bridge doctor"}`,
						},
					}},
				},
				FinishReason: "tool_calls",
			}},
		})
	}))
	defer upstream.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  "http://localhost:8080",
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		OpenAI: OpenAIConfig{
			UpstreamBaseURL: upstream.URL + "/v1",
			Model:           "qwen/qwen3.5-9b",
			ToolName:        "governed_action",
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup_docs","description":"look up docs","parameters":{"type":"object"}}}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(upstreamReq.Tools) != 2 {
		t.Fatalf("expected client tool plus governed tool, got %+v", upstreamReq.Tools)
	}
	if upstreamReq.Tools[0].Function.Name != "lookup_docs" || upstreamReq.Tools[1].Function.Name != "governed_action" {
		t.Fatalf("expected tool ordering to preserve client tool before governed tool, got %+v", upstreamReq.Tools)
	}
	if !strings.Contains(rec.Body.String(), `"lookup_docs"`) {
		t.Fatalf("expected raw client tool call to pass through, got %s", rec.Body.String())
	}
}

func TestBridgeChatCompletionsStreamsFinalAssistantResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(openAIChatResponse{
			ID:      "chatcmpl-1",
			Object:  "chat.completion",
			Created: 1774462653,
			Model:   "qwen/qwen3.5-9b",
			Choices: []openAIChoice{{
				Index: 0,
				Message: openAIChatMessage{
					Role:    "assistant",
					Content: "The newest demo users are Alice, Bob, and Charlie.",
				},
				FinishReason: "stop",
			}},
		})
	}))
	defer upstream.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  "http://localhost:8080",
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Tools: []ToolConfig{
			{Tool: "postgres", Action: "query.readonly", RiskScore: 2},
		},
		OpenAI: OpenAIConfig{
			UpstreamBaseURL: upstream.URL + "/v1",
			Model:           "qwen/qwen3.5-9b",
			ToolName:        "governed_action",
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"qwen/qwen3.5-9b","stream":true,"messages":[{"role":"user","content":"fetch users"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("expected SSE content type, got %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"object":"chat.completion.chunk"`) || !strings.Contains(body, "Alice, Bob, and Charlie") || !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected streamed chunks in body, got %s", body)
	}
}

func TestBridgeChatCompletionsStreamsUpstreamChunksTokenByToken(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if !req.Stream {
			t.Fatalf("expected upstream stream request, got %+v", req)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Join([]string{
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1774462653,"model":"qwen/qwen3.5-9b","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
			``,
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1774462653,"model":"qwen/qwen3.5-9b","choices":[{"index":0,"delta":{"content":"Hello "}}]}`,
			``,
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1774462653,"model":"qwen/qwen3.5-9b","choices":[{"index":0,"delta":{"content":"world"}}]}`,
			``,
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1774462653,"model":"qwen/qwen3.5-9b","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n"))
	}))
	defer upstream.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  "http://localhost:8080",
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Tools:    []ToolConfig{{Tool: "postgres", Action: "query.readonly", RiskScore: 2}},
		OpenAI: OpenAIConfig{
			UpstreamBaseURL: upstream.URL + "/v1",
			Model:           "qwen/qwen3.5-9b",
			ToolName:        "governed_action",
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"qwen/qwen3.5-9b","stream":true,"messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"content":"Hello "`) || !strings.Contains(body, `"content":"world"`) || !strings.Contains(body, `data: [DONE]`) {
		t.Fatalf("expected streamed upstream content, got %s", body)
	}
}

func TestBridgeChatCompletionsStreamsToolLoopFromUpstreamChunks(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "text/event-stream")
		switch upstreamCalls {
		case 1:
			_, _ = io.WriteString(w, strings.Join([]string{
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1774462653,"model":"qwen/qwen3.5-9b","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
				``,
				`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1774462653,"model":"qwen/qwen3.5-9b","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"governed_action","arguments":"{\"operation\":\"postgres.query.readonly\",\"params\":{\"sql\":\"select 1\",\"params\":[]}}"}}]},"finish_reason":"tool_calls"}]}`,
				``,
				`data: [DONE]`,
				``,
			}, "\n"))
		case 2:
			_, _ = io.WriteString(w, strings.Join([]string{
				`data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":1774462654,"model":"qwen/qwen3.5-9b","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
				``,
				`data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":1774462654,"model":"qwen/qwen3.5-9b","choices":[{"index":0,"delta":{"content":"The demo "}}]}`,
				``,
				`data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":1774462654,"model":"qwen/qwen3.5-9b","choices":[{"index":0,"delta":{"content":"users are ready."}}]}`,
				``,
				`data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":1774462654,"model":"qwen/qwen3.5-9b","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
				``,
				`data: [DONE]`,
				``,
			}, "\n"))
		default:
			t.Fatalf("unexpected upstream call count %d", upstreamCalls)
		}
	}))
	defer upstream.Close()

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
			EventID:  "evt-stream-sse",
			Decision: types.DecisionAllow,
			Result: &types.ExecutionResult{
				Status:     "success",
				OutputJSON: json.RawMessage(`{"row_count":1}`),
			},
		})
	}))
	defer gateway.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  gateway.URL,
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Defaults: DefaultsConfig{SessionPrefix: "support-bot"},
		Tools:    []ToolConfig{{Tool: "postgres", Action: "query.readonly", RiskScore: 2}},
		OpenAI: OpenAIConfig{
			UpstreamBaseURL: upstream.URL + "/v1",
			Model:           "qwen/qwen3.5-9b",
			ToolName:        "governed_action",
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"qwen/qwen3.5-9b","stream":true,"messages":[{"role":"user","content":"fetch users"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, `"governed_action"`) || !strings.Contains(body, `The demo `) || !strings.Contains(body, `users are ready.`) || !strings.Contains(body, `data: [DONE]`) {
		t.Fatalf("expected streamed follow-up assistant content, got %s", body)
	}
}

func TestBridgeChatCompletionsStreamsClientToolDeltasWithoutInterception(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Join([]string{
			`data: {"id":"chatcmpl-client-tool","object":"chat.completion.chunk","created":1774462653,"model":"qwen/qwen3.5-9b","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
			``,
			`data: {"id":"chatcmpl-client-tool","object":"chat.completion.chunk","created":1774462653,"model":"qwen/qwen3.5-9b","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"client-call-1","type":"function","function":{"name":"lookup_docs","arguments":"{\"query\":\"bridge doctor\"}"}}]},"finish_reason":"tool_calls"}]}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n"))
	}))
	defer upstream.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  "http://localhost:8080",
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Tools:    []ToolConfig{{Tool: "postgres", Action: "query.readonly", RiskScore: 2}},
		OpenAI: OpenAIConfig{
			UpstreamBaseURL: upstream.URL + "/v1",
			Model:           "qwen/qwen3.5-9b",
			ToolName:        "governed_action",
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"qwen/qwen3.5-9b","stream":true,"messages":[{"role":"user","content":"help me"}],"tools":[{"type":"function","function":{"name":"lookup_docs","parameters":{"type":"object"}}}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"name":"lookup_docs"`) || !strings.Contains(body, `data: [DONE]`) {
		t.Fatalf("expected raw upstream client tool deltas in stream output, got %s", body)
	}
}

func TestBridgeChatCompletionsReturnsStructuredGovernedResultsForMixedToolCalls(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
			EventID:  "evt-mixed",
			Decision: types.DecisionAllow,
			Result: &types.ExecutionResult{
				Status:     "success",
				OutputJSON: json.RawMessage(`{"row_count":1}`),
			},
		})
	}))
	defer gateway.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(openAIChatResponse{
			ID:     "chatcmpl-mixed",
			Object: "chat.completion",
			Model:  "qwen/qwen3.5-9b",
			Choices: []openAIChoice{{
				Index: 0,
				Message: openAIChatMessage{
					Role: "assistant",
					ToolCalls: []openAIToolCall{
						{
							ID:   "client-call-1",
							Type: "function",
							Function: openAIFunctionCall{
								Name:      "lookup_docs",
								Arguments: `{"query":"bridge doctor"}`,
							},
						},
						{
							ID:   "governed-call-1",
							Type: "function",
							Function: openAIFunctionCall{
								Name:      "governed_action",
								Arguments: `{"operation":"postgres.query.readonly","params":{"sql":"select 1","params":[]}}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			}},
		})
	}))
	defer upstream.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  gateway.URL,
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Tools:    []ToolConfig{{Tool: "postgres", Action: "query.readonly", RiskScore: 2}},
		OpenAI: OpenAIConfig{
			UpstreamBaseURL: upstream.URL + "/v1",
			Model:           "qwen/qwen3.5-9b",
			ToolName:        "governed_action",
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"qwen/qwen3.5-9b","messages":[{"role":"user","content":"help me"}],"tools":[{"type":"function","function":{"name":"lookup_docs","parameters":{"type":"object"}}}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp openAIChatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected one choice, got %+v", resp)
	}
	if len(resp.Choices[0].Message.ToolCalls) != 1 || resp.Choices[0].Message.ToolCalls[0].Function.Name != "lookup_docs" {
		t.Fatalf("expected client tool call to remain available, got %+v", resp.Choices[0].Message.ToolCalls)
	}
	if resp.OpenClause == nil || len(resp.OpenClause.GovernedResults) != 1 {
		t.Fatalf("expected one governed result inside openclause envelope, got %+v", resp.OpenClause)
	}
	if resp.OpenClause.GovernedResults[0].ToolCallID != "governed-call-1" || resp.OpenClause.GovernedResults[0].Tool != "postgres" || resp.OpenClause.GovernedResults[0].Action != "query.readonly" {
		t.Fatalf("expected governed result metadata, got %+v", resp.OpenClause.GovernedResults[0])
	}
	if resp.OpenClause.GovernedResults[0].EventID != "evt-mixed" || resp.OpenClause.GovernedResults[0].Decision != string(types.DecisionAllow) {
		t.Fatalf("expected governed result event payload, got %+v", resp.OpenClause.GovernedResults[0])
	}
	if resp.Choices[0].Message.Content != nil && strings.Contains(fmt.Sprint(resp.Choices[0].Message.Content), "OpenClause bridge already executed") {
		t.Fatalf("expected governed execution note to stay machine-readable, got %+v", resp.Choices[0].Message.Content)
	}
}

func TestBridgeChatCompletionsStreamsStructuredGovernedResultsForMixedToolCalls(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
			EventID:  "evt-mixed-stream",
			Decision: types.DecisionAllow,
			Result: &types.ExecutionResult{
				Status:     "success",
				OutputJSON: json.RawMessage(`{"row_count":1}`),
			},
		})
	}))
	defer gateway.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Join([]string{
			`data: {"id":"chatcmpl-mixed-stream","object":"chat.completion.chunk","created":1774462653,"model":"qwen/qwen3.5-9b","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
			``,
			`data: {"id":"chatcmpl-mixed-stream","object":"chat.completion.chunk","created":1774462653,"model":"qwen/qwen3.5-9b","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"client-call-1","type":"function","function":{"name":"lookup_docs","arguments":"{\"query\":\"bridge doctor\"}"}},{"index":1,"id":"governed-call-1","type":"function","function":{"name":"governed_action","arguments":"{\"operation\":\"postgres.query.readonly\",\"params\":{\"sql\":\"select 1\",\"params\":[]}}"}}]},"finish_reason":"tool_calls"}]}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n"))
	}))
	defer upstream.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  gateway.URL,
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Tools:    []ToolConfig{{Tool: "postgres", Action: "query.readonly", RiskScore: 2}},
		OpenAI: OpenAIConfig{
			UpstreamBaseURL: upstream.URL + "/v1",
			Model:           "qwen/qwen3.5-9b",
			ToolName:        "governed_action",
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"qwen/qwen3.5-9b","stream":true,"messages":[{"role":"user","content":"help me"}],"tools":[{"type":"function","function":{"name":"lookup_docs","parameters":{"type":"object"}}}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"name":"lookup_docs"`) || !strings.Contains(body, `"openclause":{"governed_results":[{"tool_call_id":"governed-call-1","event_id":"evt-mixed-stream","decision":"allow"`) || !strings.Contains(body, `data: [DONE]`) {
		t.Fatalf("expected mixed-tool stream to preserve client tools and emit governed results, got %s", body)
	}
	if strings.Contains(body, "OpenClause bridge already executed the governed action") {
		t.Fatalf("expected machine-readable governed results instead of assistant note, got %s", body)
	}
}

func TestBridgeChatCompletionsFailsExplicitlyWhenToolLoopExceedsLimit(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		_ = json.NewEncoder(w).Encode(openAIChatResponse{
			ID:     fmt.Sprintf("chatcmpl-loop-%d", upstreamCalls),
			Object: "chat.completion",
			Model:  "qwen/qwen3.5-9b",
			Choices: []openAIChoice{{
				Index: 0,
				Message: openAIChatMessage{
					Role: "assistant",
					ToolCalls: []openAIToolCall{{
						ID:   fmt.Sprintf("call-%d", upstreamCalls),
						Type: "function",
						Function: openAIFunctionCall{
							Name:      "governed_action",
							Arguments: `{"operation":"postgres.query.readonly","params":{"sql":"select 1","params":[]}}`,
						},
					}},
				},
				FinishReason: "tool_calls",
			}},
		})
	}))
	defer upstream.Close()

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
			EventID:  "evt-loop",
			Decision: types.DecisionAllow,
			Result: &types.ExecutionResult{
				Status:     "success",
				OutputJSON: json.RawMessage(`{"row_count":1}`),
			},
		})
	}))
	defer gateway.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  gateway.URL,
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Tools:    []ToolConfig{{Tool: "postgres", Action: "query.readonly", RiskScore: 2}},
		OpenAI: OpenAIConfig{
			UpstreamBaseURL: upstream.URL + "/v1",
			Model:           "qwen/qwen3.5-9b",
			ToolName:        "governed_action",
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"qwen/qwen3.5-9b","messages":[{"role":"user","content":"keep going forever"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "bridge chat exceeded") {
		t.Fatalf("expected explicit tool loop exhaustion error, got %s", rec.Body.String())
	}
	if upstreamCalls != maxBridgeChatLoopSteps {
		t.Fatalf("expected loop to stop after %d steps, got %d", maxBridgeChatLoopSteps, upstreamCalls)
	}
}

func TestBridgeChatCompletionsStreamRunsGovernedToolLoop(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		var req openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode upstream chat request: %v", err)
		}
		upstreamCalls++
		switch upstreamCalls {
		case 1:
			_ = json.NewEncoder(w).Encode(openAIChatResponse{
				ID:     "chatcmpl-1",
				Object: "chat.completion",
				Model:  "qwen/qwen3.5-9b",
				Choices: []openAIChoice{{
					Index: 0,
					Message: openAIChatMessage{
						Role: "assistant",
						ToolCalls: []openAIToolCall{{
							ID:   "call-1",
							Type: "function",
							Function: openAIFunctionCall{
								Name:      "governed_action",
								Arguments: `{"operation":"postgres.query.readonly","params":{"sql":"select 1","params":[]}}`,
							},
						}},
					},
					FinishReason: "tool_calls",
				}},
			})
		case 2:
			if len(req.Messages) < 3 {
				t.Fatalf("expected tool result conversation turn, got %+v", req.Messages)
			}
			last := req.Messages[len(req.Messages)-1]
			if last.Role != "tool" || !strings.Contains(asString(t, last.Content), `"decision":"allow"`) {
				t.Fatalf("expected governed tool result in follow-up stream request, got %+v", last)
			}
			_ = json.NewEncoder(w).Encode(openAIChatResponse{
				ID:      "chatcmpl-2",
				Object:  "chat.completion",
				Created: 1774462653,
				Model:   "qwen/qwen3.5-9b",
				Choices: []openAIChoice{{
					Index: 0,
					Message: openAIChatMessage{
						Role:    "assistant",
						Content: "The newest demo users are Alice, Bob, and Charlie.",
					},
					FinishReason: "stop",
				}},
			})
		default:
			t.Fatalf("unexpected upstream call count %d", upstreamCalls)
		}
	}))
	defer upstream.Close()

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/toolcalls" {
			t.Fatalf("unexpected gateway path %s", r.URL.Path)
		}
		var req types.ToolCallRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode gateway request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(types.ToolCallResponse{
			EventID:  "evt-stream-1",
			Decision: types.DecisionAllow,
			Reason:   "allowed",
			Result: &types.ExecutionResult{
				Status:     "success",
				OutputJSON: json.RawMessage(`{"row_count":3}`),
			},
		})
	}))
	defer gateway.Close()

	cfg, err := ResolveConfig(Config{
		BaseURL:  gateway.URL,
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
		Defaults: DefaultsConfig{
			UserID:        "pilot-user",
			SessionPrefix: "support-bot",
			RiskMode:      "configured",
		},
		Tools: []ToolConfig{
			{Tool: "postgres", Action: "query.readonly", RiskScore: 2},
		},
		OpenAI: OpenAIConfig{
			UpstreamBaseURL: upstream.URL + "/v1",
			Model:           "qwen/qwen3.5-9b",
			ToolName:        "governed_action",
		},
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"qwen/qwen3.5-9b","stream":true,"messages":[{"role":"user","content":"fetch users"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Alice, Bob, and Charlie") || !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected streamed assistant response after tool loop, got %s", body)
	}
}

func TestServeReturnsOnContextCancel(t *testing.T) {
	cfg, err := ResolveConfig(Config{
		Listen:   "127.0.0.1:0",
		BaseURL:  "http://localhost:8080",
		TenantID: "tenant-1",
		AgentID:  "agent-1",
		APIKey:   "sk-oc-bridge",
	})
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Serve(ctx, cfg, nil); err != nil {
		t.Fatalf("Serve returned error on canceled context: %v", err)
	}
}

func asString(t *testing.T, value any) string {
	t.Helper()
	text, ok := value.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", value)
	}
	return text
}
