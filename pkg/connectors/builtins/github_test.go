package builtins

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bturcanu/OpenClause/pkg/connectors"
)

func TestGithubIssueCommentUsesRequestedIssueNumber(t *testing.T) {
	connector := &GithubConnector{}
	params, err := json.Marshal(map[string]any{
		"owner":        "acme",
		"repo":         "web-app",
		"issue_number": 42,
		"body":         "thanks",
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	resp := connector.Exec(context.Background(), connectors.ExecRequest{
		Action: "issue.comment",
		Params: params,
	})
	if resp.Status != "success" {
		t.Fatalf("expected success, got %#v", resp)
	}

	var out map[string]any
	if err := json.Unmarshal(resp.OutputJSON, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out["html_url"] != "https://github.com/acme/web-app/issues/42#issuecomment-9001" {
		t.Fatalf("unexpected html_url: %v", out["html_url"])
	}
}

func TestGithubIssueCommentFailsClosedWhenRequiredFieldsAreMissing(t *testing.T) {
	connector := &GithubConnector{}
	resp := connector.Exec(context.Background(), connectors.ExecRequest{
		Action: "issue.comment",
		Params: []byte(`{"owner":"acme","repo":"web-app","body":"thanks"}`),
	})
	if resp.Status != "error" {
		t.Fatalf("expected error for missing issue_number, got %#v", resp)
	}
	if resp.Error != "issue_number must be greater than zero" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
}

func TestGithubIssueCreateFailsClosedWhenRequiredFieldsAreMissing(t *testing.T) {
	connector := &GithubConnector{}
	resp := connector.Exec(context.Background(), connectors.ExecRequest{
		Action: "issue.create",
		Params: []byte(`{"title":"Missing repo context"}`),
	})
	if resp.Status != "error" {
		t.Fatalf("expected error for missing owner/repo, got %#v", resp)
	}
	if resp.Error != "owner is required" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
}
