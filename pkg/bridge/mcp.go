package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/bturcanu/OpenClause/pkg/types"
	"github.com/google/uuid"
)

const mcpProtocolVersion = "2025-06-18"
const mcpSessionHeader = "Mcp-Session-Id"

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   any             `json:"error,omitempty"`
}

type mcpResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *mcpError `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpInitializeParams struct {
	ProtocolVersion string `json:"protocolVersion,omitempty"`
}

type mcpToolListResult struct {
	Tools []mcpTool `json:"tools"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type mcpToolCallResult struct {
	Content           []mcpTextContent `json:"content"`
	StructuredContent any              `json:"structuredContent,omitempty"`
	IsError           bool             `json:"isError,omitempty"`
}

type mcpTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpPromptListResult struct {
	Prompts []any `json:"prompts"`
}

type mcpResourceListResult struct {
	Resources []any `json:"resources"`
}

type mcpStdioFormat int

const (
	mcpStdioFormatUnknown mcpStdioFormat = iota
	mcpStdioFormatLine
	mcpStdioFormatContentLength
)

func (s *Server) ServeMCPStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	writer := bufio.NewWriter(out)
	profile := s.defaultProfile

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		payload, detectedFormat, err := readMCPStdioPayload(reader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read MCP stdio message: %w", err)
		}
		if len(payload) == 0 {
			return nil
		}
		if detectedFormat == mcpStdioFormatUnknown {
			return fmt.Errorf("read MCP stdio message: unknown message framing")
		}

		responses, respond, _, err := s.handleMCPPayload(ctx, profile, payload)
		if err != nil {
			return err
		}
		if !respond {
			continue
		}
		for _, item := range responses {
			encoded, err := json.Marshal(item)
			if err != nil {
				return fmt.Errorf("encode MCP stdio response: %w", err)
			}
			if err := writeMCPStdioPayload(writer, detectedFormat, encoded); err != nil {
				return fmt.Errorf("write MCP stdio response: %w", err)
			}
		}
		if err := writer.Flush(); err != nil {
			return fmt.Errorf("flush MCP stdio response: %w", err)
		}
	}
}

func readMCPStdioPayload(reader *bufio.Reader) ([]byte, mcpStdioFormat, error) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && strings.TrimSpace(line) == "" {
				return nil, mcpStdioFormatUnknown, io.EOF
			}
			if err != io.EOF {
				return nil, mcpStdioFormatUnknown, err
			}
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if err == io.EOF {
				return nil, mcpStdioFormatUnknown, io.EOF
			}
			continue
		}

		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "content-length:") {
			length, parseErr := parseMCPContentLengthHeader(trimmed)
			if parseErr != nil {
				return nil, mcpStdioFormatUnknown, parseErr
			}
			for {
				headerLine, headerErr := reader.ReadString('\n')
				if headerErr != nil {
					return nil, mcpStdioFormatUnknown, headerErr
				}
				headerTrimmed := strings.TrimSpace(headerLine)
				if headerTrimmed == "" {
					break
				}
				headerLower := strings.ToLower(headerTrimmed)
				if strings.HasPrefix(headerLower, "content-length:") {
					length, parseErr = parseMCPContentLengthHeader(headerTrimmed)
					if parseErr != nil {
						return nil, mcpStdioFormatUnknown, parseErr
					}
				}
			}
			payload := make([]byte, length)
			if _, err := io.ReadFull(reader, payload); err != nil {
				return nil, mcpStdioFormatUnknown, err
			}
			return payload, mcpStdioFormatContentLength, nil
		}

		return []byte(trimmed), mcpStdioFormatLine, nil
	}
}

func parseMCPContentLengthHeader(header string) (int, error) {
	parts := strings.SplitN(header, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid MCP Content-Length header")
	}
	length, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || length < 0 {
		return 0, fmt.Errorf("invalid MCP Content-Length header")
	}
	return length, nil
}

func writeMCPStdioPayload(writer *bufio.Writer, format mcpStdioFormat, payload []byte) error {
	switch format {
	case mcpStdioFormatContentLength:
		if _, err := fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
			return err
		}
		_, err := writer.Write(payload)
		return err
	default:
		_, err := writer.Write(append(payload, '\n'))
		return err
	}
}

func (s *Server) handleMCPHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Allow", "POST, DELETE")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	case http.MethodPost:
	case http.MethodDelete:
		sessionID := strings.TrimSpace(r.Header.Get(mcpSessionHeader))
		if sessionID == "" {
			writeAPIError(w, types.ErrBadRequest("MCP session id required"))
			return
		}
		s.deleteMCPSession(sessionID)
		w.WriteHeader(http.StatusNoContent)
		return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	profile, sessionID, err := s.resolveMCPHTTPProfile(r)
	if err != nil {
		writeBridgeError(w, err)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeAPIError(w, types.ErrInternal("failed to read MCP request"))
		return
	}

	responses, respond, initialize, err := s.handleMCPPayload(r.Context(), profile, body)
	if err != nil {
		writeBridgeError(w, err)
		return
	}
	if initialize && sessionID == "" {
		sessionID = s.createMCPSession(profile.name)
		w.Header().Set(mcpSessionHeader, sessionID)
	}
	if sessionID != "" {
		w.Header().Set(mcpSessionHeader, sessionID)
	}
	w.Header().Set("Mcp-Protocol-Version", mcpProtocolVersion)
	if !respond {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if len(responses) == 1 {
		writeJSON(w, http.StatusOK, responses[0])
		return
	}
	writeJSON(w, http.StatusOK, responses)
}

func (s *Server) resolveMCPHTTPProfile(r *http.Request) (*profileRuntime, string, error) {
	sessionID := strings.TrimSpace(r.Header.Get(mcpSessionHeader))
	if sessionID != "" {
		profileName, ok := s.lookupMCPSession(sessionID)
		if !ok {
			return nil, "", types.ErrBadRequest("unknown MCP session")
		}
		profile, err := s.profileForName(profileName)
		return profile, sessionID, err
	}
	profile, err := s.profileForRequest(r)
	return profile, "", err
}

func (s *Server) handleMCPPayload(ctx context.Context, profile *profileRuntime, body []byte) ([]mcpResponse, bool, bool, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return []mcpResponse{{
			JSONRPC: "2.0",
			Error:   &mcpError{Code: -32600, Message: "empty JSON-RPC payload"},
		}}, true, false, nil
	}

	if strings.HasPrefix(trimmed, "[") {
		var batch []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &batch); err != nil {
			return []mcpResponse{{
				JSONRPC: "2.0",
				Error:   &mcpError{Code: -32700, Message: "invalid JSON"},
			}}, true, false, nil
		}
		responses := make([]mcpResponse, 0, len(batch))
		initialize := false
		for _, raw := range batch {
			response, respond, itemInitialize := s.handleMCPRequest(ctx, profile, raw)
			initialize = initialize || itemInitialize
			if respond {
				responses = append(responses, response)
			}
		}
		return responses, len(responses) > 0, initialize, nil
	}

	response, respond, initialize := s.handleMCPRequest(ctx, profile, []byte(trimmed))
	if !respond {
		return nil, false, initialize, nil
	}
	return []mcpResponse{response}, true, initialize, nil
}

func (s *Server) handleMCPRequest(ctx context.Context, profile *profileRuntime, raw json.RawMessage) (mcpResponse, bool, bool) {
	var req mcpRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return mcpResponse{
			JSONRPC: "2.0",
			Error:   &mcpError{Code: -32700, Message: "invalid JSON-RPC message"},
		}, true, false
	}
	if req.JSONRPC != "2.0" {
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcpError{Code: -32600, Message: "jsonrpc must be 2.0"},
		}, true, false
	}
	if strings.TrimSpace(req.Method) == "" {
		if req.ID == nil {
			return mcpResponse{}, false, false
		}
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcpError{Code: -32600, Message: "method is required"},
		}, true, false
	}

	result, err := s.dispatchMCPMethod(ctx, profile, req.Method, req.Params)
	if req.ID == nil {
		return mcpResponse{}, false, strings.TrimSpace(req.Method) == "initialize"
	}
	if err != nil {
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   err,
		}, true, strings.TrimSpace(req.Method) == "initialize"
	}
	return mcpResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}, true, strings.TrimSpace(req.Method) == "initialize"
}

func (s *Server) dispatchMCPMethod(ctx context.Context, profile *profileRuntime, method string, params json.RawMessage) (any, *mcpError) {
	switch method {
	case "initialize":
		return s.mcpInitialize(params)
	case "notifications/initialized":
		return map[string]any{}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return s.mcpToolsList(profile), nil
	case "tools/call":
		return s.mcpToolsCall(ctx, profile, params)
	case "prompts/list":
		return mcpPromptListResult{Prompts: []any{}}, nil
	case "resources/list":
		return mcpResourceListResult{Resources: []any{}}, nil
	default:
		return nil, &mcpError{Code: -32601, Message: "method not found"}
	}
}

func (s *Server) mcpInitialize(params json.RawMessage) (any, *mcpError) {
	if len(params) > 0 {
		var initParams mcpInitializeParams
		if err := json.Unmarshal(params, &initParams); err != nil {
			return nil, &mcpError{Code: -32602, Message: "invalid initialize params"}
		}
	}
	return map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{
				"listChanged": false,
			},
		},
		"serverInfo": map[string]any{
			"name":    "openclause-bridge",
			"version": "v0.5-alpha",
		},
	}, nil
}

func (s *Server) mcpToolsList(profile *profileRuntime) mcpToolListResult {
	tools := make([]mcpTool, 0, len(profile.cfg.Tools))
	for _, item := range profile.cfg.Tools {
		tools = append(tools, mcpTool{
			Name:        mcpToolName(item),
			Description: mcpToolDescription(item),
			InputSchema: mcpInputSchema(item),
		})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return mcpToolListResult{Tools: tools}
}

func (s *Server) mcpToolsCall(ctx context.Context, profile *profileRuntime, params json.RawMessage) (any, *mcpError) {
	var req mcpToolCallParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &mcpError{Code: -32602, Message: "invalid tools/call params"}
	}
	toolCfg, ok := s.lookupMCPTool(profile, req.Name)
	if !ok {
		return nil, &mcpError{Code: -32602, Message: "unknown MCP tool"}
	}

	callReq, err := mcpToolCallRequest(toolCfg, req.Arguments)
	if err != nil {
		return nil, &mcpError{Code: -32602, Message: err.Error()}
	}

	resp, submitted, submitErr := s.submitToolCall(ctx, profile, callReq)
	if submitErr != nil {
		return nil, &mcpError{Code: -32000, Message: submitErr.Error()}
	}

	payload := map[string]any{
		"event_id":   resp.EventID,
		"decision":   resp.Decision,
		"reason":     resp.Reason,
		"tool":       submitted.Tool,
		"action":     submitted.Action,
		"session_id": submitted.SessionID,
		"trace_id":   submitted.TraceID,
	}
	if strings.TrimSpace(resp.ApprovalURL) != "" {
		payload["approval_url"] = resp.ApprovalURL
	}
	if resp.Result != nil && len(resp.Result.OutputJSON) > 0 {
		var structured any
		if err := json.Unmarshal(resp.Result.OutputJSON, &structured); err == nil {
			payload["result"] = structured
		} else {
			payload["result"] = json.RawMessage(resp.Result.OutputJSON)
		}
	}

	encoded, _ := json.Marshal(payload)

	return mcpToolCallResult{
		Content: []mcpTextContent{{
			Type: "text",
			Text: string(encoded),
		}},
		StructuredContent: payload,
		IsError:           resp.Decision == types.DecisionDeny,
	}, nil
}

func (s *Server) lookupMCPTool(profile *profileRuntime, name string) (ToolConfig, bool) {
	for _, item := range profile.cfg.Tools {
		if mcpToolName(item) == strings.TrimSpace(name) {
			return item, true
		}
	}
	return ToolConfig{}, false
}

func (s *Server) createMCPSession(profileName string) string {
	sessionID := uuid.NewString()
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()
	s.mcpSessions[sessionID] = profileName
	return sessionID
}

func (s *Server) lookupMCPSession(sessionID string) (string, bool) {
	s.mcpMu.RLock()
	defer s.mcpMu.RUnlock()
	profileName, ok := s.mcpSessions[sessionID]
	return profileName, ok
}

func (s *Server) deleteMCPSession(sessionID string) {
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()
	delete(s.mcpSessions, sessionID)
}

func mcpToolName(tool ToolConfig) string {
	return sanitizeMCPName("openclause_" + tool.Tool + "_" + tool.Action)
}

func mcpToolDescription(tool ToolConfig) string {
	if strings.TrimSpace(tool.Description) != "" {
		return fmt.Sprintf("Governed OpenClause tool for %s.%s. %s", tool.Tool, tool.Action, strings.TrimSpace(tool.Description))
	}
	return fmt.Sprintf("Governed OpenClause tool for %s.%s", tool.Tool, tool.Action)
}

func sanitizeMCPName(value string) string {
	value = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '_':
			return r
		default:
			return '_'
		}
	}, value)
	for strings.Contains(value, "__") {
		value = strings.ReplaceAll(value, "__", "_")
	}
	return strings.Trim(value, "_")
}

func mcpInputSchema(tool ToolConfig) map[string]any {
	switch {
	case tool.Tool == "postgres" && tool.Action == "query.readonly":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sql": map[string]any{
					"type":        "string",
					"description": "Readonly SQL query to execute through OpenClause.",
				},
				"params": map[string]any{
					"type":        "array",
					"description": "Ordered bind parameters for the SQL query.",
					"items": map[string]any{
						"type": []string{"string", "number", "boolean", "null"},
					},
				},
			},
			"required": []string{"sql"},
		}
	case strings.HasSuffix(tool.Action, "channel.list"):
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	case strings.HasSuffix(tool.Action, "msg.post"):
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"channel": map[string]any{"type": "string", "description": "Slack channel id or name."},
				"text":    map[string]any{"type": "string", "description": "Message text to post."},
			},
			"required": []string{"channel", "text"},
		}
	case strings.HasSuffix(tool.Action, "issue.list"):
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_key": map[string]any{"type": "string", "description": "Jira project key."},
				"limit":       map[string]any{"type": "number", "description": "Maximum issues to return."},
			},
		}
	case strings.HasSuffix(tool.Action, "issue.create"):
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_key": map[string]any{"type": "string", "description": "Jira project key."},
				"summary":     map[string]any{"type": "string", "description": "Issue summary."},
				"description": map[string]any{"type": "string", "description": "Issue description."},
				"issue_type":  map[string]any{"type": "string", "description": "Issue type such as Task or Bug."},
			},
			"required": []string{"project_key", "summary"},
		}
	case tool.Tool == "webhook" && tool.Action == "post":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url":     map[string]any{"type": "string", "description": "Webhook URL."},
				"payload": map[string]any{"type": "object", "description": "JSON payload to send."},
			},
			"required": []string{"url", "payload"},
		}
	default:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"params": map[string]any{
					"type":        "object",
					"description": "Tool parameters to send through OpenClause.",
				},
				"resource": map[string]any{
					"type":        "string",
					"description": "Optional resource identifier for policy context.",
				},
			},
		}
	}
}

func mcpToolCallRequest(tool ToolConfig, args map[string]any) (types.ToolCallRequest, error) {
	params := map[string]any{}
	resource := ""

	switch {
	case tool.Tool == "postgres" && tool.Action == "query.readonly":
		sql, _ := args["sql"].(string)
		if strings.TrimSpace(sql) == "" {
			return types.ToolCallRequest{}, fmt.Errorf("sql is required")
		}
		params["sql"] = strings.TrimSpace(sql)
		if provided, ok := args["params"].([]any); ok {
			params["params"] = provided
		} else {
			params["params"] = []any{}
		}
	default:
		if rawParams, ok := args["params"].(map[string]any); ok {
			params = rawParams
		} else {
			for key, value := range args {
				if key == "resource" {
					continue
				}
				params[key] = value
			}
		}
		if rawResource, ok := args["resource"].(string); ok {
			resource = strings.TrimSpace(rawResource)
		}
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return types.ToolCallRequest{}, fmt.Errorf("invalid tool arguments")
	}
	return types.ToolCallRequest{
		Tool:     tool.Tool,
		Action:   tool.Action,
		Params:   paramsJSON,
		Resource: resource,
	}, nil
}
