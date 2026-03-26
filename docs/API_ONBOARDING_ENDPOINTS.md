# Onboarding API Endpoints

OpenClause v0.5 ships a thin onboarding/admin contract surface for generated bundles. These endpoints are implemented in the console API and documented in [`api/openapi.yaml`](../api/openapi.yaml).

This remains a narrow surface over the existing admin model plus one saved integration record per tenant and agent:

- tenants
- agents
- API keys
- agent-scoped integration metadata
- the shared onboarding bundle generator in `pkg/onboarding`

## Endpoint Summary

| Endpoint | Mutates state | Purpose | Raw API key behavior |
| --- | --- | --- | --- |
| `POST /admin/onboarding/integrations` | Yes | Create or select tenant context, create agent + API key, and return a generated bundle | Returns the raw key once |
| `POST /admin/onboarding/bundles/preview` | No | Generate a non-destructive bundle for an existing tenant | Never returns raw key |
| `POST /admin/onboarding/bundles/regenerate` | No | Regenerate artifacts for an existing tenant + agent and refresh the saved onboarding metadata on that agent | Never reissues raw key |
| `POST /admin/onboarding/bundles/regenerate-defaults` | No | Regenerate artifacts from existing tenant + agent context using saved metadata when valid, then explicit curated defaults | Never reissues raw key |
| `POST /admin/onboarding/bundles/archive` | No | Return a zip of writable bundle artifacts | Uses the bundle payload you send |
| `GET /admin/tenants/{tenant_id}/agents/{agent_id}/integration` | No | Fetch the saved onboarding integration snapshot for an existing agent | No raw key |
| `GET /admin/tenants/{tenant_id}/agents/{agent_id}/integration/revisions` | No | List recent saved integration revisions for an existing agent | No raw key |
| `GET /admin/tenants/{tenant_id}/agents/{agent_id}/integration/bundle` | No | Rebuild a saved bundle or defaults bundle from the persisted integration snapshot | Never reissues raw key |

All onboarding/admin endpoints require the console bearer token used by the rest of the admin surface.

Representative example payloads live under [`docs/examples/onboarding/`](examples/onboarding/):

- [`created-response.json`](examples/onboarding/created-response.json)
- [`preview-response.json`](examples/onboarding/preview-response.json)
- [`regenerated-response.json`](examples/onboarding/regenerated-response.json)
- [`regenerated-defaults-response.json`](examples/onboarding/regenerated-defaults-response.json)
- [`fetched-response.json`](examples/onboarding/fetched-response.json)
- [`fetched-defaults-response.json`](examples/onboarding/fetched-defaults-response.json)
- [`integration-record.json`](examples/onboarding/integration-record.json)
- [`integration-revisions-response.json`](examples/onboarding/integration-revisions-response.json)
- [`archive-response.http`](examples/onboarding/archive-response.http)

## Request Shapes

## Create

`POST /admin/onboarding/integrations`

Provide either:

- `tenant_id` for an existing tenant, or
- `new_tenant_name` for inline tenant creation

Also provide:

- `runtime`
- `agent_name`
- `tools`
- optional `approval_posture`
- optional `environment_label`
- optional `owner_name`
- optional `description`

## Preview

`POST /admin/onboarding/bundles/preview`

Preview is existing-tenant only. It requires:

- `tenant_id`
- `runtime`
- `agent_name`
- `tools`

Preview intentionally rejects inline tenant creation.

## Regenerate

`POST /admin/onboarding/bundles/regenerate`

Requires:

- `tenant_id`
- `agent_id`
- `runtime`
- `tools`

It can also carry the same optional metadata fields used by create for README/bundle notes.

When regenerate succeeds, the latest runtime, tools, approval posture, environment label, owner, and description are written back onto the saved integration record for that tenant and agent.

## Regenerate With Defaults

`POST /admin/onboarding/bundles/regenerate-defaults`

Requires:

- `tenant_id`
- `agent_id`

Optional metadata fields can still be passed through for README/bundle notes.

Current server-side defaults are explicit and reviewable:

- first prefer the saved integration record for that tenant and agent when it is still valid in the current connector catalog
- then fall back to runtime `python`
- then fall back to approval posture `pilot_safe`
- then fall back to up to two curated actions from the live connector catalog

If no curated defaults are available, the endpoint fails clearly with `409`.

## Shared Response Envelope

Create, preview, regenerate, regenerate-defaults, and saved-bundle fetch all return the same thin envelope:

- `mode`
- `tenant`
- `agent`
- optional `integration`
- optional `api_key`
- `bundle`

`integration` is omitted for preview and present for create, regenerate, regenerate-defaults, fetched, and fetched-defaults. It reflects the latest saved onboarding/runtime metadata for that tenant and agent:

- runtime
- approval posture
- environment label
- owner name
- description
- selected tools
- created/updated timestamps

The `bundle` contains:

- environment map + shell script + `.env.example`
- starter runtime file
- README snippet
- sample smoke call
- explicit artifact metadata
- executable hints for shell artifacts like `setup-env.sh` and `smoke-test.sh`
- verification checklist
- deep links into Audit Trail, Sessions, and Approvals
- notes
- optional `applied_defaults`

For the `openai_local` runtime, the bundle can also include `openclause-bridge.yaml`, which is a generated local bridge config artifact. That file can now carry both:

- OpenClause bridge identity/tool settings
- upstream OpenAI-compatible model host settings for the local chat-host alpha
- an `lmstudio.mcp.example.jsonc` snippet for the LM Studio stdio MCP path
- an `lmstudio.mcp.remote.example.jsonc` snippet for the LM Studio remote MCP path against the local bridge `/mcp` endpoint

The bridge remains a CLI/runtime feature, not a new onboarding/admin persistence model or endpoint family.

The generated README, starter file, and smoke call are intentionally mode-aware:

- preview explains placeholder credentials
- create reflects one-time raw-key visibility
- regenerate/regenerate-defaults explain existing-key reuse or missing-key recovery
- fetched/fetched-defaults rebuild the saved bundle without mutating tenant, agent, or API-key state

## Saved Integration Endpoints

`GET /admin/tenants/{tenant_id}/agents/{agent_id}/integration`

- returns the saved integration snapshot for that tenant and agent
- exposes the latest persisted runtime, posture, tool, environment, and owner metadata

`GET /admin/tenants/{tenant_id}/agents/{agent_id}/integration/revisions?limit=5`

- returns recent saved integration revisions in descending `created_at` order
- revisions capture the last create/regenerate/defaulted-regenerate writes for that agent-scoped integration

`GET /admin/tenants/{tenant_id}/agents/{agent_id}/integration/bundle`

- returns a rebuilt read-only bundle response with `mode=fetched`
- when `defaults=true`, returns `mode=fetched_defaults`
- when `archive=true`, returns `application/zip`
- never creates tenant, agent, or API-key state
- omits `api_key` when no active key exists and pushes the recovery action into bundle notes instead

## Raw API Key Rules

These rules are strict and intentional:

- create is the only flow that returns `api_key.raw_key`
- preview never returns `api_key.raw_key`
- regenerate never returns `api_key.raw_key`
- regenerate-defaults never returns `api_key.raw_key`
- if no active API key exists during regenerate, the response omits `api_key` entirely and adds operator guidance to the bundle notes

Generated env artifacts follow the same honesty rule:

- create can embed the one-time raw key
- preview uses a placeholder like `${OPENCLAUSE_API_KEY:-generated-on-create}`
- regenerate uses a placeholder like `${OPENCLAUSE_API_KEY:-reuse-existing-key}`
- saved bundle fetch uses the same existing-key placeholder as regenerate

## Archive Endpoint

`POST /admin/onboarding/bundles/archive`

This endpoint accepts the same onboarding bundle response envelope and returns `application/zip`.

The zip contains only writable artifacts, such as:

- `setup-env.sh`
- `.env.example`
- starter runtime file
- `README.onboarding.md`
- `smoke-test.sh`
- `package.onboarding.json` for the TypeScript golden path
- `openclause-bridge.yaml` for the local OpenAI-compatible golden path

The archive endpoint does not store bundles server-side. It only packages the bundle payload you send.

## Verification Expectations

Generated starter code and smoke tests are expected to send:

- `tenant_id`
- `agent_id`
- `session_id`
- `trace_id`
- `idempotency_key`

Use the generated verification links to check:

1. Audit Trail for the first governed event
2. Sessions for the expected `session_id`
3. Approvals when the selected action routes through approval

## Current Limits

The onboarding contract is still intentionally thin:

- saved lifecycle history is still agent-scoped; there is not yet a broader cross-agent integration inventory, ownership, or lifecycle workflow
- the local bridge is shipped as a CLI/runtime artifact, not as a new onboarding/admin persistence or admin-endpoint surface
- native LM Studio and remote MCP support are delivered through generated runtime artifacts and the CLI bridge, not through a separate onboarding/admin API family
- CLI users can now bootstrap auth with `openclause auth login`, but server-backed flows still assume a reachable console API and an existing console user
