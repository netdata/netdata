# Query topology Functions via Netdata Cloud

This guide is part of the [`query-netdata-cloud`](./SKILL.md) skill.
Read the [SKILL.md prerequisites](./SKILL.md#prerequisites) first.
For the generic Function transport (used by topology, logs, flows,
and table-snapshot Functions alike), see
[query-functions.md](./query-functions.md).

Topology Functions return a graph: a list of **actors** (nodes in
the graph) and **links** (edges). They differ from log Functions
(which return a time-windowed skim of a larger dataset) and from
table-snapshot Functions (which return one full table).

---

## Function names registered today

Verified live and in source:

| Function | Source collector | Layer | What it discovers |
|---|---|---|---|
| `topology:snmp` | `src/go/plugin/go.d/collector/snmp_topology/` | L2 | LLDP/CDP-discovered switches+routers, FDB-derived endpoint locations, STP-derived parent-child relationships |

The `topology:` prefix is the canonical namespace; only `snmp` is
registered today (as of `STATUS_FILE_VERSION = 28`). When new
topology collectors land (network-viewer connections, streaming
parent-child, k8s service mesh, etc.), they will follow the same
`topology:<source>` naming pattern and the same response envelope
documented below. Always confirm via the function-listing endpoint
in [query-functions.md](./query-functions.md) before assuming a
given topology Function exists on a node.

---

## Endpoint and request

Use the standard Cloud Function-call endpoint. Topology is just a
Function; nothing is special about its URL.

`POST /api/v2/nodes/{nodeId}/function?function=topology:snmp`

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"

read -r -d '' PAYLOAD <<'EOF'
{
  "selections": {
    "nodes_identity":            ["mac"],
    "map_type":                  ["lldp_cdp_managed"],
    "inference_strategy":        ["fdb_minimum_knowledge"],
    "managed_snmp_device_focus": ["all_devices"],
    "depth":                     ["all"]
  },
  "timeout": 60000,
  "last":    200
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=topology:snmp" \
  -d "$PAYLOAD"
```

Always start with `{"info":true}` to discover the parameters the
node currently accepts -- topology Function parameter sets evolve
with the collector.

### Body parameters (topology:snmp)

Verified against `src/go/plugin/go.d/collector/snmp_topology/`:

| Parameter | Type | Allowed values | Purpose |
|---|---|---|---|
| `nodes_identity` | string | `ip`, `mac` | Collapse / distinguish actors by IP or MAC |
| `map_type` | string | `lldp_cdp_managed`, `high_confidence_inferred`, `all_devices_low_confidence` | Which discovery sources to include |
| `inference_strategy` | string | `fdb_minimum_knowledge`, `stp_parent_tree`, `fdb_pairwise_minimum_knowledge`, `stp_fdb_correlated`, `cdp_fdb_hybrid` | How endpoint-to-switch placement is inferred when LLDP/CDP coverage is incomplete |
| `managed_snmp_device_focus` | string | `all_devices`, `ip:<prefix>` | Restrict the discovery surface to a subset of managed SNMP devices |
| `depth` | string | `0`-`10`, or `all` | Hops away from the focus device to include |

`selections` is the standard Function selection object; values are
arrays even for single-valued parameters (Netdata convention).

---

## Response envelope

Topology Functions wrap their content in the standard Function
envelope. Top-level keys (verified live):

| Key | Description |
|---|---|
| `status` | HTTP-style status integer (200 on success) |
| `v` | Function schema version |
| `type` | **`topology`** -- the family discriminator |
| `help` | Human description |
| `accepted_params` | Parameter names the Function accepts |
| `required_params` | Per-parameter UI widgets |
| `has_history` | Whether the Function supports `after`/`before` history |
| `update_every` | Suggested refresh interval (seconds) |
| `data` | The graph payload (object) |

### `data` object

| Key | Description |
|---|---|
| `schema_version` | Topology schema version (e.g. `2.0`) |
| `source` | Discovery source string (e.g. `snmp`) |
| `layer` | OSI layer the topology lives at (e.g. `2`, `3`) |
| `agent_id` | Identifier of the producing agent |
| `collected_at` | RFC3339 timestamp |
| `view` | Rendered view kind (e.g. `summary`, `detail`) |
| `actors[]` | Graph nodes -- see schema below |
| `links[]` | Graph edges -- see schema below |
| `flows[]` | Optional, for sources that emit flow records alongside the topology |
| `stats` | Per-source counters (devices polled, fdb entries, etc.) |
| `metrics` | Optional metric block |
| `ip_policy` | Optional, when `nodes_identity:ip` is in effect |

### Actor record

| Key | Description |
|---|---|
| `actor_id` | Stable identifier of the actor; format is source-specific (e.g. `mac:<addr>[,<addr>...]` for SNMP at L2; `ip:<addr>` when collapsed by IP) |
| `actor_type` | e.g. `device`, `endpoint`, `vlan`, `service` |
| `layer` | OSI layer (`2`, `3`, ...) |
| `source` | Source collector (e.g. `snmp`) |
| `match` | Discovery facts (sysName, sysObjectID, OUI, ...) |
| `attributes` | Free-form per-actor properties |
| `derived` | Computed annotations (vendor inference, role, ...) |
| `labels` | Tag-style key/value pairs |
| `tables` | Per-actor sub-tables (e.g. interfaces, ARP entries) |

### Link record

| Key | Description |
|---|---|
| `layer` | Link layer |
| `protocol` | Discovery protocol (`lldp`, `cdp`, `fdb`, `stp`, ...) |
| `link_type` | Refinement (e.g. `lldp`, `inferred-fdb`, `stp-parent`) |
| `direction` | `bidirectional`, `forward`, `reverse` |
| `state` | Operational state (`up`, `down`, ...) |
| `src_actor_id` / `dst_actor_id` | The two endpoints (match an entry in `actors[]`) |
| `src` / `dst` | Per-end interface / port details |
| `discovered_at` / `last_seen` | RFC3339 timestamps |
| `metrics` | Optional per-link metrics |

---

## Examples

### Example 1: full LLDP/CDP topology

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"

read -r -d '' PAYLOAD <<'EOF'
{
  "selections": {
    "nodes_identity":            ["mac"],
    "map_type":                  ["lldp_cdp_managed"],
    "inference_strategy":        ["fdb_minimum_knowledge"],
    "managed_snmp_device_focus": ["all_devices"],
    "depth":                     ["all"]
  },
  "timeout": 60000
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=topology:snmp" \
  -d "$PAYLOAD" \
  | jq '.data | {actors: (.actors|length), links: (.links|length)}'
```

### Example 2: include FDB-derived low-confidence endpoints

```bash
read -r -d '' PAYLOAD <<'EOF'
{
  "selections": {
    "nodes_identity":            ["mac"],
    "map_type":                  ["all_devices_low_confidence"],
    "inference_strategy":        ["fdb_pairwise_minimum_knowledge"],
    "managed_snmp_device_focus": ["all_devices"],
    "depth":                     ["all"]
  },
  "timeout": 120000
}
EOF
```

### Example 3: focus on a single device + 2 hops

```bash
read -r -d '' PAYLOAD <<'EOF'
{
  "selections": {
    "nodes_identity":            ["ip"],
    "map_type":                  ["lldp_cdp_managed"],
    "inference_strategy":        ["stp_parent_tree"],
    "managed_snmp_device_focus": ["ip:YOUR_FOCUS_DEVICE_IP"],
    "depth":                     ["2"]
  },
  "timeout": 60000
}
EOF
```

### Example 4: discover supported parameters before querying

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"

read -r -d '' PAYLOAD <<'EOF'
{ "info": true }
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=topology:snmp" \
  -d "$PAYLOAD" \
  | jq '{accepted_params, required_params}'
```

---

## Limits and gotchas

- **Topology Functions are slow.** A full SNMP sweep can take tens
  of seconds. Bump `timeout` to 60-120 seconds.
- **`actor_id` for L2 SNMP topology can be a comma-separated list
  of MACs** -- when a single device exposes many MACs (one per
  port), they collapse into one actor with all MACs in
  `actor_id`. Use `nodes_identity:ip` to collapse by IP instead.
- **Privacy**: actor and link records carry MAC addresses, IP
  addresses, sysName strings, and SNMP descriptions. Treat as
  network-identifying data; do not paste raw responses into
  committed files. Use `<repo>/.local/audits/...` for working
  output (gitignored).
- **`info=true` is cheap and always available.** Use it to confirm
  the parameter set on a specific node before constructing a real
  query.
- **Topology Functions are agent-only sources.** Cloud only
  proxies; there is no Cloud-side aggregation across nodes for
  topology. For multi-agent topology composition, fetch each
  agent's response and merge client-side.
