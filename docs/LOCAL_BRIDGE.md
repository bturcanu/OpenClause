# Local Bridge Alpha

OpenClause v0.5 now includes a thin local bridge alpha for runtimes that want a local OpenClause seam instead of embedding tenant, agent, and API-key wiring directly into every tool call.

Mental model:

`agent/runtime -> local bridge -> OpenClause gateway -> connector`

This is the current best fit for:

- LM Studio and other local OpenAI-compatible model servers
- local tools or wrappers that can already speak HTTP
- teams that want one local config file and one local endpoint instead of repeating OpenClause identity wiring in every runtime

## What the bridge does

The bridge is a local HTTP proxy started by the CLI:

```bash
go run ./cmd/openclause bridge start --config ./openclause-bridge.yaml
go run ./cmd/openclause bridge chat --config ./openclause-bridge.yaml
go run ./cmd/openclause bridge mcp --config ./openclause-bridge.yaml
go run ./cmd/openclause bridge mcp --config ./openclause-bridge.yaml --profile finance
```

It exposes:

- `GET /healthz`
- `GET /v1/bridge/profiles`
- `GET /v1/bridge/tools`
- `GET /v1/models`
- `POST /v1/chat/completions`
- `POST /mcp`
- `DELETE /mcp`
- `POST /v1/toolcalls`
- `GET /v1/toolcalls/{event_id}`
- `POST /v1/toolcalls/{event_id}/execute`

The local bridge:

- injects `tenant_id`
- injects `agent_id`
- uses the configured OpenClause API key
- generates `session_id`, `trace_id`, and `idempotency_key` when the runtime does not provide them
- can restrict the runtime to a small configured tool allowlist
- can apply configured risk scores and other tool defaults
- can expose an OpenAI-compatible chat host backed by your local model server when the `openai:` config block is present
- now also includes a thin terminal chat client via `openclause bridge chat` so you can test the bridge-hosted chat loop without editing starter code
- now also includes `openclause bridge doctor` so you can validate config, gateway reachability, bridge surfaces, upstream model reachability, and API-key auth before the first runtime call
- now also includes a stdio MCP server via `openclause bridge mcp` so native LM Studio chat UI can call governed tools without a custom wrapper app
- `openclause bridge mcp --profile ...` now lets stdio MCP clients pin a non-default profile from a multi-profile bridge config
- can now host multiple named tenant and agent profiles from one config file and route requests by `X-OpenClause-Profile` or `?profile=...`

## Config shape

Generated local-model bundles now include `openclause-bridge.yaml`.

Example:

```yaml
listen: "127.0.0.1:8787"
base_url: "http://localhost:8080"
tenant_id: "tenant-123"
agent_id: "agent-123"
api_key: "env:OPENCLAUSE_API_KEY"

defaults:
  user_id: "pilot-user"
  session_prefix: "support-bot"
  risk_mode: "configured"

openai:
  upstream_base_url: "env:LOCAL_MODEL_BASE_URL"
  upstream_api_key: "env:LOCAL_MODEL_API_KEY"
  model: "env:LOCAL_MODEL_NAME"
  tool_name: "governed_action"
  system_prompt: "Use governed_action whenever the user asks to work with one of these operations: postgres.query.readonly."

tools:
  - tool: "postgres"
    action: "query.readonly"
    risk_score: 2
    description: "Generated first pilot read action for local-model governance."
```

Rules:

- `api_key` can be a literal value or `env:VAR_NAME`
- `tenant_id` and `agent_id` are fixed for the running bridge
- when `tools` are present, the bridge rejects requests for unconfigured tool/action pairs
- `risk_mode=configured` means the bridge applies the configured risk score for matching tools
- when `openai` is present, the bridge also becomes an OpenAI-compatible chat host for local clients

Multi-profile configs are also supported:

```yaml
listen: "127.0.0.1:8787"
default_profile: "support"

profiles:
  support:
    base_url: "http://localhost:8080"
    tenant_id: "tenant-support"
    agent_id: "agent-support"
    api_key: "env:OPENCLAUSE_SUPPORT_API_KEY"
    tools:
      - tool: "postgres"
        action: "query.readonly"
        risk_score: 2
  finance:
    base_url: "http://localhost:8080"
    tenant_id: "tenant-finance"
    agent_id: "agent-finance"
    api_key: "env:OPENCLAUSE_FINANCE_API_KEY"
    tools:
      - tool: "jira"
        action: "jira.issue.create"
        risk_score: 5
```

When multiple profiles are configured:

- set `default_profile`
- use `GET /v1/bridge/profiles` to inspect the loaded profiles
- use `X-OpenClause-Profile: finance` or `?profile=finance` to route HTTP requests
- use `openclause bridge chat --config ./openclause-bridge.yaml --profile finance` for the terminal chat helper
- use `openclause bridge mcp --config ./openclause-bridge.yaml --profile finance` when a stdio MCP client should attach to a non-default profile

## OpenAI-compatible chat host

When the `openai:` block is configured, the bridge also exposes:

- `GET /v1/models`
- `POST /v1/chat/completions`

This gives you a more normal local-model flow:

1. keep LM Studio as the upstream model server
2. start the OpenClause bridge once
3. point your OpenAI-compatible chat client at `http://127.0.0.1:8787/v1`

The bridge then:

- forwards chat completions to the upstream model
- injects a governed tool surface based on the configured `tools`
- merges any client-provided tools that do not shadow the governed tool name
- routes only the governed tool calls through OpenClause
- feeds the governed result back into the model conversation

Current limits:

- the bridge still owns the governed tool surface for this endpoint, but it now passes through client-provided tool definitions and non-governed tool calls when they do not conflict with the governed tool name
- if the upstream model mixes governed and client tool calls in one turn, the bridge executes the governed actions itself, preserves the remaining client tool calls, and returns structured `openclause.governed_results` metadata so the caller can see exactly what already ran
- `stream=true` now keeps streaming assistant content through the governed tool loop and emits remaining client tool calls after each upstream step completes
- approval results are fed back into the conversation as governed tool output, so the assistant can explain pending approval instead of pretending the write already happened

The mixed-turn extension is namespaced under `openclause` instead of using a raw top-level field. Client hosts should treat `openclause.governed_results` as optional bridge metadata and ignore it safely when they do not need governed execution details.

For first-class mixed-turn support, client hosts should:

- keep rendering the assistant text normally
- continue processing any returned client `tool_calls`
- inspect `openclause.governed_results` when present so already-executed governed calls are visible in logs, traces, or chat transcripts
- avoid re-executing governed calls that the bridge has already listed in `openclause.governed_results`

## Bridge Doctor

Run this before the first live request:

```bash
go run ./cmd/openclause bridge doctor --config ./openclause-bridge.yaml
```

Useful flags:

- `--profile finance` for multi-profile bridge configs
- `--json` for machine-readable output

The doctor checks:

- config load and profile resolution
- gateway `/healthz`
- gateway `/v1/connectors`
- configured API-key auth against `/v1/toolcalls`
- in-process bridge `/v1/bridge/tools`
- in-process bridge `/mcp`
- upstream OpenAI-compatible `/models` when `openai:` is configured
- configured model presence in the upstream model list
- in-process bridge `/v1/models` when `openai:` is configured

## Native LM Studio Chat UI Via MCP

LM Studio now has a real native path through the bridge.

1. Start the bridge:

```bash
go run ./cmd/openclause bridge start --config ./openclause-bridge.yaml
```

2. Open the generated `lmstudio.mcp.example.jsonc` or `lmstudio.mcp.remote.example.jsonc`.
3. Copy its `mcpServers` entry into LM Studio's `mcp.json`.
4. If you use the stdio snippet, adjust the command if needed:
   - installed CLI: `openclause bridge mcp --config /absolute/path/to/openclause-bridge.yaml`
   - repo-local fallback: `go run ./cmd/openclause bridge mcp --config /absolute/path/to/openclause-bridge.yaml`
5. If you use the remote snippet, keep the bridge HTTP process running on `http://127.0.0.1:8787` and adjust the URL if you changed the listen address.
6. Enable MCP servers from `mcp.json` in LM Studio.
7. Keep chatting normally in LM Studio; governed tool calls now flow through the bridge MCP server and then through OpenClause.

The stdio MCP path is the most portable option when you want LM Studio to launch everything for you. The remote MCP snippet is the simpler option when you already keep the bridge running as a local service and want LM Studio to connect by URL.

## MCP surfaces

The bridge now exposes two MCP shapes:

- `openclause bridge mcp --config ...`
  - stdio MCP server
  - this is the documented LM Studio integration path
- `POST /mcp`
  - session-aware JSON-RPC-over-HTTP MCP endpoint
  - useful for remote MCP clients, sidecar adapters, or URL-based MCP transports
  - returns `Mcp-Session-Id` after `initialize`
  - accepts `DELETE /mcp` to close a session cleanly

For HTTP MCP clients:

1. `POST /mcp` with `initialize`
2. capture the returned `Mcp-Session-Id`
3. send that header on later `tools/list` and `tools/call` requests
4. optionally send `DELETE /mcp` with the same header when the client is done

## Local-model workflow

For the local OpenAI-compatible onboarding path:

1. Create the agent integration in the console or with `init-agent`.
2. Download the bundle.
3. Load the env vars from `setup-env.sh`.
4. Start the local bridge:

```bash
go run ./cmd/openclause bridge start --config ./openclause-bridge.yaml
```

5. In another terminal, run one of these:

```bash
python local_model_agent.py --smoke
python local_model_agent.py
openclause bridge chat --config ./openclause-bridge.yaml
```

Or, for native LM Studio chat UI, copy either `lmstudio.mcp.example.jsonc` (stdio) or `lmstudio.mcp.remote.example.jsonc` (remote URL) into LM Studio's `mcp.json`.

The generated `local_model_agent.py` now prefers `OPENCLAUSE_BRIDGE_URL` when it is set, accepts `--smoke` for a first governed call, and drops into an interactive chat loop when you run it without flags.
If you already have the OpenClause CLI installed, `openclause bridge chat --config ./openclause-bridge.yaml` gives you a terminal REPL without editing any starter code.
For an even more normal integration, point your own OpenAI-compatible chat client at `OPENCLAUSE_BRIDGE_URL/v1` and let the bridge host the governed tool loop for you.

## Verification

After one successful run:

1. Check `Audit Trail` for the new event.
2. Check `Sessions` for the same `session_id`.
3. If the decision is `approve`, use `Approvals` and then re-check the session chain.

## Current limits

This is an alpha local bridge, not a full new runtime platform.

- It is config-file driven, but can now host multiple named tenant and agent profiles from one bridge process.
- It works with the saved agent integration snapshot and revision history created during onboarding, but it is not a broader cross-agent lifecycle platform by itself.
- Native LM Studio chat UI is still easiest through the stdio MCP path, while `/mcp` now serves as the remote HTTP MCP seam for URL-based clients and sidecar adapters.
- Streaming support now continues assistant SSE output through the governed tool loop, and the bridge preserves remaining client tool calls after each upstream step completes.
- `openclause bridge doctor` is a preflight helper, not a persistent health service; it validates config and reachability before runtime use.
