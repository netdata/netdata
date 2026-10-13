# Query agent Functions via Netdata Cloud

This guide is part of the [`query-netdata-cloud`](./SKILL.md) skill.
Read the [SKILL.md prerequisites](./SKILL.md#prerequisites) first.

This file documents the **generic** Function transport: the URL,
the standard response envelope, the `info` discovery query, the
four Function families and where each family's data lives in the
response.

For three of the four families there is a dedicated guide:

- **Logs** family (table-history with facets+histogram):
  [query-logs.md](./query-logs.md)
- **Topology** family (graph: actors+links):
  [query-topology.md](./query-topology.md)
- **Flows** family (network-flow records):
  [query-flows.md](./query-flows.md)

The **table-snapshot** family (full dataset in each response) is
covered here.

For querying agents directly (without going through Cloud) -- which
includes the transparent Cloud-token to agent-bearer mint flow --
see the sibling skill
[`query-netdata-agents`](../query-netdata-agents/SKILL.md).

---

## Mandatory Requirements (READ FIRST)

Follow [Choose The Task](./SKILL.md#choose-the-task) for explain, review and execution requests, and
[Safe Execution](./SKILL.md#safe-execution) for local setup, credentials and response handling.

- For a known read-only Function that supports `info`, use `{"info":true}` to discover its current parameters.
  Confirm support and behavior before invoking an unfamiliar Function; `info` is not a universal safety switch.
- **Function names are case-sensitive** (e.g. `systemd-journal`, `topology:snmp`, `flows:netflow`).

---

## Function classes

The canonical Functions v3 protocol
(`<repo>/src/plugins.d/FUNCTION_UI_REFERENCE.md`) formally defines
**two** Function classes, distinguished by the `has_history` flag
in the `info` response:

| Class | `has_history` | Frontend behavior | Examples |
|---|---|---|---|
| **Simple Table** | `false` | Backend returns the whole current dataset; frontend filters/sorts/searches in-memory | `processes`, `network-connections`, `network-interfaces`, `block-devices`, `mount-points`, `containers-vms`, `systemd-services`, `netdata-streaming`, `netdata-api-calls`, `netdata-metrics-cardinality`, `<db>:top-queries`, `<db>:running-queries`, `<db>:deadlock-info`, `<db>:error-info` |
| **Log Explorer** | `true` | Backend filters / facets / histograms before sending; supports infinite scroll, anchor pagination, delta and PLAY modes | `systemd-journal`, `windows-events`, `macos-logs`, `otel-logs` |

Two additional `type` values are used by purpose-built Functions
that build on the same envelope but emit non-tabular `data`:

| `type` | Response shape | Examples | Guide |
|---|---|---|---|
| `topology` | `data.actors`/`data.links` graph plus compact-schema sections (`data.evidence`, `data.tables`, `data.overlays`) | `topology:network-connections`, `topology:streaming`, `topology:snmp` | [query-topology.md](./query-topology.md) |
| `flows` | `data.flows[]` plus `data.facets` / `data.columns` / `data.stats` over a time window | `flows:netflow` (covers NetFlow / sFlow / IPFIX) | [query-flows.md](./query-flows.md) |

For full protocol semantics (facet pills, histograms, charts
configuration, anchor/delta/PLAY modes, error handling, edge
cases), the authoritative source is
`<repo>/src/plugins.d/FUNCTION_UI_REFERENCE.md`. This skill
summarizes the surface that matters for a Cloud API client;
the reference covers everything else.

---

## Standard response envelope

Every Function -- regardless of family -- wraps its output in this
envelope. Verified live against the agent's `systemd-journal`,
`topology:snmp`, and `flows:netflow` Functions, and against the
agent emit code at
`src/web/api/functions/function-metrics-cardinality.c:26-39,92`
plus per-collector wrappers.

| Key | Type | Required | Notes |
|---|---|---|---|
| `status` | int | yes | HTTP-style status (200, 400, ...) |
| `v` | int | yes | Function schema version (currently `3` or `4` depending on Function) |
| `type` | string | yes | Family discriminator: `table`, `logs`, `topology`, `flows` (some Functions emit a custom string -- treat unknown values as `table`-like) |
| `help` | string | typical | Human-readable description |
| `accepted_params` | array<string> | typical | Parameter names accepted in the body |
| `required_params` | array<object> | typical | Per-parameter widget descriptors -- see "info=true discovery" below |
| `has_history` | bool | typical | Whether the Function honors `after` / `before` |
| `update_every` | int | typical | Suggested refresh interval in seconds |
| `data` | array OR object | conditional | Family-specific result. **Absent on `info=true` calls and on errors.** Array for `logs` and `table` families; object (with `actors`/`links` or `flows`/`columns`/`stats`) for `topology` and `flows` |
| `columns` | object | logs / table | Column-metadata, keyed by column name. Each entry has `index` (position inside each row of `data`), `name`, `type`, `visible`, `sort`, `summary`, `filter`, ... |
| `facets` | array | logs / flows | Per-field value distribution and option counts |
| `histogram` | object | logs (when requested) | Bucketed counts over time |
| `pagination` | object | logs | `anchor`, `direction`, `last`, etc. |
| `presentation` | object | topology / flows | Visualization metadata for the Cloud UI |
| `expires` / `last_modified` / `partial` / `message` | scalar | optional | Caching, freshness, partial-result diagnostics |
| `versions` | object | optional | Source/version hashes for client cache invalidation |

`status >= 400` responses follow the same envelope but include an
`errorMessage` / `errorMsgKey` instead of `data`.

---

## `info=true` discovery

For a read-only Function that supports discovery, pass `{"info": true}` and inspect `accepted_params` and
`required_params`. Use the running Function's metadata and implementation contract to construct its request.
An arbitrary Function may ignore `info` or perform an operation; establish its behavior before calling it.

```bash
source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env
NODE="YOUR_NODE_UUID"
FN="systemd-journal"

PAYLOAD="$(cat <<'EOF'
{ "info": true }
EOF
)"

agents_query_cloud POST \
  "/api/v2/nodes/$NODE/function?function=$FN" \
  "$PAYLOAD"
```

### `required_params` widget schema

Each entry of `required_params` is a UI-widget descriptor that
tells a client what to render and what values are valid. Verified
against the emit code in
`src/collectors/network-viewer.plugin/network-viewer.c:1601-1731`
and across the topology / logs / flows Functions.

| Field | Type | Required | Purpose |
|---|---|---|---|
| `id` | string | yes | Parameter id (the body key) |
| `name` | string | yes | Display label |
| `help` | string | typical | Tooltip / help text |
| `type` | string | yes | Widget kind -- see table below |
| `options[]` | array | for select/multiselect/autocomplete | Each option: `{ "id": "<value>", "name": "<label>", "defaultSelected": <bool>? }` |
| `unique_view` | bool | optional | Single-select enforces single-value semantics |
| `multiselect` | bool | optional | Multi-value semantics |
| `pattern` | string | optional | Regex/glob input for `text`/`pattern` widgets |
| `default_value` | scalar | optional | Pre-filled value |

Widget `type` values seen in source:

| `type` | Meaning |
|---|---|
| `select` | Single-choice dropdown |
| `multiselect` | Multi-choice; each option may have `defaultSelected:true` |
| `autocomplete` | Text input backed by an autocomplete query (the Function itself answers via `mode:"autocomplete"` or similar) |
| `text` | Free-form text |
| `checkbox` | Boolean toggle |
| `range` | Numeric range / slider (newer Functions) |
| `pattern` | Pattern / regex input (newer Functions) |

The widget array is the contract between the agent and any UI or
script. To programmatically construct a valid body for a Function,
walk `required_params` and emit the body shape it implies. There
is no central widget builder API in source -- each collector emits
the array directly via `buffer_json_*` calls -- so the agent's own
`info=true` response is the only authoritative place to read the
schema for a specific node version.

---

## Endpoints

### List Functions on the nodes in a room

`POST /api/v3/spaces/{spaceID}/rooms/{roomID}/functions`

```bash
source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

PAYLOAD="$(cat <<'EOF'
{
  "scope":     { "nodes": [] },
  "selectors": { "nodes": ["*"] }
}
EOF
)"

agents_query_cloud POST \
  "/api/v3/spaces/$SPACE/rooms/$ROOM/functions" \
  "$PAYLOAD"
```

Response top-level: `functions[]` (each entry: `name`, `version`,
`help`, `ni[]`, `tags`, `access[]`, `priority`), `nodes[]` (each
`{ ni, mg, nd, nm, st }`), `agents[]`, `versions`. Match
`functions[].ni` to `nodes[].ni` to find which nodes expose a
given Function.

### Invoke a Function on a node

`POST /api/v2/nodes/{nodeId}/function?function={functionName}`

```bash
source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env
NODE="YOUR_NODE_UUID"
FN="processes"

PAYLOAD="$(cat <<'EOF'
{}
EOF
)"

agents_query_cloud POST \
  "/api/v2/nodes/$NODE/function?function=$FN" \
  "$PAYLOAD"
```

The `processes` Function returns a current snapshot. Its handler
(`src/collectors/apps.plugin/apps_functions.c:function_processes`) parses arguments from the Function string and
ignores the JSON payload; `last` and `timeout` in that body do not limit or sort its rows. Use its Function contract
for arguments; its info request uses `function=processes%20info` with an empty JSON body.

Optional headers:

| Header | Purpose |
|---|---|
| `X-Transaction-Id: <uuid>` | Correlation id propagated to the agent. Optional. |

---

## Frequently registered Functions

Function availability is per-node. The listing endpoint above is
the only authoritative source. Below are common Functions on a
stock Linux Netdata install (verified live):

| Function | Family | What it returns |
|---|---|---|
| `processes` | table | Live process list with CPU / memory / I/O / page faults / PPID |
| `network-connections` | table | Active sockets/connections (proto, state, addresses, ports, perf metrics) |
| `network-interfaces` | table | Per-interface traffic, packet counts, drops, link status |
| `block-devices` | table | Per-block-device read/write throughput, ops, latency, utilization |
| `mount-points` | table | Filesystem mount points with space and inode usage |
| `containers-vms` | table | Active containers and cgroups with resource usage |
| `systemd-services` | table | systemd service cgroups with process counts and resource use |
| `netdata-streaming` | table | Parent-child streaming/replication status, data-flow metrics, ML status |
| `netdata-api-calls` | table | Active and recent Netdata API requests with timings |
| `netdata-metrics-cardinality` | table | Cardinality stats (instances, time-series per context/node) |
| `systemd-journal` | logs | systemd journal entries -- see [query-logs.md](./query-logs.md) |
| `windows-events` | logs | Windows event log channels (Windows nodes only) |
| `macos-logs` | logs | macOS unified log entries (macOS nodes only) |
| `otel-logs` | logs | OpenTelemetry log entries (when the OTEL log receiver is enabled) |
| `topology:snmp` | topology | LLDP/CDP/FDB/STP-derived L2 topology -- see [query-topology.md](./query-topology.md) |
| `flows:netflow` | flows | NetFlow / sFlow / IPFIX records -- see [query-flows.md](./query-flows.md) |

Database collectors register a per-collector family of Functions
when active: `<collector>:top-queries`, `<collector>:running-queries`,
`<collector>:deadlock-info`, `<collector>:error-info` -- e.g.
`postgres:top-queries`, `mysql:top-queries`, `mssql:deadlock-info`.
The listing endpoint reports them when the collector is enabled.

---

## Examples (table-snapshot Functions)

For logs / topology / flows examples, see the per-family guides
linked at the top.

### Example 1: current processes snapshot

```bash
source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env
NODE="YOUR_NODE_UUID"

PAYLOAD="$(cat <<'EOF'
{}
EOF
)"

agents_query_cloud POST \
  "/api/v2/nodes/$NODE/function?function=processes" \
  "$PAYLOAD" \
  | jq '.data | length, (.[0:3])'
```

### Example 2: discover a Function's parameter widget set

```bash
PAYLOAD="$(cat <<'EOF'
{ "info": true }
EOF
)"

agents_query_cloud POST \
  "/api/v2/nodes/$NODE/function?function=network-connections" \
  "$PAYLOAD" \
  | jq '.required_params | map({id, type, name, options: (.options | length // 0)})'
```

### Example 3: list the Functions on a single node

```bash
PAYLOAD="$(cat <<'EOF'
{
  "scope":     { "nodes": ["YOUR_NODE_UUID"] },
  "selectors": { "nodes": ["*"] }
}
EOF
)"

agents_query_cloud POST \
  "/api/v3/spaces/$SPACE/rooms/$ROOM/functions" \
  "$PAYLOAD" \
  | jq -r '.functions[] | "\(.name)\t\(.tags // "")\t\(.help)"'
```

---

## Limits and gotchas

- **Cloud default timeout is 120 s** for Function calls; pass
  `"timeout": <ms>` in the body for slower Functions but Cloud
  may impose its own ceiling.
- **Response is NOT streamed.** The Cloud proxy collects the full
  agent response and returns it in one body. For potentially
  huge results (logs, flows), narrow the time window or use the
  Function's pagination (`last`, `anchor`) rather than relying on
  streaming.
- **Reachability:** historical Cloud observations associate stale nodes with HTTP 400 and
  `errorMsgKey: "ErrInstanceNotReachable"`. Check current node discovery and the actual error response.
- **Permission:** historical guidance names `PermissionFunctionExec` and distinguishes `scope:all` from
  `scope:grafana-plugin`. Exact Cloud gates and status mappings are not verified against a current server owner;
  check target-space role, token scope and endpoint restrictions before changing access.
- **Function names are case-sensitive.** Wrong casing can cause a request failure.
- **`info=true` does NOT bypass auth.** ACL is enforced on every
  call regardless of body.
- **Use the running Function's supported discovery mechanism and contract.** Tables can drift relative to the
  running version; follow [the discovery guidance](#infotrue-discovery) for that Function.
