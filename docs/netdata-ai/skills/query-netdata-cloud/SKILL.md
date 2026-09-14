---
name: query-netdata-cloud
description: Query or explain Netdata Cloud APIs for metrics, logs, topology, flows, alerts, DynCfg, Functions and discovery; review Cloud-query recipes. Use query-netdata-agents for direct Agent access.
---

# Query Netdata Cloud

Use this skill for Cloud REST queries and for reviewing or explaining those queries. Load the guide for the requested
domain; the sibling [Agent skill](../query-netdata-agents/SKILL.md) owns direct access and the shared query helpers.

## Choose The Task

- **Explain or review:** inspect the relevant guide and helper as reference material. Loading this skill does not
  authorize live requests, credential loading or configuration changes. An explanation need not end in a command.
- **Run a requested query:** follow [Prerequisites](#prerequisites) and [Safe Execution](#safe-execution), then the
  selected guide. Preserve authorization already given for the task; do not ask again for ordinary query steps.
- **Change configuration or invoke a state-changing Function:** establish that the user requested that operation.
  A request to inspect data does not authorize DynCfg updates or arbitrary Function execution. HTTP method alone does
  not establish whether a request changes state; consult the Function or action contract.

For an execution recipe, provide a complete runnable wrapper invocation with locally supplied inputs and the response
fields needed for the question. For discovery, use the endpoint appropriate to the missing identifier. Domain
requirements remain in their guides; metric queries MUST set `scope.contexts` as explained in the metrics guide.

## Domain Guides

| Domain | Guide |
|---|---|
| Time-series metrics | [query-metrics.md](./query-metrics.md) |
| Logs (`systemd-journal`, `windows-events`, `macos-logs`, `otel-logs`) | [query-logs.md](./query-logs.md) |
| Topology Functions (`topology:snmp`, ...) | [query-topology.md](./query-topology.md) |
| Network-flow Functions (`flows:netflow` -- NetFlow / sFlow / IPFIX) | [query-flows.md](./query-flows.md) |
| Alerts and alert transitions | [query-alerts.md](./query-alerts.md) |
| Dynamic Configuration (DynCfg) | [query-dyncfg.md](./query-dyncfg.md) |
| Generic Function invocation (table snapshots + protocol taxonomy) | [query-functions.md](./query-functions.md) |
| Nodes (per-room enumeration with full metadata) | [query-nodes.md](./query-nodes.md) |
| Rooms (per-space enumeration) | [query-rooms.md](./query-rooms.md) |
| Members (per-space user enumeration) | [query-members.md](./query-members.md) |
| Event feed (audit + activity log) | [query-feed.md](./query-feed.md) |
| **Operational how-tos (live catalog)** | [how-tos/INDEX.md](./how-tos/INDEX.md) |

## Prerequisites

Explanation and saved-data review need no credentials. Executable recipes require Bash, Git, curl, jq and a checkout
containing the shared helper, plus access to the chosen Cloud host. Configure `NETDATA_CLOUD_TOKEN` and
`NETDATA_CLOUD_HOSTNAME` locally; never request credential values in conversation. Use identifier placeholders or
local discovery for space, room and node IDs. Only endpoints that include those IDs require them.

[Access And Discovery](./access-and-discovery.md) covers token setup, identifier lookup and error diagnosis.
[Helper configuration](../query-netdata-agents/authentication.md#configuration) owns the environment keys and trusted
`.env` loading. Source the helper and load configuration only for authorized execution:

```bash
source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env
```

## Safe Execution

Credential-bearing requests MUST use `agents_query_cloud`, `agents_query_agent` or `agents_call_function` from the
shared helper. Do not construct raw bearer-auth curl examples or print credentials. Ordinary local processing and
unauthenticated probes MAY use ordinary commands when they capture or suppress sensitive responses.

[Shared Safe Execution](../query-netdata-agents/SKILL.md#safe-execution) owns credential and response handling.
The wrappers mask known request-auth values in their command log; they **forward responses unchanged** and do not
scrub arbitrary request-body fields. Keep sensitive bodies in private local files (the body argument accepts `@file`)
and capture or project sensitive responses before display. Do not display credential-issuing endpoint output through
these wrappers. Avoid shell tracing around credentials. Use the minimum response fields needed for the task.

For JSON bodies, use a quoted heredoc in command substitution, or a private payload file. The helper enables
`set -e`; an empty-delimiter `read` returns failure at EOF and stops the recipe. For example, after setup:

```bash
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"
PAYLOAD="$(cat <<'JSON'
{}
JSON
)"
agents_query_cloud POST \
  "/api/v3/spaces/$SPACE/rooms/$ROOM/nodes" "$PAYLOAD" \
  | jq '{node_count: (.nodes | length)}'
```

Cloud payloads can contain customer identifiers, hostnames, labels, addresses and secrets. Raw responses MUST NOT go
into committed files; token, bearer and session values MUST NOT be pasted into conversation or shared artifacts.
Repository workflows capture raw output privately under `<repo>/.local/audits/` and report sanitized summaries.
Repository contributors follow `.agents/sensitive-data-discipline.md`; outside a checkout apply the same redaction to
shared artifacts. The public helpers' exact interfaces live in
[Helper Interfaces](../query-netdata-agents/authentication.md#helper-interfaces).

## Protocol Owners

Open these when the selected task needs the underlying protocol or schema. They describe Agent payloads and DynCfg,
not Cloud-server permission mappings.

| Owner | Subject |
|---|---|
| `<repo>/src/plugins.d/FUNCTION_UI_REFERENCE.md` | Function envelopes, tables, logs, facets, pagination, PLAY and errors |
| `<repo>/src/plugins.d/FUNCTION_UI_SCHEMA.json` | Function response schema |
| `<repo>/src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` | Topology response schema |
| `<repo>/src/plugins.d/DYNCFG.md` | External-plugin DynCfg protocol |
| `<repo>/src/daemon/dyncfg/README.md` | Internal DynCfg APIs and lifecycle |

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
- Prefer updating an existing guide over duplicating it. Keep recipes operator-facing: fetching or using Cloud
  data. Developer contract validation for collectors, topology producers, schemas, fixtures, UI adapters, or
  aggregator handoffs belongs in the relevant project developer skill, not in this public skill.
