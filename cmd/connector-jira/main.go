// Connector-Jira provides Jira integrations (issue.create) for the gateway.
package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bturcanu/OpenClause/pkg/config"
	"github.com/bturcanu/OpenClause/pkg/connectors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const maxBodyBytes = 1 << 20
const maxExternalResponseBytes = 4 << 20

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	mock := strings.ToLower(os.Getenv("MOCK_CONNECTORS")) == "true"
	baseURL := os.Getenv("JIRA_BASE_URL")
	email := os.Getenv("JIRA_EMAIL")
	apiToken := os.Getenv("JIRA_API_TOKEN")

	if !mock && (baseURL == "" || email == "" || apiToken == "") {
		log.Error("JIRA_BASE_URL, JIRA_EMAIL, and JIRA_API_TOKEN are required when MOCK_CONNECTORS is not true")
		os.Exit(1)
	}

	connector := &JiraConnector{
		log:      log,
		mock:     mock,
		baseURL:  baseURL,
		email:    email,
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}

	internalToken := os.Getenv("INTERNAL_AUTH_TOKEN")
	if internalToken == "" {
		log.Error("INTERNAL_AUTH_TOKEN is required")
		os.Exit(1)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(15 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	r.Post("/exec", func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Internal-Token")), []byte(internalToken)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		var req connectors.ExecRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}

		resp := connector.Exec(r.Context(), req)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Error("response encode failed", "error", err)
		}
	})

	addr := config.EnvOr("CONNECTOR_JIRA_ADDR", ":8083")
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info("connector-jira starting", "addr", addr, "mock", mock)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down connector-jira")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error("shutdown error", "error", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Jira connector implementation
// ──────────────────────────────────────────────────────────────────────────────

type JiraConnector struct {
	log        *slog.Logger
	mock       bool
	baseURL    string
	email      string
	apiToken   string
	httpClient *http.Client
}

type jiraIssueParams struct {
	Project     string `json:"project"`
	Summary     string `json:"summary"`
	Description string `json:"description,omitempty"`
	IssueType   string `json:"issue_type"`
}

func (j *JiraConnector) Exec(ctx context.Context, req connectors.ExecRequest) connectors.ExecResponse {
	action := req.Tool + "." + req.Action
	switch action {
	case "jira.issue.create":
		return j.createIssue(ctx, req)
	case "jira.issue.list":
		return j.listIssues(ctx, req)
	default:
		return connectors.ExecResponse{
			Status: "error",
			Error:  fmt.Sprintf("unsupported action: %s", action),
		}
	}
}

func (j *JiraConnector) listIssues(ctx context.Context, req connectors.ExecRequest) connectors.ExecResponse {
	if j.mock {
		output, _ := json.Marshal(map[string]any{
			"issues": []map[string]any{
				{"id": "10001", "key": "OPS-1", "summary": "Mock issue 1"},
				{"id": "10002", "key": "OPS-2", "summary": "Mock issue 2"},
			},
			"total": 2,
			"mock":  true,
		})
		return connectors.ExecResponse{Status: "success", OutputJSON: output}
	}
	url := strings.TrimRight(j.baseURL, "/") + "/rest/api/3/search?maxResults=20"
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return connectors.ExecResponse{Status: "error", Error: err.Error()}
	}
	httpReq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString(
		[]byte(j.email+":"+j.apiToken)))
	resp, err := j.httpClient.Do(httpReq)
	if err != nil {
		return connectors.ExecResponse{Status: "error", Error: err.Error()}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxExternalResponseBytes))
	if err != nil {
		return connectors.ExecResponse{Status: "error", Error: "read response: " + err.Error()}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return connectors.ExecResponse{Status: "error", Error: string(respBody)}
	}
	return connectors.ExecResponse{Status: "success", OutputJSON: respBody}
}

func (j *JiraConnector) createIssue(ctx context.Context, req connectors.ExecRequest) connectors.ExecResponse {
	var params jiraIssueParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return connectors.ExecResponse{Status: "error", Error: "invalid params: " + err.Error()}
	}

	if params.Project == "" || params.Summary == "" {
		return connectors.ExecResponse{Status: "error", Error: "project and summary are required"}
	}
	if params.IssueType == "" {
		params.IssueType = "Task"
	}

	if j.mock {
		j.log.Info("mock jira.issue.create", "project", params.Project, "summary", params.Summary)
		output, _ := json.Marshal(map[string]any{
			"id":   "10001",
			"key":  params.Project + "-42",
			"self": "https://mock.atlassian.net/rest/api/3/issue/10001",
			"mock": true,
		})
		return connectors.ExecResponse{Status: "success", OutputJSON: output}
	}

	issueBody := map[string]any{
		"fields": map[string]any{
			"project":   map[string]string{"key": params.Project},
			"summary":   params.Summary,
			"issuetype": map[string]string{"name": params.IssueType},
		},
	}
	if params.Description != "" {
		fields := issueBody["fields"].(map[string]any)
		fields["description"] = map[string]any{
			"type":    "doc",
			"version": 1,
			"content": []any{
				map[string]any{
					"type": "paragraph",
					"content": []any{
						map[string]any{
							"type": "text",
							"text": params.Description,
						},
					},
				},
			},
		}
	}

	body, _ := json.Marshal(issueBody)
	url := strings.TrimRight(j.baseURL, "/") + "/rest/api/3/issue"

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return connectors.ExecResponse{Status: "error", Error: err.Error()}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString(
		[]byte(j.email+":"+j.apiToken)))

	resp, err := j.httpClient.Do(httpReq)
	if err != nil {
		return connectors.ExecResponse{Status: "error", Error: err.Error()}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxExternalResponseBytes))
	if err != nil {
		return connectors.ExecResponse{Status: "error", Error: "read response: " + err.Error()}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return connectors.ExecResponse{Status: "error", Error: string(respBody)}
	}

	return connectors.ExecResponse{Status: "success", OutputJSON: respBody}
}
