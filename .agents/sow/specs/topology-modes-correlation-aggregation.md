# Spec - Topology Modes, Correlation, Aggregation, And Actor Identification

## Status

Implementation contract introduced by SOW-0028.

SOW-0028 completed the cross-repo compatibility layer for modes, modal
identification, correlation classes, and table merge policy. The stronger
detailed network-connections loose-side graph model remains the target
behavior and is tracked by SOW-0029 because it requires a separate
Agent/UI/aggregator execution pass.

## Purpose

Topology payloads must let operators inspect a topology at the right level:

- exact evidence when they need detailed troubleshooting;
- compact relationships when they need an infrastructure-level map;
- consistent cross-node correlation when many independently produced payloads
  are merged;
- useful actor modals without duplicating the same facts only for display.

The contract must remain topology-agnostic. The UI and aggregator must not learn
domain words such as process, router, parent, child, endpoint, LLDP, socket, or
retention as hardcoded behavior. Producers describe identities, modes, merge
rules, table merge policies, and presentation recipes in the payload.

## Terms

- **Producer**: the Agent Function that emits one `netdata.topology.v1`
  payload, for example network-connections, SNMP/L2, streaming, or vSphere.
- **Aggregator**: Cloud service that fans out to many producers, decodes their
  payloads, correlates them, optionally aggregates detail, and returns a normal
  `netdata.topology.v1` payload.
- **UI**: Cloud frontend topology renderer and actor/link modal renderer.
- **Detailed mode**: payload or returned view that keeps the finest evidence
  grain the producer exposes for troubleshooting.
- **Aggregated mode**: payload or returned view that groups detailed evidence
  into compact relationships for map readability.
- **Known actor**: an entity the producer knows exists, such as a process, host,
  SNMP device, interface, streaming node, vSphere object, or Kubernetes object.
- **Loose side**: one side of a relationship row that has endpoint facts but no
  actor reference yet.
- **Materialized actor**: actor created from loose-side facts for presentation
  or partial correlation, such as an endpoint grouped by IP.
- **Replacement**: aggregation action where a weaker actor is removed and all
  incident relationships are rewired to a stronger actor.
- **Enrichment**: aggregation action where rows from multiple actors with the
  same identity are merged into one actor, preserving all non-conflicting facts.
- **Evidence table**: lossless or near-lossless relationship facts, for example
  sockets, L2 observations, or streaming relationships.
- **Relationship summary table**: compact relationship rows at a grain between
  graph links and detailed evidence.
- **Actor labels**: actor-owned key/value rows for modal labels and display
  selection. They are not a replacement for typed identity, matching, grouping,
  sorting, filtering, or aggregation columns.

## Global Rules

### Mode Request

The user-facing request key for topology mode is `__topology_mode`.

Allowed values:

- `detailed`
- `aggregated`

If the key is absent, each Function uses its documented default. Producers that
do not have a meaningful detailed/aggregated difference should not expose a mode
selector only to return identical output.

Mode-capable producers declare `data.view.supported_modes`. Consumers treat an
absent field or a single-value field as mode-invariant and must not show a
detailed/aggregated toggle for that payload.

### Aggregator Fanout

The aggregator must consume detailed payloads whenever a producer supports
detail mode.

When the user asks the aggregator for `__topology_mode=aggregated`, the
aggregator must rewrite fanout requests to producers as
`__topology_mode=detailed` before correlation and aggregation. This prevents
early information loss before cross-node matching. After correlation, the
aggregator returns either detailed or aggregated output according to the
original user request.

If a producer does not expose `__topology_mode`, the aggregator must not invent
that parameter for it. SNMP/L2 and streaming are expected to be mode-invariant
unless a future producer change defines a real mode difference.

### Final Output

Aggregator internal states must not appear in final payloads. Terms such as
absorbed, candidate, rewrite plan, partial class, or equivalence set may exist
inside the service, but final topology output contains only normal actors,
links, tables, labels, presentation, and diagnostics.

### No Duplicate Display Facts

Do not copy high-cardinality evidence rows only to make modal tables easier.
Modal sections must select and project existing actors, links, evidence,
relationship tables, actor tables, and actor labels.

Small scalar facts may appear in more than one plane when the grains differ. For
example, a graph link and a relationship-summary row may both carry
`socket_count`; the graph link is the renderable relationship, while the summary
row is the modal/drilldown grain.

### Actor Modal Identification

The modal identification area is part of the schema contract.

`types.actor_types.<id>.presentation.modal.labels` must be extended with an
ordered producer-selected identification list over the existing actor label
table. The UI renders those selected label keys near the actor title. The full
label table remains available in the Labels tab.

Target shape:

```json
{
  "labels": {
    "enabled": true,
    "table": "actor_labels",
    "actor_column": "actor",
    "key_column": "key",
    "value_column": "value",
    "identification": {
      "enabled": true,
      "fields": [
        { "key": "process", "label": "Process", "max_values": 1 },
        { "key": "username", "label": "User", "max_values": 1 },
        { "key": "cmdline", "label": "Command", "max_values": 1 }
      ]
    }
  }
}
```

Rules:

- `identification.fields[]` selects rows from `actor_labels` by `key`.
- Selection is per selected modal actor through `actor_column`.
- Repeated values are ordered by `value_index` when present.
- Missing selected keys are skipped; they do not create empty labels.
- `max_values` limits displayed values for one key. The full Labels tab still
  shows all values.
- The UI must not guess important labels from key names.
- Producers must not duplicate these values into a separate modal-only table.

## Schema Additions Required

This spec requires these schema extensions beyond the currently deployed v1
contract:

1. Modal label identification metadata:
   `modal.labels.identification.enabled` and
   `modal.labels.identification.fields[]`.
2. Link-side materialization policy for tables that can carry loose sides:
   producers must declare how a loose side can be grouped into presentation
   actors when a detailed row has only one real actor.
3. Aggregation/correlation rules must support three semantic outcomes:
   - loose-side resolution;
   - actor replacement;
   - actor enrichment.
4. Table merge policies must be explicit enough for the aggregator to merge
   streaming path, retention, inbound, outbound, SNMP observation, and
   relationship-summary rows without domain-specific code.

The implementation may encode these additions in the most compact shape that
fits the existing JSON schema style. This spec defines semantics; exact field
names are accepted when they are schema-valid, documented, and used uniformly
by Agent, UI, and aggregator.

## Correlation And Aggregation Model

### Rule Classes

Correlation rules are declarative. The aggregator builds keys from columns and
literals; it does not understand domain semantics.

Required rule classes:

- `resolve_loose_side`: matches a loose relationship side to a known actor or
  to a materialized partial actor.
- `replace_actor`: replaces weaker actors with stronger actors and rewires
  incident links/tables.
- `merge_enrich_actor`: merges actors with the same identity and combines their
  labels, attributes, links, evidence, and detail tables according to declared
  table policies.

### Priority

Rules run by ascending priority number. Exact rules must run before broader or
partial rules.

Example:

```json
{
  "rules": {
    "socket_exact": {
      "class": "resolve_loose_side",
      "priority": 10,
      "key_space": "socket",
      "key": [
        { "column": "protocol" },
        { "literal": "|" },
        { "column": "address_space" },
        { "literal": "|" },
        { "column": "ip" },
        { "literal": ":" },
        { "column": "port" }
      ],
      "output_link_type": "socket"
    },
    "ip_partial": {
      "class": "resolve_loose_side",
      "priority": 100,
      "key_space": "ip",
      "key": [
        { "column": "address_space" },
        { "literal": "|" },
        { "column": "ip" }
      ],
      "output_link_type": "partial_endpoint"
    }
  }
}
```

### Ambiguity

If one point matches exactly one claim, apply the rule.

If one point matches multiple claims with the same priority, the aggregator must
not pick randomly. It keeps the point unresolved or materializes a partial actor
according to the rule, and records a diagnostic.

If no match exists, the point or loose side remains unresolved and may be
materialized for UI display according to the declared materialization policy.

### Alias And NAT Evidence

NAT, load balancer, or alias information is modeled as additional keys for the
same point or claim. Alias rows add match possibilities; they do not mutate or
delete the original observation.

## Table Merge Policies

Every table type that can cross producer boundaries needs a merge policy.

Required dimensions:

- `key`: columns that identify equivalent rows.
- `action`: one of `deduplicate`, `append`, `set_union`, `merge_metrics`,
  `latest`, or `preserve`.
- `metrics`: per numeric column merge operation, such as `sum`, `min`, `max`,
  `avg_weighted`, or `latest`.
- `conflicts`: how non-key scalar conflicts are handled. Allowed policies are
  `prefer_claim`, `prefer_newest_agent`, `preserve_all`, or `diagnostic`.

General rules:

- Relationship evidence normally uses `append` or `deduplicate`.
- Relationship summaries normally use `merge_metrics` on the declared key.
- Actor labels use `set_union` keyed by actor, key, value, source, kind, and
  value_index.
- Streaming retention uses `preserve` or `append` when two parents retain data
  for the same node because each retaining parent is meaningful.
- Raw JSON columns cannot participate in key construction unless a scalar JSON
  path is explicitly declared in the table policy. Prefer typed scalar columns.

## Layer Responsibilities

### Agent Direct View

The Agent returns the producer's local topology.

- If detailed and aggregated modes are meaningful, the Agent may expose
  `__topology_mode`.
- Detailed mode keeps the finest useful evidence.
- Aggregated mode returns a readable local graph and modal relationship
  summaries.
- If modes are identical, the Agent should not expose a mode option.

### Aggregator View

The aggregator always collects the highest-detail useful input it can get.

- Requested detailed output:
  - fan out detailed when supported;
  - correlate and enrich;
  - return detailed rows with resolved actors where exact matches exist;
  - keep unresolved loose sides/materialized actors when no match exists.
- Requested aggregated output:
  - fan out detailed when supported;
  - correlate and enrich first;
  - aggregate into compact actors, links, relationship summaries, and modal
    tables according to schema policies.

### UI View

The UI renders the payload it receives.

- It does not run domain-specific correlation.
- It may materialize loose sides for direct-Agent detailed views only if the
  payload declares the materialization policy.
- It renders actor modal identification from
  `modal.labels.identification.fields[]`.
- It renders the full Labels tab from `actor_labels`.
- It renders modal tables from schema recipes and existing data planes.

## Network-Connections

### Domain Model

A socket observation is associated with a local process actor and two endpoint
tuples:

```text
protocol, src_ip, src_port, dst_ip, dst_port, direction, state
```

For inbound sockets, the process owns `dst_ip:dst_port`.
For outbound sockets, the process owns `src_ip:src_port`.
For local sockets, both process actors may be known.
For listening sockets, there is no remote side.

The exact remote tuple is required for cross-node correlation, but creating an
actor per `IP:PORT` in a single-node graph can explode actor count and force
layout noise. Therefore detailed mode must preserve the exact tuple without
requiring every tuple to be an actor.

### Agent Aggregated Mode

Agent aggregated mode is a readable local process dependency map.

Rules:

- Every graph link has two actor references.
- The producer creates materialized endpoint actors for unknown peers using a
  compact grouping policy, normally remote IP or remote IP plus address space.
- Exact socket tuples remain in relationship-summary rows when needed for
  modal drilldown, not as graph actors.
- Process actor size is driven by `socket_count`.
- Port bullets are driven by compact actor-owned port summaries, using a
  numeric count column.
- Node-to-process ownership links are graph-coherence links, not networking
  dependencies.

Synthetic example:

```json
{
  "view": { "mode": "aggregated" },
  "actors": [
    { "id": 1, "type": "node", "display_name": "node-a" },
    { "id": 2, "type": "process", "display_name": "api", "socket_count": 14 },
    { "id": 3, "type": "endpoint", "display_name": "198.51.100.20", "ip": "198.51.100.20" }
  ],
  "links": [
    { "src_actor": 1, "dst_actor": 2, "type": "ownership" },
    { "src_actor": 2, "dst_actor": 3, "type": "endpoint_socket", "protocol": "tcp", "direction": "outbound", "socket_count": 12 }
  ],
  "tables": {
    "relationship": {
      "connections": [
        { "src_actor": 2, "dst_actor": 3, "protocol": "tcp", "local_ip": "192.0.2.10", "local_port": 50120, "remote_ip": "198.51.100.20", "remote_port": 443, "socket_count": 12 }
      ]
    }
  }
}
```

The example uses row objects for readability. Production uses compact tables.

### Agent Detailed Mode

Agent detailed mode preserves exact socket evidence.

Rules:

- Actors are only known entities: node, process, container, and any other
  entity the producer knows exists.
- A link or evidence row may have one actor side and one loose side.
- The loose side carries typed endpoint facts: protocol, address space, IP,
  port, and side role.
- Listening rows have no remote side and no fake remote actor.
- Local sockets with both processes known have two actor refs.

Synthetic example:

```json
{
  "view": { "mode": "detailed" },
  "actors": [
    { "id": 1, "type": "node", "display_name": "node-a" },
    { "id": 2, "type": "process", "display_name": "api" }
  ],
  "evidence": {
    "socket": [
      {
        "src_actor": 2,
        "dst_actor": null,
        "protocol": "tcp",
        "direction": "outbound",
        "src_ip": "192.0.2.10",
        "src_port": 50120,
        "dst_ip": "198.51.100.20",
        "dst_port": 443,
        "loose_side": "dst"
      }
    ]
  }
}
```

### Aggregator Network-Connections

The aggregator receives detailed rows.

Exact match:

```text
node-a api outbound 192.0.2.10:50120 -> 198.51.100.20:443
node-b nginx inbound 192.0.2.10:50120 -> 198.51.100.20:443
```

Result:

```text
node-a/api -> node-b/nginx
```

No endpoint actors remain for the exact match. The loose side was resolved to a
known actor and the final graph is a normal process-to-process dependency.

Partial match:

```text
node-a api outbound -> 198.51.100.20:443
node-b is known to own 198.51.100.20
node-b has no matching process/socket row at collection time
```

Result:

```text
node-a/api -> node-b/[materialized endpoint for 198.51.100.20]
```

The graph remains truthful: the dependency points at node-b, but the exact
process could not be proven.

No match:

```text
node-a/api -> materialized endpoint 198.51.100.20
```

The unresolved endpoint remains visible with presentation that clearly differs
from resolved process links.

### UI Network-Connections

Direct Agent aggregated:

- Show process and endpoint actors.
- Show two-sided graph links.
- Process modal primary table is `Connections` from relationship-summary rows.
- Endpoint modal primary table is `Processes` from the same relationship rows.

Direct Agent detailed:

- Show known actors.
- Materialize loose sides only according to producer policy, for example by
  remote IP, so the graph remains readable.
- Process modal primary table is exact socket evidence.

Aggregator aggregated:

- Show the post-correlation compact dependency map.
- Exact cross-node matches are process-to-process.
- Unmatched loose sides are materialized according to policy.

Aggregator detailed:

- Show exact socket evidence with resolved actors where possible.
- Unresolved rows retain their loose-side facts.

## SNMP/L2

### Domain Model

SNMP/L2 topology observations are device, interface, neighbor, forwarding,
ARP, bridge, VLAN, and protocol facts. A graph link represents an observed or
inferred L2 relationship between two actors.

SNMP is not a loose-side topology. Every link should have two actors in both
Agent and aggregator views.

### Mode Behavior

SNMP/L2 detailed and aggregated modes are currently a no-op. The producer should
not expose `__topology_mode` until it has a real lower/higher-grain distinction.

If a global Cloud topology request asks for aggregated output, the aggregator
still consumes the same SNMP payload and returns the same semantic grain after
correlation/replacement.

### Correlation Behavior

SNMP mainly uses actor replacement.

Examples:

- A managed device actor is stronger than an LLDP remote placeholder that has
  the same chassis id.
- A managed interface actor is stronger than an inferred endpoint that has the
  same MAC/interface identity.
- A discovered management IP can help match a placeholder to a managed device,
  but ambiguous matches must not be chosen randomly.

Replacement example:

```text
payload-a: switch-a port 10 -> lldp-remote(chassis=aa:bb:cc)
payload-b: managed-switch-b(chassis=aa:bb:cc)
```

Result:

```text
switch-a port 10 -> managed-switch-b
```

The weaker LLDP remote actor is removed from the aggregated output and its
incident links/tables are rewired to the managed device actor.

### UI SNMP/L2

The UI should not expose a detailed/aggregated toggle for SNMP/L2 unless the
payload declares supported modes.

Device modals should remain port-centric:

- actor identification: device name, management IP, vendor, model, role, and
  other selected labels;
- full labels tab: all labels;
- ports table: one row per known interface/port, with SNMP `if_index` as the
  visible real numeric port ID when known, and the port name;
- expanded port rows: show a clickable neighbor actor and neighbor port name
  when graph-link facts can align the port to a remote actor;
- links/neighbor information: derived from the same port rows or aligned
  relationship rows so local port identity never contradicts the port table.

## Streaming

### Domain Model

Streaming topology describes Netdata Agent streaming relationships:

```text
child -> parent
parent <-> parent
virtual/stale/remote nodes represented as actors
```

All actors are real topology actors from the streaming view. Links always have
two actor refs. Streaming is not a loose-side topology.

### Mode Behavior

Streaming detailed and aggregated modes are currently a no-op. The producer
should not expose `__topology_mode` until it has a real lower/higher-grain
distinction.

The aggregator still consumes the same streaming payload for global aggregated
requests and returns merged/enriched streaming topology.

### Correlation Behavior

Streaming uses actor enrichment and table merging by `machine_guid`.

Example:

```text
child-1 -> parent-1 <-> parent-2 <- child-2
```

Both parents may report facts about the same node. The aggregator must not show
duplicate actors for the same `machine_guid`. It merges those actors and then
merges their tables according to table policy.

Required merge behavior:

- actor labels: set union;
- actor scalar facts: prefer non-empty, newest Agent version when explicitly
  comparable, otherwise preserve conflicts in diagnostics or expanded labels;
- stream path rows: deduplicate identical path membership rows;
- retention rows: preserve each retaining parent/source row, because multiple
  parents retaining the same child are meaningful;
- inbound/outbound stream rows: merge by declared relationship identity, with
  numeric metrics merged by table policy;
- links: merge by source actor, destination actor, type, direction, and state,
  then merge metrics/evidence according to link type policy.

Retention example:

```text
payload-parent-a: parent-a retains child-x for tier 0
payload-parent-b: parent-b retains child-x for tier 0
payload-child-x: child-x self retention tier 0
```

Result:

```text
actor child-x modal Retention table has 3 rows:
  retaining actor parent-a
  retaining actor parent-b
  retaining actor child-x
```

These rows must not be deduplicated away solely because the retained child is
the same.

### UI Streaming

The UI should not expose a detailed/aggregated toggle for streaming unless the
payload declares supported modes.

Actor modal identification should show selected labels such as role, hostname,
machine GUID when useful, and stream status. The full Labels tab remains
complete.

Tables:

- stream path: deduplicated path rows;
- retention for node: all retaining sources relevant to the selected actor,
  including the retaining `observer_actor`;
- retained nodes: all nodes whose data is maintained by the selected actor,
  using the same retention table filtered by `observer_actor`;
- received nodes: children, virtual nodes, stale nodes, and descendants
  received or transiting through the selected parent;
- upstream stream: the selected actor's own outgoing stream destination and
  stream attributes.

Highlight path must use the deduplicated stream-path table, not direct sibling
selection only.

## Cross-Topology Examples

### Agent Detailed To Aggregator Aggregated

Input request to Cloud:

```text
function=topology:network-connections __topology_mode=aggregated
```

Aggregator fanout:

```text
node-a: topology:network-connections __topology_mode=detailed
node-b: topology:network-connections __topology_mode=detailed
```

Aggregator work:

1. Decode detailed socket evidence from both nodes.
2. Resolve exact loose-side socket keys.
3. Materialize unresolved partial endpoints.
4. Aggregate graph links and relationship summaries.

Returned payload:

```json
{
  "view": { "mode": "aggregated" },
  "actors": "... compact post-correlation actor table ...",
  "links": "... compact post-correlation graph links ...",
  "tables": {
    "relationship": {
      "connections": "... aggregated drilldown rows ..."
    }
  }
}
```

### Agent Detailed To UI Direct Detailed

Input request to Agent:

```text
function=topology:network-connections __topology_mode=detailed
```

UI work:

1. Decode known actors and exact socket evidence.
2. Render known actors.
3. Materialize loose endpoints only as declared by the payload, normally by IP.
4. Show exact socket rows in process modals.

### SNMP Global Aggregated

Input request to Cloud:

```text
function=topology:snmp __topology_mode=aggregated
```

Aggregator fanout:

```text
node-a: topology:snmp
node-b: topology:snmp
```

No mode parameter is sent unless the producer advertises one. Aggregator applies
replacement rules and returns a normal graph.

### Streaming Global Aggregated

Input request to Cloud:

```text
function=topology:streaming __topology_mode=aggregated
```

Aggregator fanout:

```text
node-a: topology:streaming
node-b: topology:streaming
```

No mode parameter is sent unless the producer advertises one. Aggregator merges
actors by `machine_guid`, preserves retention rows by retaining source, and
deduplicates stream path rows by declared path identity.

## Open Edge Cases And Required Behavior

- **Ambiguous network socket match**: keep unresolved or materialize partial;
  record diagnostic; do not randomly choose a process.
- **Socket closed between node collections**: exact process match may fail;
  partial IP/node match may still be valid if a node/endpoint claim exists.
- **NAT or load balancer aliases**: add alias keys; do not overwrite original
  tuples.
- **SNMP duplicate weak actors**: replace all weak actors that match one strong
  actor; preserve evidence and diagnostics.
- **SNMP weak actor matches multiple strong actors**: keep weak actor visible or
  diagnostic; do not choose randomly.
- **Streaming stale node**: keep actor if it is meaningful to streaming status;
  merge by `machine_guid` only when identities match.
- **Streaming retention from multiple parents**: preserve rows; do not collapse
  them into one retained child row.
- **Actor label conflicts**: keep full label set; modal identification applies
  display limits only, not data loss.
- **High-cardinality detailed network-connections**: detailed mode may be large;
  aggregated mode must avoid actor-per-port explosion.

## Validation Requirements

Agent:

- Schema validation for all changed topology payloads.
- Network-connections fixtures for aggregated and detailed modes.
- SNMP fixture proving no mode selector is exposed unless behavior differs.
- Streaming fixture proving no mode selector is exposed unless behavior differs.
- Modal label identification metadata present for actor types with useful
  labels.

UI:

- Decode modal label identification metadata.
- Render selected identification labels in actor modal header.
- Keep full Labels tab.
- Render loose-side materialized actors only from schema policy.
- Do not show SNMP/streaming mode toggles unless payload capability declares
  them.

Aggregator:

- Rewrite `__topology_mode=aggregated` to `detailed` on fanout only when the
  producer supports it.
- Consume detailed network-connections and return both detailed and aggregated
  outputs.
- Resolve exact socket loose sides and preserve unresolved/partial cases.
- Replace SNMP weak actors with managed actors.
- Merge/enrich streaming actors and tables by `machine_guid` and table policy.
- Preserve schema-valid unknown future fields.
