# OpenClause Java SDK

Official Java SDK for the OpenClause access governance platform.

## Requirements

- Java 11+
- Gson

## Installation

### Gradle

```groovy
dependencies {
    implementation 'dev.openclause:openclause-sdk:0.2.0'
}
```

### Maven

```xml
<dependency>
    <groupId>dev.openclause</groupId>
    <artifactId>openclause-sdk</artifactId>
    <version>0.2.0</version>
</dependency>
```

## Quick Start

```java
import dev.openclause.sdk.OpenClauseClient;
import dev.openclause.sdk.models.ToolCallRequest;
import dev.openclause.sdk.models.ToolCallResponse;
import dev.openclause.sdk.exceptions.OpenClauseException;

import java.util.Map;

public class Example {
    public static void main(String[] args) throws OpenClauseException {
        OpenClauseClient client = new OpenClauseClient(
            "https://api.openclause.dev",
            System.getenv("OPENCLAUSE_API_KEY")
        );

        ToolCallRequest request = ToolCallRequest.builder(
                "t_acme",
                "agent_billing",
                "stripe",
                "refund",
                OpenClauseClient.generateIdempotencyKey()
            )
            .params(Map.of("charge_id", "ch_abc123", "amount", 5000))
            .resource("charges/ch_abc123")
            .riskScore(8)
            .build();

        ToolCallResponse response = client.submitToolCall(request);

        switch (response.getDecision()) {
            case "allow":
                System.out.println("Tool call allowed, executing...");
                ToolCallResponse result = client.execute(response.getEventId());
                System.out.println("Result: " + result.getResult().getStatus());
                break;

            case "approve":
                System.out.println("Approval required: " + response.getApprovalUrl());
                ToolCallResponse approved = client.waitForApproval(
                    response.getEventId(), 120_000, 2_000
                );
                System.out.println("Approval decision: " + approved.getDecision());
                break;

            case "deny":
                System.out.println("Denied: " + response.getReason());
                break;
        }
    }
}
```

## Error Handling

```java
import dev.openclause.sdk.exceptions.APIException;
import dev.openclause.sdk.exceptions.OpenClauseException;

try {
    ToolCallResponse response = client.submitToolCall(request);
} catch (APIException e) {
    System.err.println("API error " + e.getStatusCode() + ": " + e.getResponseBody());
} catch (OpenClauseException e) {
    System.err.println("Error: " + e.getMessage());
}
```

## API Reference

### `OpenClauseClient`

| Method | Description |
|--------|-------------|
| `submitToolCall(request)` | Submit a tool call for policy evaluation |
| `getEvent(eventId)` | Retrieve the status of a tool call event |
| `execute(eventId)` | Execute a previously approved tool call |
| `waitForApproval(eventId, timeoutMs, pollIntervalMs)` | Poll until approval is granted or denied |
| `generateIdempotencyKey()` | Generate a UUID idempotency key (static) |

### `ToolCallRequest.Builder`

Use the builder pattern to construct requests:

```java
ToolCallRequest request = ToolCallRequest.builder(tenantId, agentId, tool, action, idempotencyKey)
    .params(params)
    .resource(resource)
    .riskScore(8)
    .riskFactors(List.of("destructive"))
    .userId(userId)
    .sessionId(sessionId)
    .traceId(traceId)
    .schemaVersion("1.0")
    .build();
```
