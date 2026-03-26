package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	sdkclient "github.com/bturcanu/OpenClause/pkg/sdk/client"
	"github.com/bturcanu/OpenClause/pkg/types"
	"github.com/google/uuid"
)

const bridgeProfileHeader = "X-OpenClause-Profile"

type profileRuntime struct {
	name   string
	cfg    *ResolvedProfile
	client *sdkclient.Client
}

type Server struct {
	cfg              *ResolvedConfig
	defaultProfile   *profileRuntime
	profiles         map[string]*profileRuntime
	httpClient       *http.Client
	openAIHTTPClient *http.Client
	logger           *slog.Logger

	mcpMu       sync.RWMutex
	mcpSessions map[string]string
}

func NewServer(cfg *ResolvedConfig, logger *slog.Logger) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("bridge config required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	profiles := make(map[string]*profileRuntime, len(cfg.Profiles))
	for name, item := range cfg.Profiles {
		profiles[name] = &profileRuntime{
			name:   name,
			cfg:    item,
			client: sdkclient.New(item.BaseURL, item.APIKey),
		}
	}
	defaultProfile, ok := profiles[cfg.DefaultProfile]
	if !ok {
		return nil, fmt.Errorf("bridge default profile %q not found", cfg.DefaultProfile)
	}

	return &Server{
		cfg:              cfg,
		defaultProfile:   defaultProfile,
		profiles:         profiles,
		httpClient:       &http.Client{Timeout: 15 * time.Second},
		openAIHTTPClient: &http.Client{Timeout: 2 * time.Minute},
		logger:           logger,
		mcpSessions:      map[string]string{},
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /v1/bridge/tools", s.handleTools)
	mux.HandleFunc("GET /v1/bridge/profiles", s.handleProfiles)
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("GET /mcp", s.handleMCPHTTP)
	mux.HandleFunc("POST /mcp", s.handleMCPHTTP)
	mux.HandleFunc("DELETE /mcp", s.handleMCPHTTP)
	mux.HandleFunc("POST /v1/toolcalls", s.handleSubmit)
	mux.HandleFunc("GET /v1/toolcalls/{event_id}", s.handleGetEvent)
	mux.HandleFunc("POST /v1/toolcalls/{event_id}/execute", s.handleExecute)
	return mux
}

type toolsResponse struct {
	Profile           string       `json:"profile,omitempty"`
	DefaultProfile    string       `json:"default_profile,omitempty"`
	AvailableProfiles []string     `json:"available_profiles,omitempty"`
	TenantID          string       `json:"tenant_id"`
	AgentID           string       `json:"agent_id"`
	Tools             []ToolConfig `json:"tools"`
}

type bridgeProfileSummary struct {
	Name          string `json:"name"`
	TenantID      string `json:"tenant_id"`
	AgentID       string `json:"agent_id"`
	Default       bool   `json:"default"`
	ToolCount     int    `json:"tool_count"`
	OpenAIEnabled bool   `json:"openai_enabled"`
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (s *Server) handleProfiles(w http.ResponseWriter, _ *http.Request) {
	profiles := make([]bridgeProfileSummary, 0, len(s.profiles))
	for _, name := range s.profileNames() {
		item := s.profiles[name]
		profiles = append(profiles, bridgeProfileSummary{
			Name:          item.name,
			TenantID:      item.cfg.TenantID,
			AgentID:       item.cfg.AgentID,
			Default:       item.name == s.cfg.DefaultProfile,
			ToolCount:     len(item.cfg.Tools),
			OpenAIEnabled: item.cfg.OpenAI.Enabled,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"default_profile": s.cfg.DefaultProfile,
		"profiles":        profiles,
	})
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	profile, err := s.profileForRequest(r)
	if err != nil {
		writeBridgeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toolsResponse{
		Profile:           profile.name,
		DefaultProfile:    s.cfg.DefaultProfile,
		AvailableProfiles: s.profileNames(),
		TenantID:          profile.cfg.TenantID,
		AgentID:           profile.cfg.AgentID,
		Tools:             profile.cfg.Tools,
	})
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	profile, err := s.profileForRequest(r)
	if err != nil {
		writeBridgeError(w, err)
		return
	}
	var req types.ToolCallRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeAPIError(w, types.ErrBadRequest("invalid tool call request JSON"))
		return
	}
	resp, submitted, err := s.submitToolCall(r.Context(), profile, req)
	if err != nil {
		writeBridgeError(w, upstreamAPIError(err, "bridge submit failed"))
		return
	}
	s.logger.Info("bridge tool submit",
		"profile", profile.name,
		"tenant_id", submitted.TenantID,
		"agent_id", submitted.AgentID,
		"tool", submitted.Tool,
		"action", submitted.Action,
		"session_id", submitted.SessionID,
		"trace_id", submitted.TraceID,
		"event_id", resp.EventID,
		"decision", resp.Decision,
	)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	profile, err := s.profileForRequest(r)
	if err != nil {
		writeBridgeError(w, err)
		return
	}
	eventID := strings.TrimSpace(r.PathValue("event_id"))
	if eventID == "" {
		writeAPIError(w, types.ErrBadRequest("event_id required"))
		return
	}
	resp, err := profile.client.Execute(r.Context(), eventID)
	if err != nil {
		writeBridgeError(w, upstreamAPIError(err, "bridge execute failed"))
		return
	}
	s.logger.Info("bridge tool execute", "profile", profile.name, "event_id", resp.EventID, "decision", resp.Decision)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	profile, err := s.profileForRequest(r)
	if err != nil {
		writeBridgeError(w, err)
		return
	}
	eventID := strings.TrimSpace(r.PathValue("event_id"))
	if eventID == "" {
		writeAPIError(w, types.ErrBadRequest("event_id required"))
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, profile.cfg.BaseURL+"/v1/toolcalls/"+eventID, http.NoBody)
	if err != nil {
		writeAPIError(w, types.ErrInternal("failed to build upstream event request"))
		return
	}
	req.Header.Set("X-API-Key", profile.cfg.APIKey)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		writeAPIError(w, &types.APIError{
			Code:      "UPSTREAM_ERROR",
			Message:   fmt.Sprintf("bridge get event failed: %v", err),
			Retryable: true,
			HTTPCode:  http.StatusBadGateway,
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		writeAPIError(w, types.ErrInternal("failed to read upstream event response"))
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr types.APIError
		if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Message != "" {
			apiErr.HTTPCode = resp.StatusCode
			writeAPIError(w, &apiErr)
			return
		}
		writeAPIError(w, &types.APIError{
			Code:      "UPSTREAM_ERROR",
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

func (s *Server) profileNames() []string {
	names := make([]string, 0, len(s.profiles))
	for name := range s.profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *Server) profileForRequest(r *http.Request) (*profileRuntime, error) {
	if s == nil || s.defaultProfile == nil {
		return nil, types.ErrInternal("bridge profile runtime not configured")
	}
	profileName := strings.TrimSpace(r.Header.Get(bridgeProfileHeader))
	if profileName == "" {
		profileName = strings.TrimSpace(r.URL.Query().Get("profile"))
	}
	if profileName == "" {
		return s.defaultProfile, nil
	}
	profile, ok := s.profiles[profileName]
	if !ok {
		return nil, types.ErrBadRequest(fmt.Sprintf("unknown bridge profile %q", profileName))
	}
	return profile, nil
}

func (s *Server) profileForName(profileName string) (*profileRuntime, error) {
	if strings.TrimSpace(profileName) == "" {
		return s.defaultProfile, nil
	}
	profile, ok := s.profiles[strings.TrimSpace(profileName)]
	if !ok {
		return nil, types.ErrBadRequest(fmt.Sprintf("unknown bridge profile %q", profileName))
	}
	return profile, nil
}

func (s *Server) applyDefaults(profile *profileRuntime, req *types.ToolCallRequest) error {
	if req == nil {
		return types.ErrBadRequest("tool call request required")
	}
	if profile == nil || profile.cfg == nil {
		return types.ErrInternal("bridge profile required")
	}
	if err := enforceIdentity("tenant_id", profile.cfg.TenantID, &req.TenantID); err != nil {
		return err
	}
	if err := enforceIdentity("agent_id", profile.cfg.AgentID, &req.AgentID); err != nil {
		return err
	}

	tool, ok := profile.cfg.LookupTool(req.Tool, req.Action)
	if len(profile.cfg.Tools) > 0 && !ok {
		return types.ErrForbidden(fmt.Sprintf("tool %s:%s is not configured for bridge profile %s", strings.TrimSpace(req.Tool), strings.TrimSpace(req.Action), profile.name))
	}
	if ok {
		if profile.cfg.Defaults.RiskMode == "configured" || req.RiskScore == 0 {
			req.RiskScore = tool.RiskScore
		}
		if req.Resource == "" && strings.TrimSpace(tool.Resource) != "" {
			req.Resource = tool.Resource
		}
		if len(req.RiskFactors) == 0 && len(tool.RiskFactors) > 0 {
			req.RiskFactors = append([]string(nil), tool.RiskFactors...)
		}
	}

	if req.UserID == "" && strings.TrimSpace(profile.cfg.Defaults.UserID) != "" {
		req.UserID = profile.cfg.Defaults.UserID
	}
	if req.SessionID == "" {
		req.SessionID = generateSessionID(profile.cfg.Defaults.SessionPrefix)
	}
	if req.TraceID == "" {
		req.TraceID = uuid.NewString()
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = uuid.NewString()
	}
	req.Tool = strings.ToLower(strings.TrimSpace(req.Tool))
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	return nil
}

func (s *Server) submitToolCall(ctx context.Context, profile *profileRuntime, req types.ToolCallRequest) (*types.ToolCallResponse, types.ToolCallRequest, error) {
	if err := s.applyDefaults(profile, &req); err != nil {
		return nil, types.ToolCallRequest{}, err
	}
	if err := req.NormalizeAndValidate(); err != nil {
		return nil, types.ToolCallRequest{}, types.ErrValidation(err)
	}
	resp, err := profile.client.Submit(ctx, req)
	if err != nil {
		return nil, req, err
	}
	return resp, req, nil
}

func enforceIdentity(field string, want string, got *string) error {
	if got == nil {
		return types.ErrBadRequest(field + " required")
	}
	if strings.TrimSpace(*got) == "" {
		*got = want
		return nil
	}
	if strings.TrimSpace(*got) != want {
		return types.ErrBadRequest(fmt.Sprintf("%s must match bridge config", field))
	}
	return nil
}

func generateSessionID(prefix string) string {
	if strings.TrimSpace(prefix) == "" {
		prefix = "agent"
	}
	return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UTC().Unix(), uuid.NewString()[:8])
}

func decodeJSON(body io.ReadCloser, out any) error {
	defer body.Close()
	decoder := json.NewDecoder(io.LimitReader(body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeBridgeError(w http.ResponseWriter, err error) {
	if err == nil {
		writeAPIError(w, types.ErrInternal("bridge error"))
		return
	}
	var apiErr *types.APIError
	if errors.As(err, &apiErr) {
		writeAPIError(w, apiErr)
		return
	}
	writeAPIError(w, types.ErrInternal(err.Error()))
}

func writeAPIError(w http.ResponseWriter, err *types.APIError) {
	if err == nil {
		err = types.ErrInternal("bridge error")
	}
	err.WriteJSON(w)
}

func upstreamAPIError(err error, fallback string) error {
	if err == nil {
		return nil
	}
	var apiErr *types.APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return &types.APIError{
		Code:      "UPSTREAM_ERROR",
		Message:   fmt.Sprintf("%s: %v", fallback, err),
		Retryable: true,
		HTTPCode:  http.StatusBadGateway,
	}
}

func Serve(ctx context.Context, cfg *ResolvedConfig, logger *slog.Logger) error {
	server, err := NewServer(cfg, logger)
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr:    cfg.Listen,
		Handler: server.Handler(),
	}

	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}
