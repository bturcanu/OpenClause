# OpenClause TypeScript SDK

Official TypeScript/Node.js SDK for the OpenClause access governance platform.

## Requirements

- Node.js 18+ (uses native `fetch` and `crypto.randomUUID`)

## Installation

```bash
npm install openclause
```

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
