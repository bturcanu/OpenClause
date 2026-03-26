package bridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bturcanu/OpenClause/pkg/types"
)

type openAIModelsResponse struct {
	Object string        `json:"object"`
	Data   []openAIModel `json:"data"`
}

type openAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by,omitempty"`
}

type openAIChatRequest struct {
	Model       string              `json:"model,omitempty"`
	Messages    []openAIChatMessage `json:"messages"`
	Temperature *float64            `json:"temperature,omitempty"`
	MaxTokens   *int                `json:"max_tokens,omitempty"`
	TopP        *float64            `json:"top_p,omitempty"`
	Stream      bool                `json:"stream,omitempty"`
	Tools       []openAITool        `json:"tools,omitempty"`
	ToolChoice  any                 `json:"tool_choice,omitempty"`
}

type openAIChatResponse struct {
	ID                string           `json:"id,omitempty"`
	Object            string           `json:"object,omitempty"`
	Created           int64            `json:"created,omitempty"`
	Model             string           `json:"model,omitempty"`
	Choices           []openAIChoice   `json:"choices"`
	OpenClause        *openAIExtension `json:"openclause,omitempty"`
	Usage             any              `json:"usage,omitempty"`
	SystemFingerprint any              `json:"system_fingerprint,omitempty"`
	Stats             any              `json:"stats,omitempty"`
}

type openAIChoice struct {
	Index        int               `json:"index"`
	Message      openAIChatMessage `json:"message"`
	LogProbs     any               `json:"logprobs,omitempty"`
	FinishReason string            `json:"finish_reason,omitempty"`
}

type openAIChatMessage struct {
	Role             string           `json:"role"`
	Content          any              `json:"content,omitempty"`
	Name             string           `json:"name,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	ToolCalls        []openAIToolCall `json:"tool_calls,omitempty"`
	ReasoningContent any              `json:"reasoning_content,omitempty"`
}

type openAIGovernedResult struct {
	ToolCallID  string `json:"tool_call_id,omitempty"`
	EventID     string `json:"event_id,omitempty"`
	Decision    string `json:"decision,omitempty"`
	Reason      string `json:"reason,omitempty"`
	ApprovalURL string `json:"approval_url,omitempty"`
	Tool        string `json:"tool,omitempty"`
	Action      string `json:"action,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	TraceID     string `json:"trace_id,omitempty"`
	Result      any    `json:"result,omitempty"`
}

type openAIExtension struct {
	GovernedResults []openAIGovernedResult `json:"governed_results,omitempty"`
}

type openAITool struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type governedActionArgs struct {
	Operation string         `json:"operation"`
	Params    map[string]any `json:"params,omitempty"`
}

const maxBridgeChatLoopSteps = 8

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	profile, err := s.profileForRequest(r)
	if err != nil {
		writeBridgeError(w, err)
		return
	}
	if !profile.cfg.OpenAI.Enabled {
		writeAPIError(w, &types.APIError{
			Code:      "NOT_CONFIGURED",
			Message:   "openai chat host is not configured for this bridge",
			HTTPCode:  http.StatusNotImplemented,
			Retryable: false,
		})
		return
	}
	resp, err := s.proxyOpenAIRequest(r.Context(), profile, http.MethodGet, profile.cfg.OpenAI.UpstreamBaseURL+"/models", nil)
	if err != nil {
		writeBridgeError(w, err)
		return
	}
	defer resp.Body.Close()
	copyUpstreamJSON(w, resp)
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	profile, err := s.profileForRequest(r)
	if err != nil {
		writeBridgeError(w, err)
		return
	}
	if !profile.cfg.OpenAI.Enabled {
		writeAPIError(w, &types.APIError{
			Code:      "NOT_CONFIGURED",
			Message:   "openai chat host is not configured for this bridge",
			HTTPCode:  http.StatusNotImplemented,
			Retryable: false,
		})
		return
	}
	var req openAIChatRequest
	if err := decodeOpenAIJSON(r.Body, &req); err != nil {
		writeAPIError(w, types.ErrBadRequest("invalid OpenAI chat completion JSON"))
		return
	}
	if len(req.Messages) == 0 {
		writeAPIError(w, types.ErrBadRequest("messages are required"))
		return
	}
	mergedTools, err := buildUpstreamTools(profile, req.Tools)
	if err != nil {
		writeAPIError(w, types.ErrBadRequest(err.Error()))
		return
	}
	if req.Stream {
		if err := s.streamOpenAIChatLoop(r.Context(), profile, req, mergedTools, w); err != nil {
			writeBridgeError(w, err)
		}
		return
	}

	resp, err := s.runOpenAIChatLoop(r.Context(), profile, req, mergedTools)
	if err != nil {
		writeBridgeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) runOpenAIChatLoop(ctx context.Context, profile *profileRuntime, req openAIChatRequest, upstreamTools []openAITool) (*openAIChatResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = profile.cfg.OpenAI.Model
	}
	messages := append([]openAIChatMessage{}, req.Messages...)
	if strings.TrimSpace(profile.cfg.OpenAI.SystemPrompt) != "" {
		messages = append([]openAIChatMessage{{
			Role:    "system",
			Content: profile.cfg.OpenAI.SystemPrompt,
		}}, messages...)
	}

	var last *openAIChatResponse
	for step := 0; step < maxBridgeChatLoopSteps; step++ {
		upstreamReq := openAIChatRequest{
			Model:       model,
			Messages:    messages,
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
			TopP:        req.TopP,
			Tools:       upstreamTools,
			ToolChoice:  bridgeToolChoice(req.ToolChoice),
		}
		resp, err := s.postOpenAIChat(ctx, profile, upstreamReq)
		if err != nil {
			return nil, err
		}
		last = resp
		if len(resp.Choices) == 0 {
			return resp, nil
		}
		message := resp.Choices[0].Message
		if len(message.ToolCalls) == 0 {
			return resp, nil
		}
		hasGoverned, hasClient := classifyToolCalls(profile, message.ToolCalls)
		if hasClient && !hasGoverned {
			return resp, nil
		}
		if hasGoverned && hasClient {
			governedCalls, clientCalls := splitToolCalls(profile, message.ToolCalls)
			executions, err := s.executeGovernedToolCalls(ctx, profile, governedCalls)
			if err != nil {
				return nil, err
			}
			message.ToolCalls = clientCalls
			resp.Choices[0].Message = message
			resp.Choices[0].FinishReason = "tool_calls"
			resp.OpenClause = openAIExtensionForResults(governedExecutionResults(executions))
			return resp, nil
		}

		messages = append(messages, message)
		for _, call := range message.ToolCalls {
			content, err := s.executeGovernedToolCall(ctx, profile, call)
			if err != nil {
				return nil, err
			}
			messages = append(messages, openAIChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Name:       call.Function.Name,
				Content:    content,
			})
		}
	}
	if last != nil {
		if len(last.Choices) > 0 && len(last.Choices[0].Message.ToolCalls) > 0 {
			return nil, &types.APIError{
				Code:      "BRIDGE_TOOL_LOOP_EXHAUSTED",
				Message:   fmt.Sprintf("bridge chat exceeded %d governed tool steps", maxBridgeChatLoopSteps),
				Retryable: false,
				HTTPCode:  http.StatusBadGateway,
			}
		}
		return last, nil
	}
	return nil, types.ErrInternal("chat loop did not produce a response")
}

type openAIStreamStepResult struct {
	ID           string
	Object       string
	Created      int64
	Model        string
	Message      openAIChatMessage
	FinishReason string
}

func (s *Server) streamOpenAIChatLoop(ctx context.Context, profile *profileRuntime, req openAIChatRequest, upstreamTools []openAITool, w http.ResponseWriter) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return types.ErrInternal("streaming is not supported by this HTTP server")
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = profile.cfg.OpenAI.Model
	}
	messages := append([]openAIChatMessage{}, req.Messages...)
	if strings.TrimSpace(profile.cfg.OpenAI.SystemPrompt) != "" {
		messages = append([]openAIChatMessage{{
			Role:    "system",
			Content: profile.cfg.OpenAI.SystemPrompt,
		}}, messages...)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	roleSent := false
	var lastMeta openAIStreamStepResult
	for step := 0; step < maxBridgeChatLoopSteps; step++ {
		upstreamReq := openAIChatRequest{
			Model:       model,
			Messages:    messages,
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
			TopP:        req.TopP,
			Stream:      true,
			Tools:       upstreamTools,
			ToolChoice:  bridgeToolChoice(req.ToolChoice),
		}
		result, sentRoleThisStep, err := s.streamOpenAIUpstreamStep(ctx, profile, upstreamReq, w, flusher, roleSent)
		if err != nil {
			return err
		}
		roleSent = roleSent || sentRoleThisStep
		lastMeta = result
		if len(result.Message.ToolCalls) == 0 {
			writeOpenAIStreamChunk(w, flusher, openAIChatChunk{
				ID:      result.ID,
				Object:  "chat.completion.chunk",
				Created: result.Created,
				Model:   result.Model,
				Choices: []openAIChunkChoice{{
					Index:        0,
					Delta:        openAIChunkDelta{},
					FinishReason: result.FinishReason,
				}},
			})
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			flusher.Flush()
			return nil
		}
		hasGoverned, hasClient := classifyToolCalls(profile, result.Message.ToolCalls)
		if hasClient && !hasGoverned {
			writeOpenAIStreamToolCalls(w, flusher, result.ID, result.Created, result.Model, result.Message.ToolCalls)
			writeOpenAIStreamFinish(w, flusher, result.ID, result.Created, result.Model, result.FinishReason)
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			flusher.Flush()
			return nil
		}
		if hasGoverned && hasClient {
			governedCalls, clientCalls := splitToolCalls(profile, result.Message.ToolCalls)
			executions, err := s.executeGovernedToolCalls(ctx, profile, governedCalls)
			if err != nil {
				return err
			}
			writeOpenAIStreamGovernedResults(w, flusher, result.ID, result.Created, result.Model, governedExecutionResults(executions))
			writeOpenAIStreamToolCalls(w, flusher, result.ID, result.Created, result.Model, clientCalls)
			writeOpenAIStreamFinish(w, flusher, result.ID, result.Created, result.Model, "tool_calls")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			flusher.Flush()
			return nil
		}

		messages = append(messages, result.Message)
		for _, call := range result.Message.ToolCalls {
			content, err := s.executeGovernedToolCall(ctx, profile, call)
			if err != nil {
				return err
			}
			messages = append(messages, openAIChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Name:       call.Function.Name,
				Content:    content,
			})
		}
	}

	if lastMeta.ID != "" {
		if len(lastMeta.Message.ToolCalls) > 0 {
			writeOpenAIStreamTerminalAssistantMessage(
				w,
				flusher,
				lastMeta.ID,
				lastMeta.Created,
				lastMeta.Model,
				fmt.Sprintf("Bridge stopped after %d governed tool steps. Retry with a shorter tool loop or simplify the prompt.", maxBridgeChatLoopSteps),
			)
			return nil
		}
		writeOpenAIStreamChunk(w, flusher, openAIChatChunk{
			ID:      lastMeta.ID,
			Object:  "chat.completion.chunk",
			Created: lastMeta.Created,
			Model:   lastMeta.Model,
			Choices: []openAIChunkChoice{{
				Index:        0,
				Delta:        openAIChunkDelta{},
				FinishReason: "stop",
			}},
		})
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
		return nil
	}
	return types.ErrInternal("chat stream did not produce a response")
}

func (s *Server) streamOpenAIUpstreamStep(ctx context.Context, profile *profileRuntime, req openAIChatRequest, w http.ResponseWriter, flusher http.Flusher, roleSent bool) (openAIStreamStepResult, bool, error) {
	resp, err := s.postOpenAIChatStream(ctx, profile, req)
	if err != nil {
		return openAIStreamStepResult{}, false, err
	}
	defer resp.Body.Close()

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "text/event-stream") {
		data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return openAIStreamStepResult{}, false, types.ErrInternal("failed to read OpenAI chat response")
		}
		var out openAIChatResponse
		if err := json.Unmarshal(data, &out); err != nil {
			return openAIStreamStepResult{}, false, types.ErrInternal("failed to decode OpenAI chat response")
		}
		if !roleSent {
			writeOpenAIStreamChunk(w, flusher, openAIChatChunk{
				ID:      out.ID,
				Object:  "chat.completion.chunk",
				Created: out.Created,
				Model:   out.Model,
				Choices: []openAIChunkChoice{{Index: 0, Delta: openAIChunkDelta{Role: "assistant"}}},
			})
			roleSent = true
		}
		if len(out.Choices) > 0 {
			for _, piece := range splitStreamContent(assistantMessageText(out.Choices[0].Message.Content), 80) {
				writeOpenAIStreamChunk(w, flusher, openAIChatChunk{
					ID:      out.ID,
					Object:  "chat.completion.chunk",
					Created: out.Created,
					Model:   out.Model,
					Choices: []openAIChunkChoice{{Index: 0, Delta: openAIChunkDelta{Content: piece}}},
				})
			}
			return openAIStreamStepResult{
				ID:           out.ID,
				Object:       out.Object,
				Created:      out.Created,
				Model:        out.Model,
				Message:      out.Choices[0].Message,
				FinishReason: out.Choices[0].FinishReason,
			}, roleSent, nil
		}
		return openAIStreamStepResult{ID: out.ID, Object: out.Object, Created: out.Created, Model: out.Model, FinishReason: "stop"}, roleSent, nil
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 2048), 1024*1024)

	result := openAIStreamStepResult{Object: "chat.completion.chunk"}
	accumulator := streamToolAccumulator{calls: map[int]*openAIToolCall{}}
	var eventData strings.Builder

	processEvent := func(payload string) error {
		payload = strings.TrimSpace(payload)
		if payload == "" {
			return nil
		}
		if payload == "[DONE]" {
			return io.EOF
		}
		var chunk openAIChatChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return types.ErrInternal("failed to decode OpenAI stream chunk")
		}
		if result.ID == "" {
			result.ID = chunk.ID
			result.Created = chunk.Created
			result.Model = chunk.Model
		}
		if len(chunk.Choices) == 0 {
			return nil
		}
		choice := chunk.Choices[0]
		if !roleSent && (choice.Delta.Role == "assistant" || choice.Delta.Content != "" || len(choice.Delta.ToolCalls) > 0) {
			writeOpenAIStreamChunk(w, flusher, openAIChatChunk{
				ID:      chunk.ID,
				Object:  "chat.completion.chunk",
				Created: chunk.Created,
				Model:   chunk.Model,
				Choices: []openAIChunkChoice{{Index: 0, Delta: openAIChunkDelta{Role: "assistant"}}},
			})
			roleSent = true
		}
		if choice.Delta.Content != "" {
			result.Message.Role = "assistant"
			result.Message.Content = assistantMessageText(result.Message.Content) + choice.Delta.Content
			writeOpenAIStreamChunk(w, flusher, openAIChatChunk{
				ID:      chunk.ID,
				Object:  "chat.completion.chunk",
				Created: chunk.Created,
				Model:   chunk.Model,
				Choices: []openAIChunkChoice{{Index: 0, Delta: openAIChunkDelta{Content: choice.Delta.Content}}},
			})
		}
		accumulator.apply(choice.Delta.ToolCalls)
		if choice.FinishReason != "" {
			result.FinishReason = choice.FinishReason
		}
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if err := processEvent(eventData.String()); err != nil {
				if err == io.EOF {
					break
				}
				return openAIStreamStepResult{}, roleSent, err
			}
			eventData.Reset()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if eventData.Len() > 0 {
				eventData.WriteByte('\n')
			}
			eventData.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return openAIStreamStepResult{}, roleSent, types.ErrInternal("failed to read OpenAI stream response")
	}
	if eventData.Len() > 0 {
		if err := processEvent(eventData.String()); err != nil && err != io.EOF {
			return openAIStreamStepResult{}, roleSent, err
		}
	}

	result.Message.ToolCalls = accumulator.callsList()
	return result, roleSent, nil
}

type streamToolAccumulator struct {
	calls map[int]*openAIToolCall
	order []int
}

func (a *streamToolAccumulator) apply(deltas []openAIToolCallDelta) {
	for _, delta := range deltas {
		call, ok := a.calls[delta.Index]
		if !ok {
			call = &openAIToolCall{Type: "function"}
			a.calls[delta.Index] = call
			a.order = append(a.order, delta.Index)
		}
		if strings.TrimSpace(delta.ID) != "" {
			call.ID = delta.ID
		}
		if strings.TrimSpace(delta.Type) != "" {
			call.Type = delta.Type
		}
		if strings.TrimSpace(delta.Function.Name) != "" {
			call.Function.Name = delta.Function.Name
		}
		if delta.Function.Arguments != "" {
			call.Function.Arguments += delta.Function.Arguments
		}
	}
}

func (a *streamToolAccumulator) callsList() []openAIToolCall {
	if len(a.order) == 0 {
		return nil
	}
	out := make([]openAIToolCall, 0, len(a.order))
	for _, index := range a.order {
		if call, ok := a.calls[index]; ok && call != nil {
			out = append(out, *call)
		}
	}
	return out
}

func (s *Server) buildGovernedActionTool(profile *profileRuntime) openAITool {
	return governedActionToolForProfile(profile)
}

func buildUpstreamTools(profile *profileRuntime, clientTools []openAITool) ([]openAITool, error) {
	governed := profile.cfg.OpenAI.ToolName
	tools := make([]openAITool, 0, len(clientTools)+1)
	for _, tool := range clientTools {
		name := strings.TrimSpace(tool.Function.Name)
		if name == "" {
			return nil, fmt.Errorf("client-provided tools require a non-empty function.name")
		}
		if strings.EqualFold(name, governed) {
			return nil, fmt.Errorf("client-provided tools cannot shadow the governed bridge tool %q", governed)
		}
		tools = append(tools, tool)
	}
	tools = append(tools, governedActionToolForProfile(profile))
	return tools, nil
}

func governedActionToolForProfile(profile *profileRuntime) openAITool {
	ops := make([]string, 0, len(profile.cfg.Tools))
	descriptions := make([]string, 0, len(profile.cfg.Tools))
	for _, item := range profile.cfg.Tools {
		op := item.Tool + "." + item.Action
		ops = append(ops, op)
		description := op
		if strings.TrimSpace(item.Description) != "" {
			description = fmt.Sprintf("%s: %s", op, item.Description)
		}
		descriptions = append(descriptions, description)
	}
	return openAITool{
		Type: "function",
		Function: openAIToolFunction{
			Name: profile.cfg.OpenAI.ToolName,
			Description: "Execute one governed OpenClause action through the local bridge. Available operations: " +
				strings.Join(descriptions, "; "),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"operation": map[string]any{
						"type":        "string",
						"description": "One configured governed operation in tool.action form.",
						"enum":        ops,
					},
					"params": map[string]any{
						"type":        "object",
						"description": "Parameters for the selected governed operation.",
					},
				},
				"required": []string{"operation"},
			},
		},
	}
}

func bridgeToolChoice(value any) any {
	if value == nil {
		return "auto"
	}
	return value
}

func classifyToolCalls(profile *profileRuntime, calls []openAIToolCall) (hasGoverned bool, hasClient bool) {
	for _, call := range calls {
		if isGovernedToolCall(profile, call) {
			hasGoverned = true
			continue
		}
		hasClient = true
	}
	return hasGoverned, hasClient
}

func splitToolCalls(profile *profileRuntime, calls []openAIToolCall) (governed []openAIToolCall, client []openAIToolCall) {
	for _, call := range calls {
		if isGovernedToolCall(profile, call) {
			governed = append(governed, call)
			continue
		}
		client = append(client, call)
	}
	return governed, client
}

func isGovernedToolCall(profile *profileRuntime, call openAIToolCall) bool {
	return strings.EqualFold(strings.TrimSpace(call.Function.Name), strings.TrimSpace(profile.cfg.OpenAI.ToolName))
}

type governedExecution struct {
	Call   openAIToolCall
	Result openAIGovernedResult
}

func (s *Server) executeGovernedToolCalls(ctx context.Context, profile *profileRuntime, calls []openAIToolCall) ([]governedExecution, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	out := make([]governedExecution, 0, len(calls))
	for _, call := range calls {
		content, err := s.executeGovernedToolCall(ctx, profile, call)
		if err != nil {
			return nil, err
		}
		payload := map[string]any{}
		if err := json.Unmarshal([]byte(content), &payload); err != nil {
			return nil, types.ErrInternal("failed to decode governed tool result")
		}
		out = append(out, governedExecution{Call: call, Result: governedExecutionResult(call, payload)})
	}
	return out, nil
}

func (s *Server) executeGovernedToolCall(ctx context.Context, profile *profileRuntime, call openAIToolCall) (string, error) {
	if !isGovernedToolCall(profile, call) {
		return "", types.ErrBadRequest("unexpected tool call name from upstream model")
	}
	var args governedActionArgs
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return "", types.ErrBadRequest("invalid governed_action arguments")
	}
	operation := strings.TrimSpace(strings.ToLower(args.Operation))
	if operation == "" {
		return "", types.ErrBadRequest("governed_action requires operation")
	}
	parts := strings.SplitN(operation, ".", 2)
	if len(parts) != 2 {
		return "", types.ErrBadRequest("operation must use tool.action form")
	}
	params := args.Params
	if params == nil {
		params = map[string]any{}
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return "", types.ErrBadRequest("invalid governed_action params")
	}
	resp, submitted, err := s.submitToolCall(ctx, profile, types.ToolCallRequest{
		Tool:   parts[0],
		Action: parts[1],
		Params: paramsJSON,
	})
	if err != nil {
		return "", upstreamAPIError(err, "governed tool call failed")
	}
	s.logger.Info("bridge openai tool call",
		"profile", profile.name,
		"tool", submitted.Tool,
		"action", submitted.Action,
		"event_id", resp.EventID,
		"decision", resp.Decision,
		"session_id", submitted.SessionID,
		"trace_id", submitted.TraceID,
	)
	payload := map[string]any{
		"event_id":     resp.EventID,
		"decision":     resp.Decision,
		"reason":       resp.Reason,
		"approval_url": resp.ApprovalURL,
		"tool":         submitted.Tool,
		"action":       submitted.Action,
		"session_id":   submitted.SessionID,
		"trace_id":     submitted.TraceID,
	}
	if resp.Result != nil {
		payload["result"] = resp.Result.OutputJSON
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", types.ErrInternal("failed to encode governed tool result")
	}
	return string(encoded), nil
}

func governedExecutionResult(call openAIToolCall, payload map[string]any) openAIGovernedResult {
	result := openAIGovernedResult{
		ToolCallID:  strings.TrimSpace(call.ID),
		EventID:     stringValue(payload, "event_id"),
		Decision:    stringValue(payload, "decision"),
		Reason:      stringValue(payload, "reason"),
		ApprovalURL: stringValue(payload, "approval_url"),
		Tool:        stringValue(payload, "tool"),
		Action:      stringValue(payload, "action"),
		SessionID:   stringValue(payload, "session_id"),
		TraceID:     stringValue(payload, "trace_id"),
	}
	if payload != nil {
		if value, ok := payload["result"]; ok {
			result.Result = value
		}
	}
	return result
}

func governedExecutionResults(executions []governedExecution) []openAIGovernedResult {
	if len(executions) == 0 {
		return nil
	}
	results := make([]openAIGovernedResult, 0, len(executions))
	for _, execution := range executions {
		results = append(results, execution.Result)
	}
	return results
}

func stringValue(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func (s *Server) postOpenAIChat(ctx context.Context, profile *profileRuntime, req openAIChatRequest) (*openAIChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, types.ErrBadRequest("failed to encode OpenAI chat request")
	}
	resp, err := s.proxyOpenAIRequest(ctx, profile, http.MethodPost, profile.cfg.OpenAI.UpstreamBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, types.ErrInternal("failed to read OpenAI chat response")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &types.APIError{
			Code:      "UPSTREAM_OPENAI_ERROR",
			Message:   strings.TrimSpace(string(data)),
			Retryable: resp.StatusCode >= 500,
			HTTPCode:  http.StatusBadGateway,
		}
	}
	var out openAIChatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, types.ErrInternal("failed to decode OpenAI chat response")
	}
	return &out, nil
}

func (s *Server) postOpenAIChatStream(ctx context.Context, profile *profileRuntime, req openAIChatRequest) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, types.ErrBadRequest("failed to encode OpenAI chat request")
	}
	resp, err := s.proxyOpenAIRequest(ctx, profile, http.MethodPost, profile.cfg.OpenAI.UpstreamBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if readErr != nil {
			return nil, types.ErrInternal("failed to read OpenAI chat response")
		}
		return nil, &types.APIError{
			Code:      "UPSTREAM_OPENAI_ERROR",
			Message:   strings.TrimSpace(string(data)),
			Retryable: resp.StatusCode >= 500,
			HTTPCode:  http.StatusBadGateway,
		}
	}
	return resp, nil
}

func (s *Server) proxyOpenAIRequest(ctx context.Context, profile *profileRuntime, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, types.ErrInternal("failed to build OpenAI upstream request")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(profile.cfg.OpenAI.UpstreamAPIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(profile.cfg.OpenAI.UpstreamAPIKey))
	}
	resp, err := s.openAIHTTPClient.Do(req)
	if err != nil {
		return nil, &types.APIError{
			Code:      "UPSTREAM_OPENAI_ERROR",
			Message:   fmt.Sprintf("openai upstream request failed: %v", err),
			Retryable: true,
			HTTPCode:  http.StatusBadGateway,
		}
	}
	return resp, nil
}

func copyUpstreamJSON(w http.ResponseWriter, resp *http.Response) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		writeAPIError(w, types.ErrInternal("failed to read upstream response"))
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeAPIError(w, &types.APIError{
			Code:      "UPSTREAM_OPENAI_ERROR",
			Message:   strings.TrimSpace(string(body)),
			Retryable: resp.StatusCode >= 500,
			HTTPCode:  http.StatusBadGateway,
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func decodeOpenAIJSON(body io.ReadCloser, out any) error {
	defer body.Close()
	decoder := json.NewDecoder(io.LimitReader(body, 1<<20))
	return decoder.Decode(out)
}

type openAIChatChunk struct {
	ID      string              `json:"id,omitempty"`
	Object  string              `json:"object"`
	Created int64               `json:"created,omitempty"`
	Model   string              `json:"model,omitempty"`
	Choices []openAIChunkChoice `json:"choices"`
}

type openAIChunkChoice struct {
	Index        int              `json:"index"`
	Delta        openAIChunkDelta `json:"delta"`
	FinishReason string           `json:"finish_reason,omitempty"`
	LogProbs     any              `json:"logprobs,omitempty"`
}

type openAIChunkDelta struct {
	Role       string                `json:"role,omitempty"`
	Content    string                `json:"content,omitempty"`
	ToolCalls  []openAIToolCallDelta `json:"tool_calls,omitempty"`
	OpenClause *openAIExtension      `json:"openclause,omitempty"`
}

type openAIToolCallDelta struct {
	Index    int                     `json:"index,omitempty"`
	ID       string                  `json:"id,omitempty"`
	Type     string                  `json:"type,omitempty"`
	Function openAIFunctionCallDelta `json:"function,omitempty"`
}

type openAIFunctionCallDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

func writeOpenAIChatStream(w http.ResponseWriter, resp *openAIChatResponse) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAPIError(w, types.ErrInternal("streaming is not supported by this HTTP server"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	message := openAIChatMessage{}
	if resp != nil && len(resp.Choices) > 0 {
		message = resp.Choices[0].Message
	}
	content := assistantMessageText(message.Content)

	writeStreamChunk := func(chunk openAIChatChunk) {
		data, err := json.Marshal(chunk)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	writeStreamChunk(openAIChatChunk{
		ID:      resp.ID,
		Object:  "chat.completion.chunk",
		Created: resp.Created,
		Model:   resp.Model,
		Choices: []openAIChunkChoice{{
			Index: 0,
			Delta: openAIChunkDelta{Role: "assistant"},
		}},
	})

	for _, piece := range splitStreamContent(content, 80) {
		writeStreamChunk(openAIChatChunk{
			ID:      resp.ID,
			Object:  "chat.completion.chunk",
			Created: resp.Created,
			Model:   resp.Model,
			Choices: []openAIChunkChoice{{
				Index: 0,
				Delta: openAIChunkDelta{Content: piece},
			}},
		})
	}
	if resp.OpenClause != nil && len(resp.OpenClause.GovernedResults) > 0 {
		writeStreamChunk(openAIChatChunk{
			ID:      resp.ID,
			Object:  "chat.completion.chunk",
			Created: resp.Created,
			Model:   resp.Model,
			Choices: []openAIChunkChoice{{
				Index: 0,
				Delta: openAIChunkDelta{OpenClause: resp.OpenClause},
			}},
		})
	}

	writeStreamChunk(openAIChatChunk{
		ID:      resp.ID,
		Object:  "chat.completion.chunk",
		Created: resp.Created,
		Model:   resp.Model,
		Choices: []openAIChunkChoice{{
			Index:        0,
			Delta:        openAIChunkDelta{},
			FinishReason: streamFinishReason(message),
		}},
	})
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func writeOpenAIStreamChunk(w http.ResponseWriter, flusher http.Flusher, chunk openAIChatChunk) {
	data, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

func writeOpenAIStreamTerminalAssistantMessage(w http.ResponseWriter, flusher http.Flusher, id string, created int64, model, text string) {
	writeOpenAIStreamAssistantContent(w, flusher, id, created, model, text)
	writeOpenAIStreamFinish(w, flusher, id, created, model, "stop")
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func writeOpenAIStreamAssistantContent(w http.ResponseWriter, flusher http.Flusher, id string, created int64, model, text string) {
	for _, piece := range splitStreamContent(text, 80) {
		writeOpenAIStreamChunk(w, flusher, openAIChatChunk{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   model,
			Choices: []openAIChunkChoice{{
				Index: 0,
				Delta: openAIChunkDelta{Content: piece},
			}},
		})
	}
}

func writeOpenAIStreamGovernedResults(w http.ResponseWriter, flusher http.Flusher, id string, created int64, model string, results []openAIGovernedResult) {
	if len(results) == 0 {
		return
	}
	writeOpenAIStreamChunk(w, flusher, openAIChatChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []openAIChunkChoice{{
			Index: 0,
			Delta: openAIChunkDelta{OpenClause: openAIExtensionForResults(results)},
		}},
	})
}

func openAIExtensionForResults(results []openAIGovernedResult) *openAIExtension {
	if len(results) == 0 {
		return nil
	}
	return &openAIExtension{GovernedResults: results}
}

func writeOpenAIStreamToolCalls(w http.ResponseWriter, flusher http.Flusher, id string, created int64, model string, calls []openAIToolCall) {
	for index, call := range calls {
		writeOpenAIStreamChunk(w, flusher, openAIChatChunk{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   model,
			Choices: []openAIChunkChoice{{
				Index: 0,
				Delta: openAIChunkDelta{ToolCalls: []openAIToolCallDelta{{
					Index: index,
					ID:    call.ID,
					Type:  call.Type,
					Function: openAIFunctionCallDelta{
						Name:      call.Function.Name,
						Arguments: call.Function.Arguments,
					},
				}}},
			}},
		})
	}
}

func writeOpenAIStreamFinish(w http.ResponseWriter, flusher http.Flusher, id string, created int64, model, finishReason string) {
	writeOpenAIStreamChunk(w, flusher, openAIChatChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []openAIChunkChoice{{
			Index:        0,
			Delta:        openAIChunkDelta{},
			FinishReason: finishReason,
		}},
	})
}

func assistantMessageText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if kind, _ := block["type"].(string); kind != "text" {
				continue
			}
			if text, _ := block["text"].(string); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func splitStreamContent(text string, chunkSize int) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	runes := []rune(text)
	if chunkSize <= 0 || len(runes) <= chunkSize {
		return []string{text}
	}
	chunks := make([]string, 0, (len(runes)/chunkSize)+1)
	for start := 0; start < len(runes); start += chunkSize {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func streamFinishReason(message openAIChatMessage) string {
	if len(message.ToolCalls) > 0 {
		return "tool_calls"
	}
	return "stop"
}
