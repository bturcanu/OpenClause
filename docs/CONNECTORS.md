# OpenClause Connectors

Connectors bridge OpenClause to external tools and services. Every action an AI agent attempts — posting a Slack message, creating a Jira ticket, querying a database — is routed through a connector.

## Architecture

```
Agent → Gateway (policy check) → Connector Registry → Connector → External Service
```

The **Registry** (`pkg/connectors/registry.go`) holds two kinds of connectors:

| Type | Description | Latency | When to use |
|------|-------------|---------|-------------|
| **Remote** | Separate HTTP microservice registered by URL | Network hop | Production integrations with secrets, OAuth, rate limits |
| **Built-in** | In-process Go function registered at startup | Zero network | Mock/dev mode, simple stateless tools |

Remote connectors take precedence: if a tool name is registered both ways, the HTTP route wins.

## Available Connectors

### Remote connectors (separate binaries)

| Tool | Binary | Actions | Env vars |
|------|--------|---------|----------|
| `slack` | `cmd/connector-slack` | `msg.post`, `channel.list`, `approval.request` | `SLACK_BOT_TOKEN` |
| `jira` | `cmd/connector-jira` | `issue.create`, `issue.transition`, `issue.comment`, `issue.get` | `JIRA_BASE_URL`, `JIRA_EMAIL`, `JIRA_API_TOKEN` |

### Built-in connectors (`pkg/connectors/builtins/`)

All built-in connectors ship with deterministic mock data suitable for integration tests and local development.

| Tool | File | Actions |
|------|------|---------|
| `github` | `github.go` | `issue.create`, `issue.comment`, `repo.list`, `repo.readme` |
| `aws` | `aws.go` | `s3.list_buckets`, `s3.get_object`, `iam.list_users`, `iam.get_role` |
| `servicenow` | `servicenow.go` | `incident.create`, `incident.list`, `incident.get` |
| `email` | `email.go` | `send`, `list_inbox` |
| `postgres` | `postgres_ro.go` | `query.readonly` |
| `webhook` | `webhook.go` | `post` |

## Protocol

Every connector — remote or built-in — uses the same request/response types defined in `pkg/connectors/types.go`.

### ExecRequest

```json
{
  "event_id":  "evt_abc123",
  "tenant_id": "tenant_1",
  "agent_id":  "agent_deploy",
  "tool":      "github",
  "action":    "issue.create",
  "params":    { "owner": "acme", "repo": "web-app", "title": "Bug fix" },
  "resource":  "acme/web-app"
}
```

### ExecResponse

```json
{
  "status": "success",
  "output_json": { "number": 101, "html_url": "https://github.com/acme/web-app/issues/101" }
}
```

On error:

```json
{
  "status": "error",
  "error": "rate limited by GitHub API"
}
```

## Building a Remote Connector

A remote connector is a small HTTP service with two endpoints:

- `GET /healthz` — returns 200 OK
- `POST /exec` — accepts `ExecRequest`, returns `ExecResponse`

### 1. Use the SDK

The SDK (`pkg/connectors/sdk`) handles auth, body parsing, and timeouts:

```go
package main

import (
    "context"
    "encoding/json"
    "log/slog"
    "net/http"
    "os"

    "github.com/bturcanu/OpenClause/pkg/connectors"
    "github.com/bturcanu/OpenClause/pkg/connectors/sdk"
)

type myConnector struct{}

func (m myConnector) Exec(ctx context.Context, req connectors.ExecRequest) connectors.ExecResponse {
    switch req.Action {
    case "hello":
        out, _ := json.Marshal(map[string]string{"greeting": "world"})
        return connectors.ExecResponse{Status: "success", OutputJSON: out}
    default:
        return connectors.ExecResponse{Status: "error", Error: "unknown action"}
    }
}

func main() {
    mux := http.NewServeMux()
    mux.HandleFunc("/exec", sdk.Handler(myConnector{}, sdk.Config{
        InternalToken: os.Getenv("INTERNAL_AUTH_TOKEN"),
        Logger:        slog.Default(),
    }))
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(http.StatusOK)
    })
    http.ListenAndServe(":8090", mux)
}
```

### 2. Register with the gateway

Set the environment variable for the gateway:

```bash
CONNECTOR_MY_TOOL_URL=http://localhost:8090
```

Or call `Registry.Register("my_tool", "http://localhost:8090")` in code.

### 3. Mock mode

All connectors should support a `MOCK_CONNECTORS=true` env var for deterministic outputs during testing.

## Building a Built-in Connector

Built-in connectors are Go structs in `pkg/connectors/builtins/`. They implement:

```go
type BuiltinConnector interface {
    Name() string
    Actions() []string
    Exec(ctx context.Context, req connectors.ExecRequest) connectors.ExecResponse
}
```

Add your connector to the `All()` function in `pkg/connectors/builtins/registry.go` so it is discovered automatically.

### Example

```go
package builtins

import (
    "context"
    "encoding/json"
    "github.com/bturcanu/OpenClause/pkg/connectors"
)

type PagerDutyConnector struct{}

func (c *PagerDutyConnector) Name() string    { return "pagerduty" }
func (c *PagerDutyConnector) Actions() []string { return []string{"incident.trigger"} }

func (c *PagerDutyConnector) Exec(_ context.Context, req connectors.ExecRequest) connectors.ExecResponse {
    out, _ := json.Marshal(map[string]any{
        "incident_key": "pd-mock-001",
        "status":       "triggered",
        "mock":         true,
    })
    return connectors.ExecResponse{Status: "success", OutputJSON: out}
}
```

## Discovery API

The registry exposes `ListAll()` which returns every connector (remote + built-in) as `[]ConnectorInfo`:

```go
type ConnectorInfo struct {
    Name    string   `json:"name"`
    BaseURL string   `json:"base_url,omitempty"`
    Actions []string `json:"actions"`
    Type    string   `json:"type"` // "remote" or "builtin"
}
```

This powers the marketplace/catalog UI and agent capability discovery.

## Security Notes

- Remote connectors authenticate via `X-Internal-Token` header (service-to-service).
- The `webhook` built-in validates target URLs against SSRF (HTTPS-only, no private/loopback IPs).
- The `postgres` built-in is read-only by design — it only supports `query.readonly`.
- All connector responses are capped at 4 MB to prevent memory issues.
