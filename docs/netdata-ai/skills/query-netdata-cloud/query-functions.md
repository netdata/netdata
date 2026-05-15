# Query agent Functions via Netdata Cloud

This guide is part of the [`query-netdata-cloud`](./SKILL.md) skill.
Read the [SKILL.md prerequisites](./SKILL.md#prerequisites) first.

This file documents the **generic** Function transport: the URL,
the standard response envelope, the `info` discovery query, the
four Function families and where each family's data lives in the
response, plus pointers to the developer documentation for
collector authors.

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

1. **Provide actionable instructions.** Every recommendation ends
   in a runnable curl command.
2. **Never request credentials.** Use `YOUR_API_TOKEN` and
   `YOUR_NODE_UUID` placeholders.
3. **Always start with `{"info":true}`** when you don't already
   know the parameter set of the target Function. The `info`
   response is authoritative -- this skill's tables can be stale
   relative to the running agent.
4. **Function names are case-sensitive** (e.g. `systemd-journal`,
   `topology:snmp`, `flows:netflow`).

---

## Function classes

The canonical Functions v3 protocol
(`<repo>/src/plugins.d/FUNCTION_UI_REFERENCE.md`) formally defines
**two** Function classes, distinguished by the `has_history` flag
in the `info` response:

| Class | `has_history` | Frontend behavior | Examples |
|---|---|---|---|
| **Simple Table** | `false` | Backend returns the whole current dataset; frontend filters/sorts/searches in-memory | `processes`, `network-connections`, `network-interfaces`, `network-sockets-tracing`, `block-devices`, `mount-points`, `containers-vms`, `systemd-services`, `netdata-streaming`, `netdata-api-calls`, `netdata-metrics-cardinality`, `<db>:top-queries`, `<db>:running-queries`, `<db>:deadlock-info`, `<db>:error-info` |
| **Log Explorer** | `true` | Backend filters / facets / histograms before sending; supports infinite scroll, anchor pagination, delta and PLAY modes | `systemd-journal`, `windows-events`, `otel-logs` |

Two additional `type` values are used by purpose-built Functions
that build on the same envelope but emit non-tabular `data`:

| `type` | Response shape | Examples | Guide |
|---|---|---|---|
| `topology` | `data.actors[]` + `data.links[]` (a graph) | `topology:snmp` | [query-topology.md](./query-topology.md) |
| `flows` | `data.flows[]` plus `data.facets` / `data.columns` / `data.stats` over a time window | `flows:netflow` (covers NetFlow / sFlow / IPFIX) | [query-flows.md](./query-flows.md) |

For full protocol semantics (facet pills, histograms, charts
configuration, anchor/delta/PLAY modes, error handling, edge
cases), the authoritative source is
`<repo>/src/plugins.d/FUNCTION_UI_REFERENCE.md`. This skill
summarizes the surface that matters for a Cloud-side curl client;
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

The single most important call to make before constructing a real
query: pass `{"info": true}` and read `accepted_params` plus
`required_params`. The agent itself is the authoritative source --
if a parameter exists there, the Function accepts it; if it
doesn't, no other doc matters.

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"
FN="systemd-journal"

read -r -d '' PAYLOAD <<'EOF'
{ "info": true }
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=$FN" \
  -d "$PAYLOAD"
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
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "scope":     { "nodes": [] },
  "selectors": { "nodes": ["*"] }
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v3/spaces/$SPACE/rooms/$ROOM/functions" \
  -d "$PAYLOAD"
```

Response top-level: `functions[]` (each entry: `name`, `version`,
`help`, `ni[]`, `tags`, `access[]`, `priority`), `nodes[]` (each
`{ ni, mg, nd, nm, st }`), `agents[]`, `versions`. Match
`functions[].ni` to `nodes[].ni` to find which nodes expose a
given Function.

### Invoke a Function on a node

`POST /api/v2/nodes/{nodeId}/function?function={functionName}`

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"
FN="processes"

read -r -d '' PAYLOAD <<'EOF'
{
  "last":    50,
  "timeout": 30000
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=$FN" \
  -d "$PAYLOAD"
```

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
| `network-sockets-tracing` | table | Detailed open-socket information |
| `block-devices` | table | Per-block-device read/write throughput, ops, latency, utilization |
| `mount-points` | table | Filesystem mount points with space and inode usage |
| `containers-vms` | table | Active containers and cgroups with resource usage |
| `systemd-services` | table | systemd service cgroups with process counts and resource use |
| `netdata-streaming` | table | Parent-child streaming/replication status, data-flow metrics, ML status |
| `netdata-api-calls` | table | Active and recent Netdata API requests with timings |
| `netdata-metrics-cardinality` | table | Cardinality stats (instances, time-series per context/node) |
| `systemd-journal` | logs | systemd journal entries -- see [query-logs.md](./query-logs.md) |
| `windows-events` | logs | Windows event log channels (Windows nodes only) |
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

### Example 1: top processes by CPU

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"

read -r -d '' PAYLOAD <<'EOF'
{
  "last":    50,
  "timeout": 30000
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=processes" \
  -d "$PAYLOAD" \
  | jq '.data | length, (.[0:3])'
```

### Example 2: discover a Function's parameter widget set

```bash
read -r -d '' PAYLOAD <<'EOF'
{ "info": true }
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=network-connections" \
  -d "$PAYLOAD" \
  | jq '.required_params | map({id, type, name, options: (.options | length // 0)})'
```

### Example 3: list the Functions on a single node

```bash
read -r -d '' PAYLOAD <<'EOF'
{
  "scope":     { "nodes": ["YOUR_NODE_UUID"] },
  "selectors": { "nodes": ["*"] }
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v3/spaces/$SPACE/rooms/$ROOM/functions" \
  -d "$PAYLOAD" \
  | jq -r '.functions[] | "\(.name)\t\(.tags // "")\t\(.help)"'
```

---

## Developer reference (for collector authors)

If you maintain a collector and want to register a Function (or are
debugging why a Function returns `400 ErrInfoMissing`), read these
files in this order. They are the authoritative sources.

| File | Audience | Read for |
|---|---|---|
| `<repo>/src/plugins.d/FUNCTION_UI_REFERENCE.md` | All implementers | Functions v3 protocol -- envelope, simple-table vs log-explorer, facets, histograms, charts, field types, anchor/delta/PLAY pagination, error handling, edge cases. **The single most important reference.** |
| `<repo>/src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md` | Collector authors | Practical step-by-step: how to ship a simple-table or log-explorer Function, with backend examples |
| `<repo>/src/plugins.d/FUNCTION_UI_SCHEMA.json` | Validation | JSON Schema for Function responses; use it in unit tests |
| `<repo>/src/plugins.d/README.md` (sections 470-637) | External plugins (any language) | Plugin protocol, `FUNCTION` / `FUNCTION_PAYLOAD` parsing, response framing, ACL, lifecycle |
| `<repo>/src/plugins.d/DYNCFG.md` | External plugins exposing config | DynCfg protocol for go.d.plugin and other external collectors |
| `<repo>/src/daemon/dyncfg/README.md` | Internal plugins exposing config | Internal DynCfg API |
| `<repo>/src/database/rrdfunctions.h` | C collectors | C API: `rrd_function_add(host, st, name, timeout, priority, version, help, tags, access, sync, execute_cb, data)`; handler signature `rrd_function_execute_cb_t` |
| `<repo>/src/go/plugin/framework/functions/README.md` | Go.d.plugin collectors | Go function manager: `manager.Register`, handler lifecycle, cancellation, worker pool |
| `<repo>/docs/functions/` | Operators | Per-Function user docs and examples |

Skeleton signatures (extracted from the headers above, for
orientation only -- read the source for the real contract):

```c
/* C: register at boot, then emit responses via a buffer in the callback */
void rrd_function_add(
    RRDHOST *host, RRDSET *st,
    const char *name,           /* "module:method" */
    int timeout, int priority, uint32_t version,
    const char *help, const char *tags,
    HTTP_ACCESS access, bool sync,
    rrd_function_execute_cb_t execute_cb, void *execute_cb_data);

typedef int (*rrd_function_execute_cb_t)(
    struct rrd_function_execute *rfe, void *data);
```

```go
// Go: register a Function with the framework's manager
manager.Register(functions.Function{
    Name:        "module:method",
    Description: "Help text",
    Timeout:     30 * time.Second,
    Params:      []ParamDescriptor{ /* maps to required_params */ },
    Handler:     func(fn Function) { /* emit FuncResponse */ },
})
```

```text
# External plugins via the plugins.d protocol:
FUNCTION [GLOBAL] "name params" timeout "help" "tags" "access" priority version
# On call:
FUNCTION <txn_id> <timeout> "name params" "<access>" "<source>"
# Reply:
FUNCTION_RESULT_BEGIN <txn_id> <http_code> <content_type> <expiry>
<JSON envelope: status, v, type, help, accepted_params, required_params, has_history, update_every, data, ...>
FUNCTION_RESULT_END
```

The full envelope and required-params widget schemas above ARE the
contract a collector implementation must satisfy. Read
`src/plugins.d/README.md` for the line-protocol details and
`src/database/rrdfunctions.h` for the C API.

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
- **Node must be `reachable`.** A `stale` node returns HTTP 400
  with `errorMsgKey: "ErrInstanceNotReachable"`. Verify with the
  discovery endpoints in [SKILL.md](./SKILL.md).
- **Permission**: the cloud token must include
  `PermissionFunctionExec` on the target space. `scope:all`
  works; `scope:grafana-plugin` does NOT.
- **Function name is case-sensitive** -- wrong casing returns 400.
- **`info=true` does NOT bypass auth.** ACL is enforced on every
  call regardless of body.
- **The agent's own `info=true` response is authoritative for
  parameters.** Tables in this skill can drift relative to the
  running version. When in doubt, ask the agent.
