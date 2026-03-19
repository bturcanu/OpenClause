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
  baseUrl: "https://api.openclause.dev",
  apiKey: process.env.OPENCLAUSE_API_KEY!,
});

const request: ToolCallRequest = {
  tenant_id: "t_acme",
  agent_id: "agent_billing",
  tool: "stripe",
  action: "refund",
  idempotency_key: OpenClauseClient.generateIdempotencyKey(),
  params: { charge_id: "ch_abc123", amount: 5000 },
  resource: "charges/ch_abc123",
  risk_score: 8,
};

const response = await client.submitToolCall(request);

switch (response.decision) {
  case "allow":
    console.log("Tool call allowed, executing...");
    const result = await client.execute(response.event_id);
    console.log("Result:", result);
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
