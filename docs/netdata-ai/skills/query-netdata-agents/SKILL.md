---
name: query-netdata-agents
description: Query or explain direct Netdata Agent APIs and Functions; review direct-query recipes or helpers; troubleshoot bearer authentication. Use query-netdata-cloud for Cloud-proxied calls.
---

# Query Netdata Agents Directly

Use this skill for direct HTTP access to an Agent, parent or child, usually on port 19999. It serves operators and
assistants helping them. The sibling [Cloud skill](../query-netdata-cloud/SKILL.md) covers Cloud transport; matching
Function payloads use the same underlying Agent schemas.

## Choose The Task

- **Explain or review:** read the relevant guide and helper as reference material. Do not source helpers, load
  credentials, mint bearers or run live requests just because the skill was loaded.
- **Run a requested query:** use [Prerequisites](#prerequisites), [Safe Execution](#safe-execution) and the relevant
  domain guide. A query request does not authorize DynCfg updates or other state-changing Functions.
- **Diagnose authentication or maintain the helper:** read [authentication.md](./authentication.md) and the relevant
  symbol in [scripts/_lib.sh](./scripts/_lib.sh). Preserve authorization already given for the current task.

For Cloud team members, Cloud transport is the default when no direct route was requested. Direct access avoids the
Cloud request round trip, but this skill's bearer wrapper still needs a reusable cache or Cloud access to mint.
`agents_call_function` defaults to `--via cloud`; `--via agent` is explicit and has no automatic Cloud fallback.

## Domain Guides

| Domain | Read |
|---|---|
| Generic Functions and discovery | [query-functions.md](./query-functions.md) |
| Logs: systemd-journal, windows-events, macos-logs, otel-logs | [query-logs.md](./query-logs.md) |
| Topology, including topology:snmp | [query-topology.md](./query-topology.md) |
| Network flows: flows:netflow | [query-flows.md](./query-flows.md) |
| Alerts through v3 paths | [query-alerts.md](./query-alerts.md) |
| DynCfg: /api/v3/config | [query-dyncfg.md](./query-dyncfg.md) |
| Time-series metrics: /api/v3/data | [query-metrics.md](./query-metrics.md) |
| Node identity, hardware and vnodes | [query-nodes.md](./query-nodes.md) |
| Streaming, parent/child and replication | [query-streaming.md](./query-streaming.md) |
| Operational recipes | [how-tos/INDEX.md](./how-tos/INDEX.md) |

Read the matching guide for body schemas rather than loading every domain. End users MAY use the helpers as a black
box or read their source as a reference implementation.

## Prerequisites

Explanation and saved-data review need no live credentials. The shipped executable recipes require Bash, Git, curl,
jq and a repository checkout containing the helper. Direct calls require network access to the selected Agent.

For the managed bearer flow, configure `NETDATA_CLOUD_TOKEN` and `NETDATA_CLOUD_HOSTNAME` locally, then load the
helper and call `agents_load_env`. It sources the checkout's trusted `.env`; query wrappers themselves do not load
that file. Supply the target node UUID, machine GUID and `host:port` to direct wrappers. Space/room IDs are needed
only by endpoints that use them. Do not require them for every direct call.

Never request credential values in conversation. Use env-key placeholders; the user configures `.env` locally.
[Configuration](./authentication.md#configuration) documents existing key roles and identity lookup.
The direct wrapper always resolves a bearer, including for open endpoints; it does not detect protection or switch to
unauthenticated access. For an unauthenticated reachability check, discard the potentially sensitive info body:

```bash
curl -sS --max-time 10 -o /dev/null -w '%{http_code}\n' \
  "http://${AGENT_HOST:?set the Agent host:port}/api/v3/info"
```

An HTTP success establishes reachability for that request, not permission to call every API. See
[Protection And Headers](./authentication.md#protection-and-headers) for status interpretation.

## Safe Execution

Use `agents_query_cloud`, `agents_query_agent` or `agents_call_function` for credential-bearing requests. Do not write
raw curl commands containing live auth headers or invoke internal mint helpers directly. Header notation in the auth
reference describes the protocol, not a request to expose credentials. Unauthenticated probes and local processing
MAY use ordinary commands when they capture or suppress sensitive response fields.

The wrappers mask request-auth values in their command log and keep internal mint/claim discovery private. They
**forward endpoint responses unchanged**: logs, config and identity responses can contain secrets or identifiers.
Cloud tokens, Agent bearers and claim IDs MUST NOT reach assistant-visible output. Capture sensitive responses in a
shell variable or private local artifact, or pipe directly to an appropriate field projection before display. Do not
use a general query wrapper to display a credential-issuing endpoint's response. Masked request logging is not response
sanitization; do not enable shell tracing around credentials.

For example, after choosing the requested target locally:

```bash
source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env
agents_query_agent \
  --node "${NODE_UUID:?set the node UUID}" --host "${AGENT_HOST:?set the Agent host:port}" \
  --machine-guid "${AGENT_MG:?set the machine GUID}" \
  POST '/api/v3/function?function=systemd-journal' '{"info":true}' \
  | jq '{status, type}'
```

For an execution recipe, provide a complete runnable invocation and the response fields needed for the question.
Explanations and reviews do not need an unrelated live command appended. Function-specific parameters and output
projections belong in the domain guide. [Helper Interfaces](./authentication.md#helper-interfaces)
records all public interfaces, credential/cache handling and safe test commands.

Bearers stay in local configuration/cache or private in-memory variables. Never commit credential values, claim IDs,
node UUIDs or machine GUIDs. Repository users follow `.agents/sensitive-data-discipline.md`; outside a checkout,
apply the same redaction to shared artifacts. A private cache is not an artifact to display or publish.

## Knowledge Capture

- For answer-only questions, deliver the requested answer. If the work reveals a reusable, evidence-backed recipe
  not already documented, you MUST preserve a sanitized note with the finding, supporting evidence, and proposed
  owning guide. In a repository checkout, use `<repo-root>/.local/audits/<subject>/followups.md`, reusing this skill's
  audit directory when available. Outside a checkout, use an appropriate local workspace. If no writable workspace
  is available, include the sanitized follow-up in the response instead.
- Briefly report reusable documentation discoveries and proposed updates in the answer-only final response, even
  when recorded locally. Obtain authorization before those guide edits; do not delay the answer while awaiting it.
- During authorized implementation, you MUST update this skill or its guides for reusable, evidence-backed
  discoveries made while doing the work, even when the documentation is not required for the code change. This
  needs no separate authorization. Keep [`how-tos/INDEX.md`](./how-tos/INDEX.md) consistent and report the updates.
- Guide edits arising from answer-only questions require separate authorization. Documentation capture records
  observed behavior; it does not authorize additional implementation or new product contracts. Commit and
  publication require authorization too.
- Prefer updating an existing guide over duplicating it. Keep recipes operator-facing: fetching or using Agent
  data. Developer contract validation for topology producers, schemas, fixtures, UI adapters, or aggregator
  handoffs belongs in the relevant project developer skill, not in this public skill.
