# Netdata Topology Function Schema

This document defines the production topology payload contract for Netdata
Functions. It is the source of truth for new topology producers and for the
Cloud topology aggregator.

The JSON Schema is [FUNCTION_TOPOLOGY_SCHEMA.json](/src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json).

## Purpose

Topology payloads describe relationships between observed entities:

- infrastructure nodes and virtual machines;
- containers, pods, namespaces, and workloads;
- processes and sockets;
- network devices, ports, VLANs, MACs, and IPs;
- streaming parents and children;
- storage, virtualization, and custom topology domains.

The payload is optimized for two consumers:

- the Cloud aggregator, which merges topology evidence across nodes and views;
- the UI, which renders graph views and drilldown tables.

It is not optimized for reconstructing compatibility payloads. Parity with
compatibility consumers is a test concern, implemented by test-side projection
code.

## Non-Goals

Do not put these in production topology payloads:

- compatibility field names kept only for rollout adapters;
- instructions for reconstructing compatibility payloads;
- repeated per-row display strings or labels copied from actor/type metadata;
- duplicated actor modal rows that repeat relationship evidence;
- visual layout hints that the UI can own;
- raw CSS, raw SVG, component names, coordinates, viewport state, or force-layout
  physics;
- raw secrets or customer-identifying sample data in fixtures.

Type-level labels, UI-owned visual tokens, safe label policies, and small
column descriptions are acceptable when they help render generic topology UI.
Define them once per type or column, not once per row.

## Mental Model

The schema has six planes:

1. **Actors** are entities: nodes, processes, containers, ports, devices,
   vSphere objects, streaming agents, and custom objects.
2. **Graph links** are the renderable relationship groups between actors.
3. **Evidence** is the lossless relationship proof behind each graph link:
   sockets, LLDP observations, streaming path hops, vSphere inventory edges,
   or custom relationship facts.
4. **Detail tables** are non-graph data shown in actor or relationship
   drilldowns. They are explicitly typed as actor-owned or relationship-owned.
5. **Presentation** is backend-selected composition of UI-owned tokens for
   labels, colors, icons, legend, link styles, highlight behavior, and graph
   port bullets.
6. **Overlay refs** describe how to query refreshable metrics for links or
   actors without recomputing topology.

Graph links are intentionally smaller than evidence. A graph link may have
one evidence row or tens of thousands of evidence rows.

## Response Envelope

Topology Functions return a normal Function envelope with `type: "topology"`:

```json
{
  "status": 200,
  "type": "topology",
  "has_history": false,
  "update_every": 5,
  "expires": 10,
  "data": {
    "schema_version": "netdata.topology.v1",
    "producer": {
      "source": "network-connections",
      "instance": "local",
      "node_id": "node-uuid",
      "machine_guid": "machine-guid",
      "plugin": "network-viewer.plugin"
    },
    "collected_at": "2026-05-09T10:00:00Z",
    "dictionaries": {
      "strings": ["process", "endpoint", "tcp", "inbound", "established"]
    },
    "types": {
      "actor_types": {},
      "link_types": {}
    },
    "presentation": {
      "selection": {
        "actor_click": {"mode": "highlight_connections"}
      },
      "legend": {
        "actors": [],
        "links": [],
        "ports": []
      }
    },
    "actors": {
      "rows": 0,
      "columns": [],
      "values": []
    },
    "links": {
      "rows": 0,
      "columns": [],
      "values": []
    }
  }
}
```

`schema_version` is the topology contract version. It is not the producer
version. Producers may expose their own version in `producer.agent_version`,
`producer.plugin`, or `producer.capabilities`.

## Compact Tables

All large arrays use the same compact table shape:

```json
{
  "rows": 3,
  "columns": [
    {"id": "type", "type": "string_ref", "dictionary": "strings"},
    {"id": "socket_count", "type": "uint", "aggregation": "sum"}
  ],
  "values": [
    {"codec": "dict", "values": [0, 1], "indexes": [0, 0, 1]},
    {"codec": "values", "values": [10, 4, 19]}
  ]
}
```

`columns` and `values` are parallel arrays. The Nth value encoding belongs to
the Nth column. Every decoded column must produce exactly `rows` values.

Supported codecs:

- `const`: one value repeated for all rows;
- `values`: one value per row;
- `dict`: a per-column value dictionary plus integer indexes.

Column values may be plain scalars or references into a global dictionary.
For string-heavy columns, prefer `string_ref` with `dictionary: "strings"` and
integer values. This is how production socket evidence reached about 22 raw
bytes per socket evidence row in the measured corpus.

Use column type `json` only for actor/custom detail cells that must preserve
nested producer-owned data, such as SNMP port `neighbors` or `vlans`. Do not
use `json` for high-cardinality relationship evidence when a typed scalar,
reference, or array column can carry the fact.

Go topology producers should use `src/go/pkg/topology/v1` for the
`netdata.topology.v1` response model and compact-table constructors. The helper
validates row counts, parallel `columns`/`values` lengths, dictionary indexes,
and JSON round-trip behavior before a producer response reaches the Function
transport.

## Required Actor Semantics

The `actors` table must carry enough canonical identity for aggregation. Actor
row indexes are used as link endpoints inside the same payload.

Required actor columns:

- a type column, normally `type`;
- a layer column, normally `layer`;
- every identity column declared by `types.actor_types.<type>.identity`.

Common actor identity columns:

- `node_id`
- `machine_guid`
- `hostname`
- `ip`
- `mac`
- `process_name`
- `pid`
- `container_id`
- `container_name`
- `pod`
- `namespace`
- `k8s_label:<name>`
- `vsphere_moid`
- `vsphere_inventory_path`

Actor types define source-local identity and cross-source merge identity:

```json
{
  "types": {
    "actor_types": {
      "process": {
        "layer": "process",
        "identity": ["node_id", "process_name"],
        "merge_identity": ["node_id", "process_name"],
        "parent_identity": ["node_id"],
        "aggregation_scopes": ["node", "process_name", "container", "k8s_workload"],
        "presentation": {
          "label": "Process",
          "role": "actor",
          "icon": "process",
          "color_slot": "primary",
          "border": {"enabled": true},
          "size": {"mode": "link_count"},
          "label_policy": {
            "columns": ["display_name", "process_name"],
            "fallback": "type_label",
            "max_length": 80,
            "array": "reject"
          },
          "ports": {
            "show_bullets": true,
            "sources": [
              {
                "source": "evidence",
                "evidence": "socket",
                "actor_column": "src_actor",
                "name_column": "local_port",
                "default_type": "topology"
              },
              {
                "source": "evidence",
                "evidence": "socket",
                "actor_column": "dst_actor",
                "name_column": "remote_port",
                "default_type": "topology"
              }
            ]
          }
        }
      }
    }
  }
}
```

`identity` is what makes an actor unique in the producer payload.
`merge_identity` is what the Cloud aggregator may use across payloads.
`parent_identity` expresses containment or "lives on" relationships.

Do not use display names as identity. Identity is canonical matching data, not
UI text. If an identity column is not explicitly listed in the actor type
`presentation.label_policy.columns`, the UI must not use it as a label
fallback.

## Required Link Semantics

The `links` table is the renderable graph projection. It must contain:

- `src_actor`: actor row index;
- `dst_actor`: actor row index;
- `type`: link type id;
- enough counters or summary metrics for a graph view, such as
  `evidence_count` or `socket_count`.

Link rows are not required to be one-to-one with raw observations. They should
usually be grouped by graph identity. Evidence rows preserve the details.

Link types define direction and aggregation semantics:

```json
{
  "types": {
    "link_types": {
      "socket": {
        "orientation": "directed",
        "direction_role": "flow",
        "aggregation": {
          "direction": "preserve",
          "evidence": "append",
          "metrics": {
            "socket_count": "sum",
            "rtt_ms_max": "max"
          }
        },
        "evidence_types": ["socket"],
        "presentation": {
          "label": "Socket",
          "color_slot": "primary",
          "line_style": "solid",
          "width": "normal",
          "curve": "auto",
          "arrow": "forward",
          "variable": {
            "channel": "width",
            "scale_key": "sockets",
            "value_column": "socket_count",
            "min": "normal",
            "max": "emphasis"
          }
        }
      },
      "l2_adjacency": {
        "orientation": "undirected",
        "direction_role": "observation",
        "aggregation": {
          "direction": "canonicalize_unordered",
          "evidence": "append"
        },
        "evidence_types": ["snmp_l2_observation"]
      }
    }
  }
}
```

Direction rules:

- `directed` links preserve source and destination order;
- `undirected` links may be canonicalized by sorting endpoints;
- `hierarchical` links express ownership or containment;
- `observed_bidirectional` links preserve observation completeness without
  implying traffic direction.

`direction_role` tells the aggregator and UI what direction means:

- `flow`: traffic or socket direction;
- `dependency`: logical dependency direction;
- `ownership`: parent to child or owner to owned;
- `observation`: discovery direction only;
- `none`: direction is not meaningful.

## Presentation Plane

Presentation is compact and backend-controlled, but it is not raw frontend
layout. The UI owns token meanings; producers only choose from documented
tokens.

Type definitions carry their own presentation:

- actor types choose label, role, icon token, fill color token, border,
  annotation ring, size policy, safe label policy, and graph port-bullet policy;
- link types choose label, color token, line style, width token, curve token,
  arrow token, and one optional variable visual channel;
- port types choose label, color token, and opacity token.

The graph-level `data.presentation` object carries definitions that are not
owned by one type: legend order, actor-click highlight behavior, port tooltip
field labels, and scale-key labels.

Graph-level and type-level presentation are complementary. If both exist, there
is no precedence rule to apply: type-level presentation styles actor/link/port
types, while `data.presentation` describes cross-type behavior.

Example:

```json
{
  "types": {
    "port_types": {
      "topology": {
        "presentation": {
          "label": "Socket",
          "color_slot": "primary",
          "opacity": "normal"
        }
      }
    }
  },
  "presentation": {
    "profile_version": "network-connections.v1",
    "selection": {
      "actor_click": {
        "mode": "highlight_connections"
      }
    },
    "legend": {
      "actors": [
        {"type": "process", "label": "Process"}
      ],
      "links": [
        {"type": "socket", "label": "Socket"}
      ],
      "ports": [
        {"type": "topology", "label": "Socket"}
      ]
    },
    "port_fields": [
      {"key": "type", "label": "Type"}
    ],
    "scale_keys": {
      "sockets": {
        "label": "Sockets",
        "unit": "count"
      }
    }
  }
}
```

Safe label policy is mandatory for polished aggregated graphs. It prevents
canonical identity arrays such as many MAC addresses from becoming actor names.
Use only human-safe scalar columns in `label_policy.columns`; set
`fallback: "type_label"` unless showing row numbers is explicitly useful.

Link variable scaling is intentionally raw. The producer gives one numeric
`value_column` and one `scale_key`; Cloud or the UI scales visible links that
share the same key. `min` and `max` are visual tokens for the selected channel:
width scaling uses width tokens and opacity scaling uses opacity tokens. Do not
pre-scale to pixels or opacity in the producer.

Actor size supports:

- `fixed`: no data-driven size changes;
- `link_count`: size may reflect graph degree;
- `metric`: size comes from a numeric actor-table column named by
  `metric_column`.

When `selection.actor_click.mode` is `highlight_path`, the payload must also
set `path_table`, `path_actor_column`, and `path_order_column`. The path table
is an actor detail table. The actor column must contain path-member `actor_ref`
values and the order column must be numeric so the UI can render the path
deterministically. If the table carries paths for multiple clicked actors, also
set `path_owner_column` to an `actor_ref` column that identifies the actor whose
click should use that row.

Port bullets are graph presentation, not modal/table composition. Producers
must define `ports.sources[]` for actor types that set `show_bullets: true`.
Each source says where bullets come from:

- `links`: read from the graph links table;
- `evidence`: read from a named evidence type or evidence section;
- `actor_table`: read from a named actor detail table when present.

Each source must define `actor_column` and `name_column`. Optional
`type_column`, `status_column`, `mode_column`, `role_column`, and
`sources_column` enrich bullet color and tooltip fields. `default_type` must
reference `types.port_types` and is used when `type_column` is absent or empty.
`name_column` must reference a scalar display column, not raw actor/link/
evidence references, arrays, or JSON cells. For evidence sources, `evidence`
names an evidence type id.

`hover.fields` is intentionally lightweight graph hover metadata. Full modal
and table composition, column hiding, nested JSON rendering, and richer
formatting belong to the table-composition contract.

`annotation` is a small actor marker such as a ring or dot. It must use closed
color slots and style tokens; it is not a free-form badge or CSS hook.

Do not emit raw SVG icons. Use the closed UI icon token vocabulary and request
a new UI token when a producer needs a new visual concept.

### Closed Token Vocabulary

Color slots:

`primary`, `secondary`, `accent`, `self`, `neutral`, `muted`, `dim`, `derived`,
`info`, `structural`, `warning`, `success`, `danger`, `blue`, `green`,
`orange`, `purple`, `cyan`, `yellow`, `teal`, `gray`.

Prefer semantic slots such as `primary`, `warning`, `derived`, or `structural`
when they fit. Use hue slots only when a topology needs distinct categories that
do not map to the semantic slots, such as the vSphere migration.

Opacity tokens:

`normal`, `muted`, `faded`.

Width tokens:

`thin`, `normal`, `thick`, `emphasis`.

Icon tokens:

`router`, `switch`, `firewall`, `access_point`, `server`, `storage`,
`load_balancer`, `printer`, `phone`, `ups`, `camera`, `process`, `agent`,
`netdata-agent`, `parent`, `remote-endpoint`, `local-endpoint`, `segment`,
`self`, `ip`, `cloud`, `container`, `vm`, `database`, `service`, `datacenter`,
`cluster`, `host`, `network`, `datastore`, `datastore_cluster`,
`resource_pool`.

Producers must not emit tokens outside the schema. The UI should render
unsupported tokens with a safe fallback and record diagnostics for version skew.

## Evidence Plane

Evidence sections are keyed by evidence type. Each evidence row must reference
the graph link it supports using a `link_ref` column.

Example socket evidence type:

```json
{
  "types": {
    "evidence_types": {
      "socket": {
        "link_type": "socket",
        "role": "relationship_evidence",
        "match_columns": ["local_ip", "local_port", "remote_ip", "remote_port", "protocol", "direction"],
        "columns": [
          {"id": "link", "type": "link_ref", "role": "reference"},
          {"id": "local_ip", "type": "ip_ref", "dictionary": "strings", "role": "group_key"},
          {"id": "local_port", "type": "uint", "role": "group_key"},
          {"id": "remote_ip", "type": "ip_ref", "dictionary": "strings", "role": "group_key"},
          {"id": "remote_port", "type": "uint", "role": "group_key"},
          {"id": "protocol", "type": "string_ref", "dictionary": "strings", "role": "group_key"},
          {"id": "direction", "type": "string_ref", "dictionary": "strings", "role": "group_key"},
          {"id": "namespace", "type": "string_ref", "dictionary": "strings"},
          {"id": "rtt_ms_max", "type": "float", "unit": "ms", "aggregation": "max"}
        ]
      }
    }
  }
}
```

Evidence rows are the lossless plane for topology relationships. If Cloud must
cross-match sockets across nodes, every socket evidence row must remain
available. The graph link can be highly aggregated while the evidence rows stay
one-per-observation.

## Detail Tables

Detail tables support actor modals and drilldowns.

There are four roles:

- `relationship_evidence`: exact relationship rows, usually backed by an
  evidence section;
- `relationship_summary`: aggregated relationship rows derived from evidence;
- `actor_detail`: actor-owned custom data that is not generally aggregatable;
- `actor_inventory`: actor-owned inventory data that may be appended or set
  merged.

Streaming `stream_path` is actor detail. It must not be treated as a link
evidence table unless each row is also a relationship proof.

Some actor-detail data is intentionally producer-specific and not generally
aggregatable. Use table role `actor_detail` with aggregation `append` or
`none`; use column type `json` only for cells that need nested objects or
arrays that cannot be represented as scalar columns.

Example actor custom table type:

```json
{
  "types": {
    "table_types": {
      "stream_path": {
        "role": "actor_detail",
        "owner": "actor",
        "aggregation": "append",
        "columns": [
          {"id": "actor", "type": "actor_ref", "role": "reference"},
          {"id": "hop", "type": "uint"},
          {"id": "node_id", "type": "string_ref", "dictionary": "strings"},
          {"id": "since", "type": "timestamp"}
        ]
      }
    }
  }
}
```

Do not duplicate evidence in actor-owned tables. If a modal needs a socket list
for an actor, derive it from socket evidence by filtering evidence rows whose
link touches that actor.

## Telemetry Overlays

Topology should be refreshable without recomputing topology. Overlay templates
define how the UI or Cloud can query metrics for an actor or link.

Templates live once in the type registry. Overlay refs carry only template ids
and parameters.

Example:

```json
{
  "types": {
    "overlay_templates": {
      "snmp_interface_traffic": {
        "provider": "netdata.metrics",
        "contexts": ["snmp.interface_traffic"],
        "dimensions": ["received", "sent"],
        "selector_params": ["node_id", "interface_id"],
        "merge": {
          "refs": "set",
          "values": "sum"
        }
      }
    }
  },
  "overlays": {
    "refs": {
      "rows": 1,
      "columns": [
        {"id": "owner_kind", "type": "string_ref", "dictionary": "strings"},
        {"id": "owner", "type": "link_ref"},
        {"id": "template", "type": "string_ref", "dictionary": "strings"},
        {"id": "node_id", "type": "string_ref", "dictionary": "strings"},
        {"id": "interface_id", "type": "string_ref", "dictionary": "strings"}
      ],
      "values": []
    }
  }
}
```

Overlay refs are optional. Do not fabricate per-link bandwidth if the producer
does not have it. For network sockets, current evidence may include snapshot
metrics such as RTT or retransmissions, but that is not a time-series overlay
unless a query provider exists.

## Aggregation Rules

Aggregation must be schema-driven:

- actor type identity defines what actors can merge;
- link type direction policy defines whether edge direction is preserved;
- evidence type match columns define which details must remain exact;
- column aggregation rules define how summaries are produced.

Common column rules:

- `set`: preserve unique values;
- `sum`: add counters;
- `min` / `max`: preserve bounds;
- `last` / `first`: pick by observation order;
- `count`: count rows;
- `none`: not aggregatable.

If a table or column has no valid aggregation, mark it `none` and keep rows
attached to their owner. Do not silently drop rows.

## Network Connections Shape

For network-connections, graph links are grouped process-to-endpoint
relationships. Socket evidence preserves exact tuples.

Canonical socket evidence columns:

- graph link reference;
- local bind IP;
- local port;
- remote IP;
- remote port;
- protocol;
- direction;
- TCP state when available;
- network namespace or address-space tags;
- optional snapshot metrics such as RTT, receive RTT, retransmissions, and
  socket count.

Do not emit these per socket:

- repeated display strings or labels;
- `port_name` if it can be derived from port;
- duplicated `src` and `dst` objects;
- actor labels repeated as evidence labels;
- actor modal socket rows.

In the measured Cloud corpus, this production-only socket evidence shape was
about 7.25 MB raw for 323,077 socket evidence rows, or about 11.25 MB raw when
including current RTT/retransmission metrics.

## Streaming Shape

For streaming:

- actors are Netdata agents or streaming endpoints;
- links are parent/child or replication relationships;
- direction is meaningful as hierarchy/ownership, not packet flow;
- `stream_path` is an actor-detail table, not relationship evidence unless a
  row proves a specific relationship.

Streaming custom tables are allowed because they carry actor-owned state that
cannot be derived from links.

Streaming detail tables can contain stable node or Cloud identifiers. These
fields must not be used as graph labels and must not be copied into logs,
diagnostics, docs, SOWs, or durable review artifacts without redaction.

## SNMP/L2 Shape

For SNMP/L2:

- actors are devices, interfaces, VLANs, bridge domains, and endpoints;
- links are L2 adjacencies, containment, forwarding evidence, and ownership;
- adjacency links are usually `undirected` or `observed_bidirectional`;
- discovery direction is observation metadata and should not prevent
  aggregation of the same physical adjacency;
- FDB/ARP/LLDP/CDP rows are evidence sections or actor inventory tables,
  depending on whether each row proves a relationship.

SNMP interface traffic, packets, errors, and state should use overlay
templates with compact refs to node/context/label selectors.

## vSphere Shape

For vSphere:

- actors should use stable vSphere managed object ids when available;
- inventory path can be an additional merge identity but should not be the
  only identity when a stable object id exists;
- containment links such as datacenter -> cluster -> host -> VM are
  `hierarchical` with `direction_role: "ownership"`;
- VM-to-host relationships are topology facts, not metric overlays;
- metrics for datastore, host, or VM utilization should be overlay templates
  when the UI needs refreshable values.

The vSphere producer in the separate worktree must be updated in place only
after coordination with the user because another agent may be editing it.

## Test Reconstruction

Compatibility tests may project the new canonical payload into rollout adapter
shapes to prove that no information needed by compatibility consumers was lost.

That projection code must live only in tests or local schema-lab tools. It may
hardcode adapter presentation fields, display strings, modal table shapes, and
compatibility object layouts. None of that belongs in production payloads.

The acceptance check is:

1. decode the new canonical payload;
2. derive the compatibility shape in test code;
3. compare against sanitized fixtures or expected compatibility behavior.

Do not add fields to production payloads solely to make this projection easier.

## Producer Checklist

Before shipping a topology producer:

- define actor, link, evidence, table, and overlay types;
- choose actor identities that survive aggregation;
- separate graph links from evidence rows;
- mark direction semantics explicitly in link types;
- classify custom tables as actor detail, actor inventory,
  relationship evidence, or relationship summary;
- migrate old `show_port_bullets` to
  `types.actor_types.<id>.presentation.ports.show_bullets` and define
  `ports.sources[]` so bullet data is not implicit;
- use compact tables for high-cardinality sections;
- use `src/go/pkg/topology/v1` helpers for Go producers;
- avoid per-row display strings and duplicate labels;
- include only canonical facts and optional metrics the producer really has;
- validate with [FUNCTION_TOPOLOGY_SCHEMA.json](/src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json);
- run payload-size measurements on realistic or captured fixtures;
- keep raw real-environment payloads under `.local/` only.
