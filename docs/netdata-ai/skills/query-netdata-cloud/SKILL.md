---
name: query-netdata-cloud
description: Query Netdata Cloud via its REST API -- metrics, logs (systemd-journal / windows-events / otel-logs), topology graphs (topology:snmp), network flows (flows:netflow), alerts, dynamic configuration (DynCfg), and generic Functions on a node. Use when the user asks about querying Netdata Cloud, fetching metrics from the cloud, querying logs / topology / netflow / sflow / ipfix through Cloud, listing or modifying configurations via DynCfg, calling agent Functions through Cloud, listing spaces/rooms/nodes, or building a curl command against `app.netdata.cloud`. Pairs with the `query-netdata-agents` skill when direct-agent access is needed.
---

# Query Netdata Cloud via REST API

This skill teaches end-users (and AI assistants helping them) how to
construct REST API queries against Netdata Cloud
(`https://app.netdata.cloud`) using a long-lived API token.

It is split into one shared overview (this file) and four
domain-specific guides. Each guide is self-contained and includes
runnable curl commands.

| Domain | Guide |
|---|---|
| Time-series metrics | [query-metrics.md](./query-metrics.md) |
| Logs (`systemd-journal`, `windows-events`, `otel-logs`) | [query-logs.md](./query-logs.md) |
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
| **Verification questions (consumed by SOW-0006 harness)** | [verify/questions.md](./verify/questions.md) |

### Canonical reference docs (in this repo)

For the protocol-level details these guides build on, read the
authoritative sources directly:

| File | What it covers |
|---|---|
| `<repo>/src/plugins.d/FUNCTION_UI_REFERENCE.md` | Functions v3 protocol -- envelope, simple-table vs log-explorer, facets, histograms, charts, field types, pagination, delta mode, PLAY mode, error handling. The single most important reference for any Function work. |
| `<repo>/src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md` | Practical guide for collector authors implementing a Function (simple-table or log-explorer) |
| `<repo>/src/plugins.d/FUNCTION_UI_SCHEMA.json` | JSON Schema for validating Function responses |
| `<repo>/src/plugins.d/DYNCFG.md` | External-plugin DynCfg protocol (go.d.plugin and other external collectors) |
| `<repo>/src/daemon/dyncfg/README.md` | Internal DynCfg (high-level and low-level APIs, command enums, lifecycle) |
| `<repo>/src/database/rrdfunctions.h` | C-level Function registration API (`rrd_function_add`) |
| `<repo>/src/go/plugin/framework/functions/README.md` | Go-plugin Function framework |

For querying agents directly (without going through Cloud) -- including
auto-minting agent bearer tokens from a Cloud token -- see the sibling
skill [`query-netdata-agents`](../query-netdata-agents/SKILL.md).

---

## Mandatory Requirements (READ FIRST)

1. **If you analyze, you author a how-to.** When asked a concrete
   question about a Netdata environment that isn't already covered
   by an existing how-to under [`how-tos/`](./how-tos/), you MUST
   author a new how-to in this directory and add it to
   [`how-tos/INDEX.md`](./how-tos/INDEX.md) BEFORE completing the
   task. The catalog is meant to be **live** -- the next assistant
   should not redo the same analysis from scratch.
2. **Use the token-safe wrappers.** Every example in this skill
   uses `agents_query_cloud` (and friends) from
   `../query-netdata-agents/scripts/_lib.sh`. Never paste raw
   `Authorization: Bearer $TOKEN` curl commands -- that exposes
   the cloud token to the assistant. The wrappers handle auth
   internally and emit only the response body to stdout.
3. **Provide actionable instructions.** You don't run queries for
   users. Your role is to teach them. Every response that proposes a
   query must end in a complete, runnable command (wrapper-based,
   not raw curl).

2. **Never ask for credentials.** Do not request API tokens, Space
   IDs, or Room IDs. Use placeholders (`YOUR_API_TOKEN`,
   `YOUR_SPACE_ID`, `YOUR_ROOM_ID`) at the top of your curl examples
   so the user fills them in locally.

3. **Always include a runnable curl command.** A response without a
   complete `curl -X METHOD ... -H ... -d '...'` block is incomplete.
   Use a heredoc for the JSON body so the user does not have to
   escape quotes:

   ```bash
   read -r -d '' PAYLOAD <<'EOF'
   { "scope": { "contexts": ["system.cpu"] }, ... }
   EOF
   ```

4. **Domain-specific gotchas live in the per-domain guide.** For
   metrics, the most important is `scope.contexts` MUST be set. See
   the per-domain guide for the rest.

---

## Prerequisites

Three things are needed for any query:

### 1. API Token

1. Login to [app.netdata.cloud](https://app.netdata.cloud)
2. Click the user icon (lower-left corner -- tooltip shows your name)
3. Select **User Settings**
4. Open the **API Tokens** tab
5. Click the **[+]** button (top-left)
6. Pick a scope, enter a description, click **Create**
7. **Copy the token immediately** -- it is shown once.

Recommended scope: `scope:all` (full access) or `scope:grafana-plugin`
(read-only data endpoints).

### 2. Space ID

1. In the dashboard, click the **gear icon** below the spaces list
   (tooltip: "Space Settings")
2. In the **Info** tab, copy the **Space Id**.

### 3. Room ID

1. In Space Settings, open the **Rooms** tab
2. Click the **>** icon at the right of the row (tooltip: "Room
   Settings")
3. In the **Room** tab, copy the **Room Id**.

---

## Authentication

All endpoints accept the cloud token as an HTTP `Authorization`
header:

```
Authorization: Bearer YOUR_API_TOKEN
Content-Type: application/json   (for POST endpoints)
```

GET endpoints do not require the `Content-Type` header but accept it.

---

## Discovery Endpoints (used by every domain)

These endpoints enumerate what the cloud token can see. Use them when
you don't know the Space ID, Room ID, or node UUID up front.

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/v2/accounts/me` | GET | Confirm the token works; returns the user identity. |
| `/api/v2/spaces` | GET | List spaces visible to this token. |
| `/api/v2/spaces/{spaceID}/rooms` | GET | List rooms in a space. |
| `/api/v3/spaces/{spaceID}/rooms/{roomID}/nodes` | POST `{}` | List nodes in a room with full metadata. |

### Example: list spaces

```bash
TOKEN="YOUR_API_TOKEN"

curl -sS \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/spaces"
```

Each space record contains `id`, `slug`, `name`, `permissions[]`, and
metadata. Match by `name` or `slug` to find the space you want.

### Example: list rooms in a space

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"

curl -sS \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/spaces/$SPACE/rooms"
```

### Example: list nodes in a room

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v3/spaces/$SPACE/rooms/$ROOM/nodes" \
  -d '{}'
```

Per-node response fields:

| Field | Description |
|---|---|
| `nd` | Node UUID -- required for any node-targeted call |
| `mg` | Machine GUID |
| `nm` | Hostname |
| `state` | `reachable` (live) or `stale` (disconnected) |
| `v` | Agent version |
| `labels` | Key-value labels |
| `hw`, `os`, `health`, `capabilities` | Metadata blocks |

The `nd` value is what the four domain guides call "node UUID" or
`{nodeId}` in their endpoint paths.

---

## Common errors

| Symptom | Likely cause |
|---|---|
| HTTP 401 | Token missing, malformed, or revoked. Re-create. |
| HTTP 403 | Token lacks the scope/role for this endpoint or space. |
| HTTP 404 with HTML body | Wrong path; check method (GET vs POST) and version (`/api/v2` vs `/api/v3`). The API does not enumerate paths via Swagger, so 404 means the path does not exist. |
| HTTP 400 with `errorCode` JSON | Missing required parameter. The error message names the missing field. |
| Empty/silent response | Filter excludes everything. Most endpoints return empty data without error. Verify scope/selectors. |

---

## Sensitive data

Cloud responses contain space names, node hostnames, machine GUIDs,
node UUIDs, claim IDs, cloud-provider labels, IP addresses, and other
identifiers. Treat fetched payloads as personal/customer data:

- Do not paste raw response bodies into committed files.
- Do not paste tokens, bearer values, or session ids anywhere.
- For maintainer workflows in this repository, redirect raw output
  to `<repo>/.local/audits/...` (gitignored) and report only
  sanitized summaries upstream.

See `<repo>/.agents/sow/specs/sensitive-data-discipline.md` for the
full rule and the pre-commit verification grep.
