# OpenClause TypeScript SDK

Official TypeScript/Node.js SDK for the OpenClause access governance platform.

## Requirements

- Node.js 18+ (uses native `fetch` and `crypto.randomUUID`)

## Installation

```bash
npm install openclause
```

## Scope

This SDK wraps the gateway tool-call APIs only. Console-admin flows such as invite delivery, session exports, and evidence bundle exports live on `console-api` and are documented in the repo [README](../../readme.md) and [Local Testing Guide](../../docs/LOCAL_TESTING.md).

Current console contracts worth knowing:
- Session exports return `404` when the session is missing or outside the caller's tenant scope.
- Evidence bundle export honors `since` / `until` and returns `400` when the requested window exceeds 10,000 events.
- `POST /admin/invites` returns the raw invite token once plus `accept_url` and `email_status`; later invite-list responses omit the raw token.

## Quick Start

```typescript
import { OpenClauseClient, ToolCallRequest } from "openclause";

const client = new OpenClauseClient({
  baseUrl: "http://localhost:8080",
  apiKey: process.env.OPENCLAUSE_API_KEY!,
});

const request: ToolCallRequest = {
  tenant_id: "<tenant_id>",
  agent_id: "<agent_id>",
  tool: "slack",
  action: "msg.post",
  idempotency_key: OpenClauseClient.generateIdempotencyKey(),
  params: { channel: "#general", text: "Hello from OpenClause!" },
  risk_score: 3,
};

const response = await client.submitToolCall(request);

switch (response.decision) {
  case "allow":
    console.log("Tool call allowed.");
    console.log("Result:", response.result);
    break;

  case "approve":
    console.log("Approval required:", response.approval_url);
    const approved = await client.waitForApproval(response.event_id, {
      timeoutMs: 120_000,
    });
    console.log("Approval decision:", approved.decision);
    break;

  case "deny":
    console.log("Denied:", response.reason);
    break;
}
```

## Error Handling

```typescript
import { APIError, AuthenticationError, TimeoutError } from "openclause";

try {
  const response = await client.submitToolCall(request);
} catch (err) {
  if (err instanceof AuthenticationError) {
    console.error("Invalid API key");
  } else if (err instanceof TimeoutError) {
    console.error("Request timed out");
  } else if (err instanceof APIError) {
    console.error(`API error ${err.statusCode}: ${err.responseBody}`);
  }
}
```

## API Reference

### `OpenClauseClient`

| Method | Description |
|--------|-------------|
| `submitToolCall(request)` | Submit a tool call for policy evaluation |
| `getEvent(eventId)` | Retrieve the status of a tool call event |
| `execute(eventId)` | Execute a previously approved tool call |
| `waitForApproval(eventId, options?)` | Retry `execute()` until it succeeds (409 "awaiting approval" -> retry) |
| `generateIdempotencyKey()` | Generate a UUID v4 idempotency key (static) |

### Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `baseUrl` | `string` | — | API base URL |
| `apiKey` | `string` | — | API key for authentication |
| `timeout` | `number` | `30000` | HTTP request timeout in milliseconds |
