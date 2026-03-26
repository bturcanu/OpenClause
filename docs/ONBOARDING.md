# Agent Onboarding Guide

OpenClause v0.5 treats onboarding as a generated bundle workflow rather than a list of low-level API steps. The current golden paths are:

- Python SDK wrapper
- TypeScript SDK wrapper
- LangChain
- Local OpenAI-compatible model

This guide covers the shipped lifecycle today:

- Console create
- Console preview
- Console regenerate
- Console regenerate with defaults
- CLI local-only generation
- CLI server-backed preview, create, regenerate, and regenerate with defaults

For a concrete first pilot after onboarding, use [PILOTS.md](PILOTS.md). For the current production-shaped deployment and failure-mode guidance, use [PRODUCTION.md](PRODUCTION.md).

The shipped admin/API contract for those flows is documented in [`docs/API_ONBOARDING_ENDPOINTS.md`](API_ONBOARDING_ENDPOINTS.md) and captured in [`api/openapi.yaml`](../api/openapi.yaml). Representative response examples also live in [`docs/examples/onboarding/`](examples/onboarding/).

## Five-Minute Success Path

1. Start the stack with `make dev` and finish console setup at `http://localhost:3000`.
2. Open `Tenants` or a specific tenant detail page.
   You can also start from `Overview -> Create Agent Integration`, which routes into the same Tenants onboarding flow.
3. Launch `Create Agent Integration`.
4. Pick a runtime, choose governed tools, and preview the bundle first.
5. Create the real integration when the bundle looks right.
6. Download the archive or copy the generated files into your runtime.
7. Run the generated smoke test.
8. Verify the first request in Audit Trail, Sessions, and Approvals.

If you prefer terminal workflows, use `go run ./cmd/openclause init-agent ...` with either local-only or server-backed mode.

For the local OpenAI-compatible path, treat LM Studio as an OpenAI-style server rather than a separately registered OpenClause object:

1. Start LM Studio's local server.
2. Run `curl http://localhost:1234/v1/models` and copy the returned model id.
3. Put that id into `LOCAL_MODEL_NAME` in the generated env artifacts.
4. Start the local bridge with `go run ./cmd/openclause bridge start --config ./openclause-bridge.yaml`.
5. Either:
   - run `python local_model_agent.py --smoke` for the first governed call,
   - run `python local_model_agent.py` for an interactive chat loop, or
   - run `openclause bridge chat --config ./openclause-bridge.yaml` if you already have the OpenClause CLI installed, or
   - point your own OpenAI-compatible chat client at `OPENCLAUSE_BRIDGE_URL/v1`, or
   - copy either `lmstudio.mcp.example.jsonc` (stdio) or `lmstudio.mcp.remote.example.jsonc` (remote URL) into LM Studio's `mcp.json`
6. Run `python -m pip install --upgrade pip setuptools wheel` followed by `python -m pip install openai && python -m pip install --no-build-isolation -e ../sdk/python`.

If your generated bundle should be used from your host machine instead of inside the Docker network, set `PUBLIC_GATEWAY_URL` on `console-api` so downloaded bundles use a host-reachable `OPENCLAUSE_BASE_URL` such as `http://localhost:8080`. The local Docker compose stack now sets this by default, so downloaded bundles work from the host without editing `setup-env.sh`.

## Console Lifecycle

### Create

Use this when you want OpenClause to create the real tenant context, agent, and API key.

- Entry points:
  - `Tenants` page header action
  - `Tenants` row-level `Onboard agent` action for a fixed tenant
  - `Tenant Detail -> Agents`
- Result:
  - creates tenant when requested
  - creates agent
  - creates API key
  - for `pilot_safe` and `read_only_first`, applies a starter tenant policy config for the selected tools so the first governed call is not blocked by an empty allowlist
  - returns the one-time raw API key
  - shows the raw key in the result view with a copy action and embeds it in the generated env artifacts once
  - returns a generated artifact bundle and verification links
  - persists the runtime, tools, approval posture, environment label, owner, and description on the agent for future regenerations

### Preview

Use this when you want to inspect the generated env vars, starter file, README, and smoke test without mutating state.

- Requires an existing tenant
- Uses a synthetic preview agent id
- Never creates credentials
- Bundle env output uses a placeholder API key value

### Regenerate

Use this when you already have a tenant and agent and want refreshed artifacts without minting new state.

- Requires `tenant_id` and `agent_id`
- Does not create tenant, agent, or API key
- Does not recover or reissue a raw API key
- Returns an existing key reference when an active key is available
- Returns operator guidance when no active key exists
- Revalidates the saved tool selections against the live connector catalog before enabling the flow, so stale hidden tool choices do not sneak into regeneration
- Refreshes the saved onboarding metadata on the agent so future regenerations start from the latest runtime, tool, and posture choices

### Regenerate With Defaults

Use this when you want the fastest safe handoff from existing tenant + agent context.

- Requires `tenant_id` and `agent_id`
- Applies explicit, reviewable defaults
- Current defaults:
  - first tries the saved onboarding metadata already stored on the agent
  - falls back to `python`
  - falls back to approval posture `pilot_safe`
  - falls back to up to two curated safe actions from the current connector catalog
- Fails clearly when curated defaults are unavailable

### Saved Bundle Fetch

Use this when you want to come back later, download the latest saved handoff, or inspect recent integration history without mutating anything.

- Available from `Tenant Detail -> Agents -> View history`
- Rebuilds the saved bundle from the persisted integration snapshot
- Can also return a defaults bundle from the same saved integration context
- Never creates tenant, agent, or API-key state
- Keeps raw-key behavior honest:
  - returns an existing key reference when an active key exists
  - omits `api_key` entirely and shows recovery notes when no active key exists

## Starter Policy Behavior

OpenClause onboarding now does a little more than create credentials.

- `pilot_safe` create:
  - adds selected read-like actions to the tenant read allowlist
  - adds selected write-like actions to the tenant destructive/approval allowlist
  - sets `max_risk_auto_approve=4`
- `read_only_first` create:
  - adds selected read-like actions to the tenant read allowlist
  - leaves selected write-like actions denied until an operator expands policy deliberately
  - sets `max_risk_auto_approve=4`
- `tenant_default` create:
  - does not change tenant policy config

This starter policy is additive. It reuses the existing tenant policy-config seam and merges the onboarding-selected actions instead of introducing a separate policy/onboarding control plane.

## CLI Lifecycle

`cmd/openclause` reuses the same shared onboarding contracts and artifacts as the console.

### Local-only bundle generation

```bash
go run ./cmd/openclause init-agent \
  --local-only \
  --tenant-id tenant-123 \
  --tenant-name "Demo Corp" \
  --agent-name "Support Bot" \
  --runtime typescript \
  --tools slack:slack.channel.list,slack:slack.msg.post \
  --output-dir ./support-bot
```

### Server-backed preview

```bash
go run ./cmd/openclause auth login \
  --server-url http://localhost:8090 \
  --email admin@openclause.dev \
  --password 'Admin123!'

go run ./cmd/openclause init-agent \
  --server-url http://localhost:8090 \
  --preview \
  --tenant-id tenant-123 \
  --agent-name "Support Bot" \
  --runtime python \
  --tools slack:slack.channel.list
```

### Server-backed create

```bash
go run ./cmd/openclause init-agent \
  --server-url http://localhost:8090 \
  --tenant-id tenant-123 \
  --agent-name "Support Bot" \
  --runtime typescript \
  --tools slack:slack.channel.list,slack:slack.msg.post \
  --output-dir ./support-bot
```

### Server-backed regenerate

```bash
go run ./cmd/openclause init-agent \
  --server-url http://localhost:8090 \
  --regenerate \
  --tenant-id tenant-123 \
  --agent-id agent-123 \
  --agent-name "Support Bot" \
  --runtime python \
  --tools slack:slack.channel.list
```

### Server-backed regenerate with defaults

```bash
go run ./cmd/openclause init-agent \
  --server-url http://localhost:8090 \
  --regenerate \
  --use-defaults \
  --tenant-id tenant-123 \
  --agent-id agent-123
```

## Artifact Bundle Shape

The onboarding bundle is intentionally file-oriented so the console and CLI can share it directly.

Writable artifacts currently include:

- `setup-env.sh`
- `.env.example`
- runtime starter file such as `agent.py`, `agent.ts`, `langchain_agent.py`, or `local_model_agent.py`
- `README.onboarding.md`
- `smoke-test.sh`
- `package.onboarding.json` for the TypeScript golden path
- `openclause-bridge.yaml` for the local OpenAI-compatible golden path
- `lmstudio.mcp.example.jsonc` for the native LM Studio stdio MCP path
- `lmstudio.mcp.remote.example.jsonc` for the native LM Studio remote MCP path against the local bridge `/mcp` endpoint

The console can also download the writable artifact set as a zip archive from the onboarding result view.

Each generated bundle now includes:

- runtime-specific setup guidance
- a starter file that shows the approval wait branch explicitly
- a smoke command for the selected runtime
- verification links and notes that make the first event/session check faster

## Local Bridge Alpha

OpenClause now ships a thin local bridge alpha for the local OpenAI-compatible path.

- Start it with `go run ./cmd/openclause bridge start --config ./openclause-bridge.yaml`
- Generated local-model bundles now include `openclause-bridge.yaml`
- Generated local-model bundles now also include both `lmstudio.mcp.example.jsonc` and `lmstudio.mcp.remote.example.jsonc` for native LM Studio chat-UI tool access
- Generated `local_model_agent.py` now behaves as a thin OpenAI-compatible chat client pointed at the bridge and supports both `--smoke` and interactive usage without editing the file
- The bridge also exposes `GET /v1/models` and `POST /v1/chat/completions` for OpenAI-compatible clients
- The bridge now also exposes `openclause bridge mcp --config ...` and `POST /mcp` for MCP-based tool hosting
- `openclause bridge mcp --profile ...` can now pin the stdio MCP server to a non-default profile from a multi-profile bridge config
- The bridge now also exposes `openclause bridge doctor --config ...` so users can validate config, gateway reachability, API-key auth, upstream model reachability, bridge models, and MCP startup before they hit the first runtime error
- The bridge injects tenant, agent, API key, and default request metadata so your runtime code can stay thinner
- `POST /v1/chat/completions` now supports governed streaming through the full tool loop, so assistant content can continue streaming after governed tool execution instead of waiting for a final buffered answer
- The bridge now merges client-provided tools with the generated governed tool when it hosts OpenAI-compatible chat, and only intercepts the governed tool surface
- The bridge config now supports multi-profile sidecar setups, so one local bridge can host more than one tenant and agent pair and route requests by profile header or query selection

Use [`LOCAL_BRIDGE.md`](LOCAL_BRIDGE.md) for the exact config shape and local endpoint behavior.

## Reading The Result View

The console result view is intentionally explicit about what happened:

- Preview:
  - nothing was created
  - tenant context is reused
  - agent id is synthetic (`preview-*`)
  - API key values are placeholders only
- Create:
  - tenant is created only if you asked for inline tenant creation
  - agent and API key are real
  - the full raw API key is shown only in that create result, can be copied directly from the result view, and is embedded in the generated env artifacts once
- Regenerate:
  - tenant and agent are reused
  - raw API key is never reissued
  - the result points to an existing key prefix when one is available
  - the saved integration record for that tenant and agent is refreshed with the latest runtime, tool, posture, and handoff metadata
  - the saved agent setup is updated so the next regenerate flow starts from the latest values
- Regenerate with defaults:
  - tenant and agent are reused
  - raw API key is never reissued
  - runtime, approval posture, and tool defaults are shown explicitly in the result before handoff
  - the saved integration record is the canonical source for defaults when it is still valid in the current connector catalog
  - otherwise OpenClause falls back to the curated golden-path defaults
- Saved bundle fetch:
  - tenant and agent are reused
  - the bundle is rebuilt from the saved integration snapshot without mutating tenant, agent, or API-key state
  - `Tenant Detail` now exposes recent saved integration history plus one-click download of the saved or defaults bundle

Use the result summary first, then work through:

1. Environment
2. Files
3. Test call
4. Verify in console

## Raw API Key Rules

These rules are strict on purpose:

- Create is the only mode that returns a raw API key.
- Preview never returns a raw API key.
- Regenerate never reissues a raw API key.
- Regenerate with defaults never reissues a raw API key.
- When no active key exists, regenerate omits `api_key` entirely and tells the operator to create or rotate a key first.

Generated env content follows the same rules:

- Create can embed the raw key once.
- Preview uses a placeholder such as `${OPENCLAUSE_API_KEY:-generated-on-create}`.
- Regenerate uses an existing-key placeholder such as `${OPENCLAUSE_API_KEY:-reuse-existing-key}`.

## Verification

Generated smoke calls and starter code are expected to carry:

- `tenant_id`
- `agent_id`
- `session_id`
- `trace_id`
- `idempotency_key`

Those fields are onboarding-critical because they drive:

- Sessions grouping
- Audit Trail filtering
- approval/execution linkage
- operator triage

Recommended verification flow:

1. Run one governed read action.
2. Open Audit Trail and filter to the generated tenant + agent.
3. Open Sessions and confirm the same `session_id`.
4. Run one higher-risk write action.
5. Open Approvals and confirm the gated action appears.
6. Approve it and verify execution shows up in the same session chain.

After that first-run check, open `Tenant Detail -> Analytics` and use the pilot cockpit to answer:

- was the first event seen?
- did the agent produce a usable `session_id` and `trace_id`?
- are approvals piling up?
- are connector failures or deny reasons blocking the pilot?
- what should the operator do next?

## Troubleshooting

### Preview vs create confusion

- Preview is non-destructive and uses a synthetic preview agent id.
- Create is the real persistence step.

### Regenerate shows no API key

- This means the tenant has no active API key.
- Create or rotate a key from `Tenant Detail -> API Keys`, then regenerate again.

### Regenerate starts from the wrong tool or runtime

- OpenClause now treats the saved integration record for the tenant and agent as the source of truth for regenerate/defaulted-regenerate.
- Manual regenerate updates that saved integration state atomically, and the saved integration remains the only onboarding contract surface for later console and CLI handoffs.
- Regenerate with defaults prefers the saved integration setup when it is still valid, then falls back to the explicit curated defaults.

### Server-backed CLI mode asks for an auth token

- Use `openclause auth login --server-url http://localhost:8090 --email ... --password ...` once.
- That stores a reusable bearer token for later `openclause init-agent --server-url ...` runs.
- `openclause auth whoami` shows the current stored profile, and `openclause auth logout` removes it.

### Bridge or LM Studio setup fails before the first request

- Run `openclause bridge doctor --config ./openclause-bridge.yaml`.
- It checks:
  - bridge config loading
  - gateway `/healthz`
  - gateway connector catalog reachability
  - configured API-key auth against the gateway
  - bridge `/v1/bridge/tools`
  - bridge `/mcp`
  - upstream `/models` when `openai:` is configured
  - configured model presence in the upstream model list
- Use `--json` if you want machine-readable output for scripts.

### Regenerate with defaults is unavailable

- This means the current connector catalog does not expose a curated default action yet.
- Use manual regenerate to choose tools explicitly, or enable a supported connector action first.

### Preview button stays disabled

- Preview needs:
  - an existing tenant
  - an agent name
  - at least one governed tool
- Inline tenant creation is only applied during the real create step.

### First traffic does not show up in Sessions

- Check that `session_id` and `trace_id` are being generated and sent.
- Re-run the generated smoke test before debugging custom runtime code.

### Approval path is missing

- Confirm the selected tool/action is high-risk in your current policy posture.
- Use the generated verification links to jump straight to `Approvals?tenant_id=...`.

## Current Limits

The current v0.5 onboarding seam is still intentionally reviewable:

- onboarding persistence is now a saved per-agent integration snapshot plus recent revision history, not a broader cross-agent lifecycle/inventory system
- local bridge alpha now includes:
  - native LM Studio chat-UI support through the stdio MCP path
  - a session-aware HTTP `/mcp` seam for remote MCP clients and sidecar-style adapters
  - a bridge-hosted OpenAI-compatible chat endpoint with governed streaming through the tool loop
  - a bridge doctor/preflight command for config and reachability checks
  - multi-profile bridge configs for broader local sidecar/runtime use
- CLI auth bootstrap now exists through `openclause auth login`, but it still assumes a reachable console API and an existing console user

That keeps the system reviewable while the shared bundle, console flow, and CLI lifecycle harden.
