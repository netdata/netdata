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
  "v": 3,
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

`v` is the optional Function transport protocol version. It belongs to the
response envelope, not to `data`, and is commonly `3` for Functions that accept
POST JSON parameters.

`schema_version` is the topology contract version. It is not the producer
version. Producers may expose their own version in `producer.agent_version`,
`producer.plugin`, or `producer.capabilities`.

Function `info` responses are response metadata, not topology payloads. They
may advertise `accepted_params`, `required_params`, and help text without a
`data` object. Validate only full topology responses against
`FUNCTION_TOPOLOGY_SCHEMA.json`.

## Mode Requests

Use the request parameter `__topology_mode` when a topology producer has a real
detailed vs aggregated output difference. Supported values are `detailed` and
`aggregated`.

Do not expose a detailed/aggregated selector for mode-invariant topologies.
SNMP/L2 and streaming currently emit the same topology grain for both use cases
and should not advertise a mode option until a real difference exists.

When a producer has a real mode split, set `data.view.supported_modes` to the
available modes. When the field is absent or contains a single value, consumers
must treat the topology as mode-invariant and avoid showing a mode toggle.

The Cloud topology aggregator consumes detailed payloads when a producer
supports the mode, even when the user asked Cloud for aggregated output. The
aggregator correlates first, then aggregates the returned graph. Producers must
therefore keep detailed mode lossless enough for cross-node matching.

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
                "source": "actor_table",
                "table": "socket_ports",
                "actor_column": "actor",
                "name_column": "port",
                "value_column": "socket_count",
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
          "layout": {
            "strength": "normal",
            "distance": "normal"
          },
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
  arrow token, tokenized layout strength/distance, and one optional variable
  visual channel;
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

Link layout is also tokenized. Producers may set
`types.link_types.<id>.presentation.layout.strength` and `.distance` for any
link type. These are relative hints for the UI force layout, not raw physics
values. The allowed strength tokens are `weakest`, `weaker`, `normal`,
`stronger`, and `strongest`. The allowed distance tokens are `closest`,
`closer`, `normal`, `farther`, and `farthest`.

Current Netdata topology tuning keeps `strength` at `normal` and varies only
`distance` where a topology needs semantic separation. Do not use non-normal
`strength` tokens for graph polish unless a later product decision explicitly
re-enables force-strength tuning.

Actor size supports:

- `fixed`: no data-driven size changes;
- `link_count`: size may reflect graph degree;
- `metric`: size comes from a numeric actor row column named by
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
`value_column`, `type_column`, `status_column`, `mode_column`, `role_column`,
and `sources_column` enrich bullet multiplicity, color, and tooltip fields.
`value_column` must reference a numeric source-table column; the UI sums it per
bullet key and uses the sum for visible bullet count, overflow, and sizing. Use
it when an aggregated row represents several observations, such as several
sockets on the same process port. `default_type` must reference
`types.port_types` and is used when `type_column` is absent or empty.
`name_column` must reference a scalar display column, not raw actor/link/
evidence references, arrays, or JSON cells. For evidence sources, `evidence`
names an evidence type id.

`hover.fields` is intentionally lightweight graph hover metadata. Full modal
and table composition lives in `types.actor_types.<id>.presentation.modal`,
`types.link_types.<id>.presentation.modal`, and optional
`types.table_types.<id>.presentation`.

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
        "match_columns": ["client_ip", "client_port", "server_ip", "server_port", "protocol"],
        "columns": [
          {"id": "link", "type": "link_ref", "role": "reference"},
          {"id": "src_actor", "type": "actor_ref", "role": "reference"},
          {"id": "dst_actor", "type": "actor_ref", "role": "reference"},
          {"id": "client_ip", "type": "ip_ref", "dictionary": "strings", "role": "group_key"},
          {"id": "client_port", "type": "uint", "role": "group_key"},
          {"id": "server_ip", "type": "ip_ref", "dictionary": "strings", "role": "group_key"},
          {"id": "server_port", "type": "uint", "role": "group_key"},
          {"id": "protocol", "type": "string_ref", "dictionary": "strings", "role": "group_key"},
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

## Actor Labels And Modal Composition

Actor modals are composed from existing topology facts. They do not get a
second copy of actor attributes, socket rows, SNMP endpoint objects, or
relationship evidence only for display.

Every actor modal has four top-level entities:

- actor name from the actor row through `presentation.label_policy`;
- actor labels from an actor-owned `actor_labels` table when labels exist;
- a depth-1 topology miniature built from existing incident links and opposite
  actors;
- one or more table sections built from actors, links, evidence, or detail
  tables.

Use a compact actor-owned label table for display labels and metadata:

```json
{
  "types": {
    "table_types": {
      "actor_labels": {
        "role": "actor_inventory",
        "owner": "actor",
        "aggregation": "set",
        "columns": [
          {"id": "actor", "type": "actor_ref", "role": "reference"},
          {"id": "key", "type": "string_ref", "dictionary": "strings"},
          {"id": "value", "type": "string_ref", "dictionary": "strings"},
          {"id": "source", "type": "string_ref", "dictionary": "strings", "nullable": true},
          {"id": "kind", "type": "string_ref", "dictionary": "strings", "nullable": true},
          {"id": "value_index", "type": "uint", "nullable": true}
        ]
      }
    }
  },
  "tables": {
    "actor": {
      "actor_labels": {
        "type": "actor_labels",
        "table": {"rows": 0, "columns": [], "values": []}
      }
    }
  }
}
```

The `key`, `value`, `source`, and `kind` columns may use either `string` or
`string_ref` encoding. Producers should prefer `string_ref` when a local
dictionary is already used, but aggregators and UI adapters must treat both
encodings as the same logical label fields.

Host/node actors should expose the complete host label set when available.
Non-node actors should expose all useful producer-known labels and metadata,
such as process command line, user, group, namespace, interface role, or
virtualization object properties. If a fact is needed for identity,
correlation, grouping, sorting, filtering, or aggregation, keep it as a typed
canonical actor/evidence/detail column too; `actor_labels` is not a replacement
for canonical data.

Repeated label values are repeated rows with the same `actor` and `key`,
ordered by `value_index`. Do not encode repeated labels as raw JSON arrays for
normal modal display.

`actor_labels` inherits the topology Function sensitive-data classification.
Labels may include command lines, users, host labels, system contact/location
fields, or other operator-controlled metadata. Cloud services, aggregators, and
UI adapters that consume topology payloads must apply the same access-control
assumptions as the source Function.

Use `modal.labels.identification.fields[]` to select the small ordered subset
of label keys that belongs in the actor modal identification/header area. This
selection references the existing `actor_labels` table; it must not duplicate
values into a separate modal-only table. Missing selected keys are skipped, and
the full Labels tab remains complete.

Modal sections are recipes over existing tables:

```json
{
  "types": {
    "actor_types": {
      "process": {
        "layer": "process",
        "identity": ["node_id", "process_name"],
        "presentation": {
          "label": "Process",
          "label_policy": {
            "columns": ["display_name", "process_name"],
            "fallback": "type_label",
            "array": "reject"
          },
          "modal": {
            "labels": {
              "enabled": true,
              "table": "actor_labels",
              "identification": {
                "fields": [
                  {"key": "process", "label": "Process", "max_values": 1},
                  {"key": "username", "label": "User", "max_values": 1}
                ]
              }
            },
            "mini_topology": {
              "enabled": true,
              "depth": 1,
              "exclude_link_types": ["ownership"]
            },
            "sections": [
              {
                "id": "connections",
                "label": "Connections",
                "source": {"kind": "links"},
                "owner_filter": {
                  "mode": "incident_link",
                  "src_actor_column": "src_actor",
                  "dst_actor_column": "dst_actor"
                },
                "row_filters": [
                  {"column": "type", "op": "not_in", "values": ["ownership"]}
                ],
                "columns": [
                  {
                    "id": "remote",
                    "label": "Remote",
                    "projection": {
                      "kind": "opposite_actor",
                      "src_actor_column": "src_actor",
                      "dst_actor_column": "dst_actor"
                    },
                    "cell": "actor_link"
                  },
                  {
                    "id": "protocol",
                    "label": "Protocol",
                    "projection": {"kind": "direct", "column": "protocol"},
                    "cell": "badge"
                  },
                  {
                    "id": "sockets",
                    "label": "Sockets",
                    "projection": {"kind": "direct", "column": "socket_count"},
                    "cell": "number"
                  }
                ]
              }
            ]
          }
        }
      }
    }
  }
}
```

Table recipes must support these source kinds:

- `actors`;
- `links`;
- `evidence` with an `evidence` id;
- `actor_table` with a `table` id;
- `relationship_table` with a `table` id.

Use `owner_filter` to bind rows to the selected actor or link. Common filters
are `actor_column`, `link_column`, `incident_link`, `incident_evidence`, and
`selected_link`.

Use projections instead of duplicated display fields:

- `direct`: read a column from the source row;
- `actor_ref_label`: render an actor-ref column through actor label policy;
- `opposite_actor`: render the opposite endpoint of a link row;
- `formatted_endpoint`: combine IP, port, and optional protocol columns;
- `selected_side_endpoint`: choose local or remote endpoint fields based on
  whether the selected actor matches the source or destination actor columns;
  producers must provide `src_actor_column` and `dst_actor_column` plus at
  least one local endpoint column and one remote endpoint column;
- `label_lookup`: read a value from `actor_labels` by `label_key`; omit
  `actor_column` to look up labels for the selected modal actor, or provide an
  actor-ref source column when the lookup belongs to another actor in the row;
- `coalesce`: choose the first non-empty column;
- `json_path`: extract a declared scalar `path` from a declared JSON `column`;
- `const`: emit a fixed value.

Use cell types to keep rendering generic and polished:

- `text`;
- `number`;
- `badge`;
- `actor_link`;
- `timestamp`;
- `duration`;
- `endpoint`;
- `array_count`;
- `debug_json`.

Use visibility annotations instead of duplicating rows:

- `table`: shown in the normal table;
- `expanded`: hidden from the main grid but shown when the row expands;
- `hidden`: available for joins, sorting, or future use, not displayed;
- `debug`: diagnostic-only. Raw JSON belongs here unless a curated scalar
  projection exists.

`json` columns are allowed only when they preserve nested producer-owned facts
that the UI or aggregator understands through declared projections, or when
they are explicitly marked as `debug_json`. They are not acceptable as the
normal user-facing rendering for labels, endpoint objects, neighbor arrays, or
actor attributes.

Use `empty_label` for the section-level empty-state label. Use column
`badge_map` only for explicit value-to-token mapping, and keep `align` and
`sortable` as presentation hints over already-projected columns. These fields
must not add new data or embed UI components.

The Cloud aggregator should preserve and merge modal/table definitions by the
same namespace/deduplicate rules used for type presentation. It should not
materialize modal rows during aggregation unless it is already merging the
underlying canonical table.

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

## Correlation Plane

Correlation is producer-visible graph semantics, not aggregator state. Producers
declare how independently produced topology maps can be correlated by keys, but
they do not expose internal aggregator states such as candidate, absorbed,
rewrite plan, or equivalence class in final payloads.

Correlation can resolve several shapes:

- loose relationship sides, where one side of a detailed row has endpoint facts
  but no known actor;
- visible correlation actors, where the input graph intentionally materializes
  unresolved peers;
- weaker placeholder actors that should be replaced by stronger managed actors;
- equivalent actors that should be merged and enriched with facts from multiple
  payloads.

Use `data.correlation` when a topology can resolve correlation actors across
payloads:

- `rules` defines named correlation rules, priority, key space, key template,
  rule class, action, point actor types when visible points exist, optional
  claim actor types, correlation link types, and the final output link type;
- `points` is a compact table of visible correlation actors and their keys;
- `claims` is a compact table of real actors and the keys they satisfy.

Correlation keys are declarative. A key is built from table columns and string
literals. The aggregator normalizes values by column type and concatenates the
parts; it does not execute code and does not need to know what IP, port, MAC,
chassis id, vSphere MOID, or another domain value means.

Supported rule classes:

- `resolve_loose_side`: resolve a loose relationship side or visible
  correlation actor to a claim actor when the key is unambiguous;
- `replace_actor`: remove weaker placeholder actors and rewire incident
  relationships to stronger managed actors;
- `merge_enrich_actor`: merge actors with the same declared identity and merge
  their labels, attributes, evidence, and detail tables by table policy.

Supported rule actions remain the visible output behavior:

- `absorb`: on an exact unambiguous match, matching correlation actors are
  removed from the aggregated output, or loose-side placeholders are consumed,
  and incident correlation relationships are rewired to the matched real actor
  using `output_link_type`;
- `link`: on an unambiguous broader/partial match, the correlation actor remains
  visible, or a materialized partial actor remains visible, and a weak
  correlation link to the matched actor is emitted using `output_link_type`.

No match keeps the correlation actor visible. Ambiguous matches must stay
unresolved and produce diagnostics in the aggregator; producers must not encode
guessing policy in the schema.

NAT or other alias evidence can be represented by adding more point or claim
rows for the same actor and rule. The original key remains intact; aliases are
additional facts, not mutations of the source observation.

Links between real actors and visible correlation actors must use semantic
correlation link types even in a single-node topology. The UI and aggregator
must not infer correlation behavior from actor type names or topology kind. The
legend should include visible correlation actor and link types so users can
distinguish unresolved, partial, inferred, and resolved relationships.

## Network Connections Shape

For network-connections, graph links must be split into semantic families:

- node-to-process ownership links that keep the graph together;
- resolved process-to-process links where both process endpoints are already
  known;
- process-to-correlation-endpoint links for unresolved or cross-node socket
  endpoints.

Socket evidence preserves exact tuples.

Canonical socket evidence columns:

- graph link reference;
- source actor reference;
- destination actor reference;
- client IP;
- client port;
- server IP;
- server port;
- protocol;
- TCP state when available;
- network namespace or address-space tags;
- optional snapshot metrics such as RTT, receive RTT, retransmissions, and
  socket count.

Do not emit these per socket:

- repeated display strings or labels;
- `port_name` if it can be derived from port;
- duplicated endpoint objects;
- actor labels repeated as evidence labels;
- actor modal socket rows.

In the measured Cloud corpus, this production-only socket evidence shape was
about 7.25 MB raw for 323,077 socket evidence rows, or about 11.25 MB raw when
including current RTT/retransmission metrics.

Network-connections graph direction is dependency direction: link types use
`direction_role: "dependency"`, the client actor is always `src_actor`, and the
server actor is always `dst_actor`. The topology payload must not expose a
separate `local` direction. Local host or same-node sockets still resolve to
either inbound or outbound dependency direction, and same-node process pairs
should emit resolved process-to-process links when both actors are known.

For outbound sockets, the process claims the client `protocol + client_ip +
client_port` tuple and the correlation endpoint points at the server `protocol +
server_ip + server_port` tuple. For inbound sockets, the process claims the
server tuple and the correlation endpoint points at the client tuple. Listening
sockets have no remote correlation point.
The current `socket_exact` rule key is `protocol + address_space + ip + port`;
`address_space` prevents private or otherwise scoped endpoint identities from
matching across unrelated address domains.

Network-connections uses four link types with different graph semantics:

- `endpoint_socket`: process-to-correlation-endpoint links. These are the
  primary unresolved network dependencies in a single-node view. They should be
  solid, colored, thin, normal-strength, and normal-distance so unresolved
  endpoints do not force the graph to zoom out.
- `correlated_socket`: aggregator output after exact endpoint absorption. These
  are cross-payload process-to-process dependencies and should be
  normal-strength and farthest so independent topology clusters do not blend
  into one dense layout.
- `socket`: resolved local process-to-process socket links. These are gray,
  thin, normal-distance links whose width can vary by `socket_count`.
- `ownership`: node-to-process containment links. These are graph-coherence
  links, not network traffic. They should be dotted, faded/dim, thin, normal
  strength, and normal distance.

Aggregated network-connections payloads still need process port bullets without
shipping detailed socket evidence. Emit an `actor_inventory` table such as
`socket_ports` with `actor`, `port`, and numeric `socket_count`, then point the
process actor type's `presentation.ports.sources[]` at that actor table with
`value_column: "socket_count"`. Size process actors with
`presentation.size: {"mode": "metric", "metric_column": "socket_count"}`.

Network-connections actor modals should expose dependency semantics, not every
internal table as a peer tab:

- self/node actors: `Processes` over `links`, filtered to `type == ownership`;
- non-node actors in aggregated mode: `Dependencies` and `Dependants` over
  `tables.relationship.connections`;
- non-node actors in detailed mode: `Dependencies` and `Dependants` over
  `evidence.socket`;
- `Dependencies` filters rows where the selected actor is `src_actor`;
- `Dependants` filters rows where the selected actor is `dst_actor`.

Do not show `socket_ports` as a normal network-connections modal section. It is
only the graph port-bullet inventory. Put less common per-connection fields such
as retransmissions or receiver RTT behind `visibility: "expanded"` instead of
creating another duplicate tab.

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

Managed SNMP device actor modals are port-centric:

- use `modal.labels.identification.fields[]` to show key device labels such as
  device name, management IP, vendor, model, port counts, and LLDP/CDP counts
  in the modal identification area;
- use `actor_ports` as the primary `Ports` section;
- expose real port identity columns, including SNMP `if_index` as the visible
  numeric port ID when known, source `port_id`, display `name`, `if_name`,
  `if_descr`, `if_alias`, MAC, speed, status, mode, role, VLAN, FDB, link, and
  neighbor counts;
- never invent numeric port IDs. Do not use row order or any generated sequence;
  `if_index` must come from the device/SNMP facts;
- include compact expanded-row neighbor columns such as nullable
  `neighbor_actor` and `neighbor_port_name` when graph-link facts can align the
  port to a remote actor;
- use `actor_port_links` as the `Port Neighbors` section when device modal
  rows need remote actor/port/evidence details;
- keep endpoint, segment, and custom actors on a generic graph-link `Links`
  section when they do not own port inventory.

`actor_port_links` is an actor-owned modal index over existing graph links and
evidence. It may duplicate compact references and side-specific port facts so
the UI can show a device's local port next to its remote actor, but it must not
duplicate raw LLDP/CDP/FDB/ARP/STP evidence JSON. Every row should include the
selected actor, link reference, remote actor, local `if_index`, local port name,
remote port facts, protocol, link type, state, evidence count, confidence,
inference, attachment mode, and timestamps when known.

SNMP endpoint port names must come only from real port fields: `port_name`,
`if_name`, `if_descr`, or source `port_id`. Do not fall back to actor labels
such as `display_name` or `sys_name`; those belong in actor labels, not in
local/remote port cells.

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
- expose host/node labels in an actor-owned `actor_labels` table when
  available;
- expose useful non-node actor labels and metadata in `actor_labels`, while
  keeping identity, correlation, grouping, sorting, filtering, and aggregation
  facts as typed canonical columns;
- define actor/link modal sections as recipes over existing actors, links,
  evidence, and detail tables;
- migrate old `show_port_bullets` to
  `types.actor_types.<id>.presentation.ports.show_bullets` and define
  `ports.sources[]` so bullet data is not implicit;
- use compact tables for high-cardinality sections;
- use `src/go/pkg/topology/v1` helpers for Go producers;
- avoid per-row display strings and duplicate labels;
- keep raw JSON out of normal user-facing modal tables unless a section marks
  it as explicit debug output;
- include only canonical facts and optional metrics the producer really has;
- validate with [FUNCTION_TOPOLOGY_SCHEMA.json](/src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json);
- run payload-size measurements on realistic or captured fixtures;
- keep raw real-environment payloads under `.local/` only.
