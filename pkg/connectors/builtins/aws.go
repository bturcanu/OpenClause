package builtins

import (
	"context"
	"encoding/json"

	"github.com/bturcanu/OpenClause/pkg/connectors"
)

// AWSConnector provides mock AWS API actions.
type AWSConnector struct{}

func (c *AWSConnector) Name() string { return "aws" }

func (c *AWSConnector) Actions() []string {
	return []string{"s3.list_buckets", "s3.get_object", "iam.list_users", "iam.get_role"}
}

func (c *AWSConnector) Exec(_ context.Context, req connectors.ExecRequest) connectors.ExecResponse {
	switch req.Action {
	case "s3.list_buckets":
		return c.s3ListBuckets()
	case "s3.get_object":
		return c.s3GetObject(req)
	case "iam.list_users":
		return c.iamListUsers()
	case "iam.get_role":
		return c.iamGetRole(req)
	default:
		return connectors.ExecResponse{Status: "error", Error: "unsupported action: " + req.Action}
	}
}

func (c *AWSConnector) s3ListBuckets() connectors.ExecResponse {
	out, _ := json.Marshal(map[string]any{
		"buckets": []map[string]any{
			{"name": "acme-prod-data", "creation_date": "2024-06-01T00:00:00Z", "region": "us-east-1"},
			{"name": "acme-logs", "creation_date": "2024-03-15T00:00:00Z", "region": "us-east-1"},
			{"name": "acme-backups", "creation_date": "2025-01-10T00:00:00Z", "region": "eu-west-1"},
		},
		"owner": map[string]string{"display_name": "acme-root", "id": "abc123"},
		"mock":  true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}

func (c *AWSConnector) s3GetObject(req connectors.ExecRequest) connectors.ExecResponse {
	var p struct {
		Bucket string `json:"bucket"`
		Key    string `json:"key"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return connectors.ExecResponse{Status: "error", Error: "invalid params: " + err.Error()}
	}
	out, _ := json.Marshal(map[string]any{
		"bucket":         p.Bucket,
		"key":            p.Key,
		"content_type":   "application/json",
		"content_length": 1024,
		"last_modified":  "2026-01-10T08:00:00Z",
		"etag":           "\"d41d8cd98f00b204e9800998ecf8427e\"",
		"body_preview":   "{\"sample\": \"data\"}",
		"mock":           true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}

func (c *AWSConnector) iamListUsers() connectors.ExecResponse {
	out, _ := json.Marshal(map[string]any{
		"users": []map[string]any{
			{"user_name": "alice", "user_id": "AIDA000000000EXAMPLE1", "arn": "arn:aws:iam::123456789012:user/alice", "create_date": "2024-01-01T00:00:00Z"},
			{"user_name": "bob", "user_id": "AIDA000000000EXAMPLE2", "arn": "arn:aws:iam::123456789012:user/bob", "create_date": "2024-06-15T00:00:00Z"},
			{"user_name": "deploy-bot", "user_id": "AIDA000000000EXAMPLE3", "arn": "arn:aws:iam::123456789012:user/deploy-bot", "create_date": "2025-03-01T00:00:00Z"},
		},
		"is_truncated": false,
		"mock":         true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}

func (c *AWSConnector) iamGetRole(req connectors.ExecRequest) connectors.ExecResponse {
	var p struct {
		RoleName string `json:"role_name"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return connectors.ExecResponse{Status: "error", Error: "invalid params: " + err.Error()}
	}
	out, _ := json.Marshal(map[string]any{
		"role": map[string]any{
			"role_name":   p.RoleName,
			"role_id":     "AROA000000000EXAMPLE",
			"arn":         "arn:aws:iam::123456789012:role/" + p.RoleName,
			"path":        "/",
			"create_date": "2025-01-01T00:00:00Z",
			"assume_role_policy_document": map[string]any{
				"Version": "2012-10-17",
				"Statement": []map[string]any{
					{
						"Effect":    "Allow",
						"Principal": map[string]string{"Service": "ec2.amazonaws.com"},
						"Action":    "sts:AssumeRole",
					},
				},
			},
		},
		"mock": true,
	})
	return connectors.ExecResponse{Status: "success", OutputJSON: out}
}
