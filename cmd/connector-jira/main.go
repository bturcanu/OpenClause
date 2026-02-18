// Connector-Jira provides Jira integrations (issue.create) for the gateway.
package main

import (
	"bytes"
	"context"
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

	"github.com/agenticaccess/governance/pkg/connectors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	mock := strings.ToLower(os.Getenv("MOCK_CONNECTORS")) == "true"

	connector := &JiraConnector{
		log:      log,
		mock:     mock,
		baseURL:  os.Getenv("JIRA_BASE_URL"),
		email:    os.Getenv("JIRA_EMAIL"),
		apiToken: os.Getenv("JIRA_API_TOKEN"),
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
		var req connectors.ExecRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}

		resp := connector.Exec(r.Context(), req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	addr := envOr("CONNECTOR_JIRA_ADDR", ":8083")
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		log.Info("connector-jira starting", "addr", addr, "mock", mock)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down connector-jira")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
}

// ──────────────────────────────────────────────────────────────────────────────
// Jira connector implementation
// ──────────────────────────────────────────────────────────────────────────────

type JiraConnector struct {
	log      *slog.Logger
	mock     bool
	baseURL  string
	email    string
	apiToken string
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
	default:
		return connectors.ExecResponse{
			Status: "error",
			Error:  fmt.Sprintf("unsupported action: %s", action),
		}
	}
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

	// ── Mock mode ────────────────────────────────────────────────────────
	if j.mock {
		j.log.Info("mock jira.issue.create", "project", params.Project, "summary", params.Summary)
		output, _ := json.Marshal(map[string]any{
			"id":   "10001",
			"key":  params.Project + "-42",
			"self": fmt.Sprintf("https://mock.atlassian.net/rest/api/3/issue/10001"),
			"mock": true,
		})
		return connectors.ExecResponse{Status: "success", OutputJSON: output}
	}

	// ── Real Jira API ────────────────────────────────────────────────────
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

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return connectors.ExecResponse{Status: "error", Error: err.Error()}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return connectors.ExecResponse{Status: "error", Error: string(respBody)}
	}

	return connectors.ExecResponse{Status: "success", OutputJSON: respBody}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
