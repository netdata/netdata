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
| `topology:vsphere` | vSphere collector | inventory and virtualization relationships |

Always start with an info request to discover the parameters supported by the
Agent version you are querying.

## Endpoint

Use the standard Cloud Function endpoint:

`POST /api/v2/nodes/{nodeId}/function?function=topology:<source>`

Example info request:

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=topology:network-connections" \
  -d '{"info":true,"timeout":30000}'
```

Example data request:

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"

read -r -d '' PAYLOAD <<'EOF'
{
  "selections": {
    "mode": ["aggregated"]
  },
  "timeout": 60000
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=topology:network-connections" \
  -d "$PAYLOAD"
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
  evidence_rows: (.evidence | to_entries | map(.value.table.rows) | add // 0),
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
