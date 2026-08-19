# Query topology Functions via Netdata Cloud

This guide is part of the [`query-netdata-cloud`](./SKILL.md) skill.
Read the [SKILL.md prerequisites](./SKILL.md#prerequisites) first.
For the generic Function transport, see [query-functions.md](./query-functions.md).

Topology Functions return compact graph payloads using the production topology
schema:

- [FUNCTION_TOPOLOGY_SCHEMA.json](../../../../src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json)

The response contains actors, graph links, relationship evidence, optional
actor detail tables, and optional telemetry overlay refs. Large sections use
compact columnar tables.

## Function namespace

Topology Functions use the `topology:<source>` namespace.

Known producer families:

| Function | Source | Typical topology |
|---|---|---|
| `topology:network-connections` | Network Viewer plugin | process, endpoint, socket evidence |
| `topology:streaming` | Netdata streaming subsystem | parent/child streaming graph |
| `topology:snmp` | SNMP topology collector | L2 devices, interfaces, endpoints, adjacencies |
| `topology:vsphere` | vSphere collector (planned) | inventory and virtualization relationships |

Always start with an info request to discover the parameters supported by the
Agent version you are querying.

## Network-connections grouping

`topology:network-connections` supports three actor grouping levels:

- `group_by:process_name` returns grouped process-name actors.
- `group_by:pid` returns one process actor per PID and is the only view that
  emits raw fields such as PID, UID, command line, cgroup path, and detailed
  container metadata.
- `group_by:container` returns container actors grouped by canonical
  `container_name`. Services use the service name, and non-container,
  non-service processes fall back to process name.

The payload advertises these `view.group_by` ids: `process_name`, `pid`, and
`container`.

Useful request arguments:

- `group_by:pid` returns per-PID process actors.
- `group_by:container` returns container/service actors.
- `labels:<pattern>` allows optional free-form actor labels. Omit it to hide
  free-form labels. Tokens are pipe-separated, for example
  `labels:team|app|version-*`; commas are literal.

## Endpoint

Use the standard Cloud Function endpoint:

`POST /api/v2/nodes/{nodeId}/function?function=topology:<source>`

Example info request:

```bash
NODE="YOUR_NODE_UUID"

source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

agents_call_function \
  --via cloud \
  --node "$NODE" \
  --function 'topology:network-connections' \
  --body '{"info":true,"timeout":30000}'
```

Example data request:

```bash
NODE="YOUR_NODE_UUID"

source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

read -r -d '' PAYLOAD <<'EOF'
{
  "selections": {
    "mode": ["aggregated"]
  },
  "timeout": 60000
}
EOF

agents_call_function \
  --via cloud \
  --node "$NODE" \
  --function 'topology:network-connections' \
  --body "$PAYLOAD"
```

Example with exact per-PID raw fields and selected labels:

```bash
agents_call_function \
  --via cloud \
  --node "$NODE" \
  --function 'topology:network-connections' \
  --body '{"timeout":60000,"selections":{"group_by":["pid"],"labels":["team|app"]}}' \
  | jq '.data | {
      group_by: .view.group_by,
      process_scopes: .types.actor_types.process.aggregation_scopes,
      container_scopes: .types.actor_types.container.aggregation_scopes,
      actor_columns: [.actors.columns[].id]
    }'
```

Example Kubernetes pod/namespace view inspection:

```bash
agents_call_function \
  --via cloud \
  --node "$NODE" \
  --function 'topology:network-connections' \
  --body '{"timeout":60000,"selections":{"group_by":["pid"]}}' \
  | jq '.data.actors as $actors
        | ($actors.columns | map(.id)) as $cols
        | ($cols | index("k8s_namespace")) as $ns
        | ($cols | index("k8s_pod_name")) as $pod
        | {namespaces: $actors.values[$ns].values, pods: $actors.values[$pod].values}'
```

## Response shape

Top-level response:

```json
{
  "status": 200,
  "type": "topology",
  "has_history": false,
  "data": {
    "schema_version": "netdata.topology.v1",
    "producer": {},
    "collected_at": "2026-05-09T10:00:00Z",
    "dictionaries": {},
    "types": {},
    "actors": {},
    "links": {},
    "evidence": {},
    "tables": {},
    "overlays": {},
    "stats": {}
  }
}
```

The important fields:

| Field | Description |
|---|---|
| `data.schema_version` | Topology contract version, currently `netdata.topology.v1` |
| `data.producer` | Producer source, instance, node, plugin, and version metadata |
| `data.dictionaries` | Shared dictionaries, especially `strings` |
| `data.types.actor_types` | Actor identity and aggregation-scope metadata |
| `data.types.link_types` | Link direction and aggregation policy |
| `data.types.evidence_types` | Evidence role and exact match columns |
| `data.actors` | Compact table of graph actors |
| `data.links` | Compact table of renderable graph links |
| `data.evidence` | Compact relationship evidence sections |
| `data.tables` | Optional actor or relationship detail tables |
| `data.overlays` | Optional metric/function overlay refs |
| `data.stats` | Producer and payload counters |

## Decode compact tables

Every table has:

- `rows`: number of rows;
- `columns`: column definitions;
- `values`: parallel array of column encodings.

Supported codecs:

| Codec | Meaning |
|---|---|
| `const` | one value repeated for all rows |
| `values` | one value per row |
| `dict` | per-column dictionary plus row indexes |

Minimal jq-friendly counts:

```bash
jq '.data | {
  schema: .schema_version,
  actors: .actors.rows,
  links: .links.rows,
  evidence_rows: ([.evidence[]?.table.rows] | add // 0),
  stats
}'
```

## Interpretation rules

- Actors are entities.
- Links are graph edges.
- Evidence rows are the exact facts behind links.
- Actor custom tables are separate from relationship evidence.
- Direction semantics come from `data.types.link_types`.
- Telemetry overlays come from `data.types.overlay_templates` plus
  `data.overlays.refs`.

Do not assume every evidence row is rendered as a graph edge. A single graph
link may summarize many evidence rows.

## See also

- [query-functions.md](./query-functions.md) -- generic Function transport.
- [../query-netdata-agents/query-topology.md](../query-netdata-agents/query-topology.md)
  -- direct-agent transport for the same topology payload.
