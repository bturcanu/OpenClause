# OpenClause Python SDK

Python client for the [OpenClause](https://openclause.dev) agentic access governance gateway.

## Install

```bash
pip install openclause
```

## Quick Start (5 minutes)

```python
from openclause import OpenClauseClient, ToolCallRequest

client = OpenClauseClient(
    base_url="http://localhost:8080",
    api_key="sk-test-key-1"
)

# Submit a tool call for policy evaluation
response = client.submit_tool_call(ToolCallRequest(
    tenant_id="tenant1",
    agent_id="agent-1",
    tool="slack",
    action="msg.post",
    idempotency_key=OpenClauseClient.generate_idempotency_key(),
    params={"channel": "#general", "text": "Hello from OpenClause!"},
    risk_score=3
))

if response.decision == "approve":
    # Wait for human approval, then auto-execute
    result = client.wait_for_approval(response.event_id)
    print(result.result)
elif response.decision == "allow":
    print(response.result)
```

## Retrieving Events

```python
event = client.get_event(response.event_id)
print(event.decision, event.reason)
```

## Manual Execution

If a tool call has been approved and you want to trigger execution yourself:

```python
result = client.execute(response.event_id)
print(result.result.status)
```

## Error Handling

```python
from openclause.exceptions import APIError, AuthenticationError, TimeoutError

try:
    resp = client.submit_tool_call(request)
except AuthenticationError:
    print("Bad API key")
except APIError as e:
    print(f"Server returned {e.status_code}: {e.message}")
except TimeoutError:
    print("Request timed out")
```

## Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `base_url` | — | Root URL of the OpenClause gateway |
| `api_key` | — | API key for authentication |
| `timeout` | `30.0` | HTTP request timeout (seconds) |

## Development

```bash
# Install dev dependencies
pip install -e ".[dev]"

# Run tests
pytest
```

## License

Apache-2.0
