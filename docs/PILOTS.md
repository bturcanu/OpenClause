# Pilot Recipes

OpenClause creates value fastest when a team runs one small, well-instrumented pilot instead of trying to govern every tool and every agent at once.

This guide turns the current v0.5 onboarding surface into a practical pilot package you can run this week.

## Recommended Starting Shape

Start with:

- one tenant
- one agent
- one approver group
- one read action
- one write action
- one runtime you already control

Recommended runtime order:

1. Python SDK wrapper
2. TypeScript SDK wrapper
3. LangChain if your agent already uses it
4. Local OpenAI-compatible model only when local-model proof is the goal

Recommended approval posture:

- `pilot_safe`

That keeps low-risk reads flowing while routing higher-risk writes into approval so operators can see the system work end to end.

## Canonical Pilot Journeys

### Slack: read plus approved write

Best for:

- support bots
- internal assistants
- agent handoff demos

Start with:

- `slack:slack.channel.list`
- `slack:slack.msg.post`

Why this works:

- the read action gives a fast first success
- the write action exercises the approval path in a way operators understand immediately

### Jira: list plus approved create

Best for:

- engineering operations
- ticket triage copilots
- internal dev-assistant pilots

Start with:

- `jira:jira.issue.list`
- `jira:jira.issue.create`

Why this works:

- operators can inspect both auditability and business usefulness quickly
- a created issue is easy to verify in both OpenClause and Jira

### Postgres readonly plus Webhook post

Best for:

- internal workflow orchestration
- data-aware assistants
- sandboxed integration pilots

Start with:

- `postgres:postgres.query.readonly`
- `webhook:webhook.post`

Why this works:

- the read path proves data access governance without permitting writes
- the webhook write path gives a clear approval and execution trail without needing a complex SaaS connector

## Canonical Monday Plan

If you are starting from zero, use this sequence:

1. Create one tenant and one agent through the onboarding flow.
2. Pick one of the pilot journeys above.
3. Use `pilot_safe`.
4. Send one governed read action.
5. Send one governed write action.
6. Approve the write.
7. Review the tenant pilot cockpit in `Tenant Detail -> Analytics`.

Use these guides for setup details:

- [ONBOARDING.md](ONBOARDING.md)
- [API_ONBOARDING_ENDPOINTS.md](API_ONBOARDING_ENDPOINTS.md)

## What To Watch For In Week 1

Track these signals daily:

- total governed calls
- allow / deny / approve counts
- approval latency
- execution success rate
- missing `session_id` rate
- missing `trace_id` rate
- top connector failures
- top deny reasons

The tenant pilot cockpit now surfaces these directly on `Tenant Detail -> Analytics`.

## Operator Review Checklist

At the end of the first 1-2 weeks, review:

1. Could operators trace one request from Audit Trail to Sessions to Approvals?
2. Were `session_id` and `trace_id` present often enough to make triage useful?
3. Were approval reasons and deny reasons understandable?
4. Did connector failures contain enough context to act on?
5. Did the pilot reduce risk without slowing the team too much?

If the answer to any of those is "not yet", fix that before expanding to more tools or more agents.

## What Success Looks Like

A successful first pilot does not need broad connector coverage. It needs:

- one runtime the team already understands
- one read action that proves traffic is flowing
- one write action that proves approval + execution work
- one operator who can explain what happened without looking at the code

That is the point where OpenClause becomes useful enough to keep.
