package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bturcanu/OpenClause/pkg/connectors"
)

// GithubConnector provides mock GitHub API actions.
type GithubConnector struct{}

func (c *GithubConnector) Name() string { return "github" }

func (c *GithubConnector) Actions() []string {
	return []string{"issue.create", "issue.comment", "repo.list", "repo.readme"}
}

func (c *GithubConnector) Exec(_ context.Context, req connectors.ExecRequest) connectors.ExecResponse {
	switch req.Action {
	case "issue.create":
		return c.issueCreate(req)
	case "issue.comment":
		return c.issueComment(req)
	case "repo.list":
		return c.repoList()
	case "repo.readme":
		return c.repoReadme(req)
	default:
		return connectors.ExecResponse{Status: "error", Error: "unsupported action: " + req.Action}
	}
}

func (c *GithubConnector) issueCreate(req connectors.ExecRequest) connectors.ExecResponse {
	var p struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return connectors.ExecResponse{Status: "error", Error: "invalid params: " + err.Error()}
	}
	if strings.TrimSpace(p.Owner) == "" {
		return connectors.ExecResponse{Status: "error", Error: "owner is required"}
	}
	if strings.TrimSpace(p.Repo) == "" {
		return connectors.ExecResponse{Status: "error", Error: "repo is required"}
	}
	if strings.TrimSpace(p.Title) == "" {
		return connectors.ExecResponse{Status: "error", Error: "title is required"}
	}
	out, _ := json.Marshal(map[string]any{
		"id":         42,
		"number":     101,
		"html_url":   "https://github.com/" + p.Owner + "/" + p.Repo + "/issues/101",
		"title":      p.Title,
		"state":      "open",
		"created_at": "2026-01-15T10:30:00Z",
		"mock":       true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}

func (c *GithubConnector) issueComment(req connectors.ExecRequest) connectors.ExecResponse {
	var p struct {
		Owner       string `json:"owner"`
		Repo        string `json:"repo"`
		IssueNumber int    `json:"issue_number"`
		Body        string `json:"body"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return connectors.ExecResponse{Status: "error", Error: "invalid params: " + err.Error()}
	}
	if strings.TrimSpace(p.Owner) == "" {
		return connectors.ExecResponse{Status: "error", Error: "owner is required"}
	}
	if strings.TrimSpace(p.Repo) == "" {
		return connectors.ExecResponse{Status: "error", Error: "repo is required"}
	}
	if p.IssueNumber <= 0 {
		return connectors.ExecResponse{Status: "error", Error: "issue_number must be greater than zero"}
	}
	if strings.TrimSpace(p.Body) == "" {
		return connectors.ExecResponse{Status: "error", Error: "body is required"}
	}
	out, _ := json.Marshal(map[string]any{
		"id":         9001,
		"html_url":   "https://github.com/" + p.Owner + "/" + p.Repo + "/issues/" + fmt.Sprint(p.IssueNumber) + "#issuecomment-9001",
		"body":       p.Body,
		"created_at": "2026-01-15T11:00:00Z",
		"mock":       true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}

func (c *GithubConnector) repoList() connectors.ExecResponse {
	out, _ := json.Marshal(map[string]any{
		"repositories": []map[string]any{
			{"full_name": "acme/web-app", "private": false, "default_branch": "main"},
			{"full_name": "acme/backend-api", "private": true, "default_branch": "main"},
			{"full_name": "acme/infra", "private": true, "default_branch": "production"},
		},
		"total_count": 3,
		"mock":        true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}

func (c *GithubConnector) repoReadme(req connectors.ExecRequest) connectors.ExecResponse {
	var p struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return connectors.ExecResponse{Status: "error", Error: "invalid params: " + err.Error()}
	}
	if strings.TrimSpace(p.Owner) == "" {
		return connectors.ExecResponse{Status: "error", Error: "owner is required"}
	}
	if strings.TrimSpace(p.Repo) == "" {
		return connectors.ExecResponse{Status: "error", Error: "repo is required"}
	}
	out, _ := json.Marshal(map[string]any{
		"name":     "README.md",
		"path":     "README.md",
		"size":     256,
		"content":  "# " + p.Repo + "\n\nThis is a sample README for the repository.",
		"encoding": "utf-8",
		"mock":     true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}
