# Production Guide

OpenClause v0.5 has a strong local story today. The current production-shaped path is intentionally thinner: deploy the existing services with the shipped Helm charts, point them at real infrastructure, and verify one small pilot before scaling out.

This guide stays honest to what is in the repo today.

## Recommended Deployment Path

Use the service-by-service Helm charts in `deploy/helm/` together with managed or otherwise production-ready shared dependencies:

- Postgres
- OPA
- S3-compatible evidence storage
- SMTP if you want real invite delivery

Core charts already present in the repo:

- `deploy/helm/gateway`
- `deploy/helm/approvals`
- `deploy/helm/console-api`
- `deploy/helm/console-ui`
- `deploy/helm/alert-worker`
- `deploy/helm/connector-slack`
- `deploy/helm/connector-jira`

Recommended first production stack:

- `gateway`
- `approvals`
- `console-api`
- `console-ui`
- `alert-worker`
- `connector-slack` and/or `connector-jira` only if those are part of the pilot

## Minimum External Dependencies

### Postgres

Required for:

- tenants
- agents
- API keys
- tool events
- approvals
- session views
- console auth and user management

Start from the envs in [`.env.example`](../.env.example):

- `POSTGRES_HOST`
- `POSTGRES_PORT`
- `POSTGRES_USER`
- `POSTGRES_PASSWORD`
- `POSTGRES_DB`
- `POSTGRES_SSLMODE`

For production, use TLS and managed backups where possible.

### OPA

Required for policy evaluation.

Key env:

- `OPA_URL`

If OPA is unavailable, tool-call governance cannot make policy decisions reliably.

### Evidence Storage

Required if you want archival and production-shaped evidence retention.

Key envs:

- `EVIDENCE_S3_ENDPOINT`
- `EVIDENCE_S3_BUCKET`
- `EVIDENCE_S3_ACCESS_KEY`
- `EVIDENCE_S3_SECRET_KEY`
- `EVIDENCE_S3_SECURE`

MinIO is fine for non-production or internal environments. For serious pilots, use hardened S3-compatible storage and backup/versioning.

### Shared Secrets

At minimum, manage these outside plain chart values:

- `CONSOLE_JWT_SECRET`
- `INTERNAL_AUTH_TOKEN`
- database credentials
- evidence storage credentials
- connector credentials such as `SLACK_BOT_TOKEN` or Jira API tokens
- `SMTP_PASS` if using SMTP

Use each chart's `secretRef` support instead of hard-coding secrets into values files.

## Recommended Rollout Order

1. Provision Postgres, OPA, and evidence storage.
2. Deploy `gateway`, `approvals`, `console-api`, and `console-ui`.
3. Deploy only the connectors the pilot actually needs.
4. Run migrations and health checks.
5. Complete console first-run setup.
6. Onboard one tenant and one agent.
7. Send one governed read action and one governed write action.
8. Verify the tenant pilot cockpit, Audit Trail, Sessions, and Approvals.

## Backup And Restore

### Postgres

Back up:

- the database itself via managed snapshots or `pg_dump`
- migration state

Restore drill:

1. restore a fresh Postgres instance
2. point a staging deployment at it
3. verify tenants, agents, API keys, sessions, and approvals render correctly
4. send one governed tool call before trusting the restore

### Evidence Storage

Back up:

- the evidence bucket
- lifecycle/versioning configuration if used

Restore drill:

1. restore the bucket or object prefix
2. verify archived evidence is readable
3. confirm the archiver can continue from current checkpoints

## Upgrade Guide

OpenClause does not yet ship a one-command production upgrader. The honest path today is:

1. read the release notes and migration notes
2. back up Postgres and evidence storage
3. update container images / chart refs
4. apply schema migrations
5. `helm upgrade` each deployed service
6. run smoke verification:
   - console login
   - one allow path
   - one approve path
   - one archive/export path if used
7. verify the tenant pilot cockpit still shows healthy recent traffic

## Failure-Mode Runbook

### Gateway unavailable

Impact:

- agents cannot submit governed tool calls

What to check:

- gateway health endpoint
- ingress/service routing
- database reachability
- OPA connectivity

### OPA unavailable

Impact:

- policy evaluation fails
- new governed requests cannot be trusted

What to check:

- `OPA_URL`
- policy bundle mount / OPA logs
- network path from gateway and console-api

### Connector unavailable

Impact:

- allow/approved actions may fail during execution

What to check:

- connector health endpoint
- connector credentials
- top connector failures in `Tenant Detail -> Analytics`

### Approvals unavailable

Impact:

- approval-required requests can pile up or fail to resolve

What to check:

- approvals health endpoint
- Slack connector/notification path if interactive approvals are used
- pending approval count and oldest pending time in the pilot cockpit

### Missing session or trace context

Impact:

- sessions, audit linkage, and triage quality degrade fast

What to check:

- generated runtime smoke code
- custom agent wrapper code
- pilot cockpit missing-context rates

## Production Verification Checklist

Before calling a deployment ready for a pilot, confirm:

1. console login works
2. tenant creation works
3. one agent can be onboarded
4. one governed read action appears in Audit Trail and Sessions
5. one approval-gated write appears in Approvals and can be executed
6. the pilot cockpit shows recent event/session/approval data
7. connector failures and deny reasons are visible enough to debug

## Current Limits

The production story is improving, but still intentionally thin:

- service-by-service Helm charts, not a single turnkey production distribution
- the local bridge and MCP runtime seam now exist, but they are still alpha and config-driven rather than a managed runtime platform
- onboarding now persists one saved integration record per tenant and agent, but there is still no broader integration history or lifecycle API family

That is enough for real pilots. It is not yet a full enterprise platform package.
