# Preserve Semantic Link Types

## Question

How should a topology producer preserve different visual treatments for links
that share the same general relationship family?

## Inputs

- A `netdata.topology.v1` producer.
- A graph links table with a required `type` column.
- Link protocols, states, or confidence markers that users must see
  differently, such as verified LLDP/CDP links versus inferred SNMP/L2 links.

## Schema Choices

- Use `links.type` for the renderable semantic link type, not only for the
  broad relationship family.
- Define one `data.types.link_types.<id>.presentation` object for every link
  type that needs distinct color, width, line style, curve, arrow, hover, or
  variable-scaling behavior.
- Keep raw producer facts, such as protocol, state, confidence, source port, and
  endpoint detail, in link columns or evidence rows.
- If evidence rows are grouped by link type, define a matching
  `data.types.evidence_types.<id>.link_type` and a matching `data.evidence`
  section for each emitted type.

## Implementation Steps

1. Inventory the legacy or intended visual categories.
   - Example: SNMP/L2 uses `lldp`, `cdp`, `bridge`, `fdb`, `stp`, `arp`,
     `snmp`, and `probable`.
2. Add all renderable categories to `data.types.link_types`.
3. Put the visual contract in the matching link type presentation.
   - Example: verified LLDP/CDP can use an accent color and thicker width.
   - Example: probable/inferred links can use a dim color or dashed line.
4. Emit the selected semantic type in `data.links.type` for every row.
5. Keep the original protocol/state in separate columns, so drilldowns and
   evidence stay factual.
6. Add legend entries for the user-facing categories that should be explained.
7. Add a test that decodes `data.links.type` and checks presentation tokens and
   evidence type linkage.

## Validation

- Validate the full response against `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.
- Run `topologyv1.ValidateDecodedData()` so type references are checked.
- For Go producers, add a package test that verifies:
  - rows with verified protocols keep their protocol link type;
  - inferred/probable rows use the inferred/probable link type;
  - every emitted evidence section uses an evidence type whose `link_type`
    matches the graph link type;
  - legend entries include the visible categories.

## Gotchas

- Do not collapse distinct visual categories into one generic link type and
  expect the UI to infer producer-specific meaning from protocol or state.
- Do not encode UI policy in frontend conditionals such as "if protocol is
  LLDP, make it green"; the producer owns type composition and the UI only maps
  schema tokens.
- If two facts are both true, keep both: the row can have `type: "probable"`
  and `protocol: "bridge"` at the same time.
- A fallback generic type such as `l2_observation` is useful for unknown
  protocols, but known user-visible categories should have explicit types.
